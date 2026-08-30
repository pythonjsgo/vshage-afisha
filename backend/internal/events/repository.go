package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(p *pgxpool.Pool) *Repository {
	return &Repository{pool: p}
}

const selectCols = `
	e.id, e.title, d.short_description, e.description, e.location, e.start_time, e.end_time,
	e.status, e.category, COALESCE(e.tags, '[]'::jsonb),
	e.max_attendees, e.photo_url,
	COALESCE((SELECT COUNT(*) FROM event_registrations r
	          WHERE r.event_id = e.id AND r.status != 'cancelled'), 0),
	COALESCE(d.registration_mode, 'auto'), d.external_registration_url, d.registration_deadline,
	COALESCE(d.price_type, 'free'), d.price_min, d.price_max, COALESCE(d.currency, 'RUB'),
	d.city, d.venue_name, d.address, d.online_url, d.age_limit, d.attendees_note,
	f.position IS NOT NULL,
	f.position,
	p.name, p.photo_url,
	COALESCE(ARRAY(
		SELECT url FROM afisha_event_photos
		WHERE event_id = e.id
		ORDER BY position ASC
	), ARRAY[]::TEXT[])
`

// Joins referenced by selectCols. Used by every SELECT in this file.
const selectFrom = `
	FROM events e
	LEFT JOIN profiles p ON p.id = e.organizer_id
	LEFT JOIN organizer_event_details d ON d.event_id = e.id
`

func (r *Repository) List(ctx context.Context, q ListQuery) (ListResult, error) {
	since := q.Since
	if since.IsZero() {
		since = time.Now().Add(-24 * time.Hour)
	}

	featured, err := r.query(ctx, `
		SELECT `+selectCols+selectFrom+`
		INNER JOIN afisha_featured f ON f.event_id = e.id
		WHERE e.status = 'published'
		  AND e.start_time >= $1
		  AND COALESCE(d.visibility, 'public') = 'public'
		ORDER BY f.position ASC, e.start_time ASC
		LIMIT 10
	`, since)
	if err != nil {
		return ListResult{}, err
	}

	// Потолок общий с хендлером (maxWindow): раньше здесь стояло `> 100 → 30`,
	// и слияние получало 30 событий там, где просило 200 — страница выглядела
	// полной, а событий основного стора в ней не было вовсе.
	limit := q.Limit
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxWindow {
		limit = maxWindow
	}
	all, err := r.query(ctx, `
		SELECT `+selectCols+selectFrom+`
		LEFT JOIN afisha_featured f ON f.event_id = e.id
		WHERE e.status = 'published'
		  AND e.start_time >= $1
		  AND COALESCE(d.visibility, 'public') = 'public'
		ORDER BY e.start_time ASC
		LIMIT $2 OFFSET $3
	`, since, limit, q.Offset)
	if err != nil {
		return ListResult{}, err
	}

	var total int
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM events e
		LEFT JOIN organizer_event_details d ON d.event_id = e.id
		WHERE e.status = 'published'
		  AND e.start_time >= $1
		  AND COALESCE(d.visibility, 'public') = 'public'
	`, since).Scan(&total); err != nil {
		return ListResult{}, err
	}

	return ListResult{Featured: featured, All: all, Total: total}, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*PublicEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+selectCols+selectFrom+`
		LEFT JOIN afisha_featured f ON f.event_id = e.id
		WHERE e.id = $1
		  AND e.status = 'published'
		  AND COALESCE(d.visibility, 'public') = 'public'
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, pgx.ErrNoRows
	}
	var ev PublicEvent
	if err := rows.Scan(&ev.ID, &ev.Title, &ev.ShortDescription, &ev.Description, &ev.Location, &ev.StartTime, &ev.EndTime,
		&ev.Status, &ev.Category, &ev.Tags,
		&ev.MaxAttendees, &ev.PhotoURL, &ev.AttendeeCount,
		&ev.RegistrationMode, &ev.ExternalRegURL, &ev.RegDeadline,
		&ev.PriceType, &ev.PriceMin, &ev.PriceMax, &ev.Currency,
		&ev.City, &ev.VenueName, &ev.Address, &ev.OnlineURL, &ev.AgeLimit, &ev.AttendeesNote,
		&ev.IsFeatured, &ev.FeaturedPosition,
		&ev.OrganizerName, &ev.OrganizerPhoto, &ev.Photos); err != nil {
		return nil, err
	}
	return &ev, nil
}

func (r *Repository) RegisterPublic(ctx context.Context, eventID string, input PublicRegistrationInput) (*PublicRegistrationResult, error) {
	name := strings.TrimSpace(input.Name)
	contact := normalizePublicContact(input.Contact)
	if len([]rune(name)) < 2 {
		return nil, &RegistrationError{Status: http.StatusBadRequest, Code: "invalid_name", Message: "Укажите имя"}
	}
	if len(contact) < 5 {
		return nil, &RegistrationError{Status: http.StatusBadRequest, Code: "invalid_contact", Message: "Укажите контакт для связи"}
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var ev struct {
		ID           string
		Status       string
		StartTime    time.Time
		MaxAttendees *int
		RegMode      string
		Visibility   string
		ExternalURL  *string
		Deadline     *time.Time
		Registered   int
	}
	err = tx.QueryRow(ctx, `
		SELECT
			e.id, e.status, e.start_time, e.max_attendees,
			COALESCE(d.registration_mode, 'auto'),
			COALESCE(d.visibility, 'public'),
			d.external_registration_url,
			d.registration_deadline,
			(SELECT count(*)::int FROM event_registrations er
			 WHERE er.event_id = e.id AND er.status != 'cancelled')
		FROM events e
		LEFT JOIN organizer_event_details d ON d.event_id = e.id
		WHERE e.id = $1
		FOR UPDATE OF e
	`, eventID).Scan(&ev.ID, &ev.Status, &ev.StartTime, &ev.MaxAttendees, &ev.RegMode, &ev.Visibility, &ev.ExternalURL, &ev.Deadline, &ev.Registered)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &RegistrationError{Status: http.StatusNotFound, Code: "event_not_found", Message: "Событие не найдено"}
		}
		return nil, err
	}
	if ev.Status != "published" {
		return nil, &RegistrationError{Status: http.StatusConflict, Code: "registration_unavailable", Message: "Регистрация на это событие недоступна"}
	}
	if ev.Visibility != "public" {
		return nil, &RegistrationError{Status: http.StatusNotFound, Code: "event_not_found", Message: "Событие не найдено"}
	}
	if ev.RegMode == "external" {
		return nil, &RegistrationError{Status: http.StatusConflict, Code: "external_registration", Message: "Регистрация проходит на внешней странице", ExternalURL: ev.ExternalURL}
	}
	now := time.Now()
	if ev.Deadline != nil && now.After(*ev.Deadline) {
		return nil, &RegistrationError{Status: http.StatusConflict, Code: "registration_closed", Message: "Регистрация уже закрыта"}
	}
	if now.After(ev.StartTime) {
		return nil, &RegistrationError{Status: http.StatusConflict, Code: "registration_closed", Message: "Событие уже началось"}
	}
	if ev.MaxAttendees != nil && *ev.MaxAttendees > 0 && ev.Registered >= *ev.MaxAttendees {
		return nil, &RegistrationError{Status: http.StatusConflict, Code: "sold_out", Message: "Свободных мест больше нет"}
	}

	var profileID string
	deviceID := "afisha:" + publicContactHash(contact)
	profileID, err = upsertPublicProfile(ctx, tx, deviceID, name, contact)
	if err != nil {
		return nil, err
	}

	status := "registered"
	if ev.RegMode == "manual" {
		status = "waitlisted"
	}

	var existingID, existingStatus string
	err = tx.QueryRow(ctx, `
		SELECT id, status
		FROM event_registrations
		WHERE event_id = $1 AND profile_id = $2
		FOR UPDATE
	`, eventID, profileID).Scan(&existingID, &existingStatus)
	if err == nil {
		if existingStatus != "cancelled" {
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return &PublicRegistrationResult{
				RegistrationID:    existingID,
				EventID:           eventID,
				Status:            existingStatus,
				AlreadyRegistered: true,
			}, nil
		}
		if err := tx.QueryRow(ctx, `
			UPDATE event_registrations
			SET status = $3, created_at = now(),
			    signup_name = $4, signup_contact = $5
			WHERE event_id = $1 AND profile_id = $2
			RETURNING id, status
		`, eventID, profileID, status, name, contact).Scan(&existingID, &existingStatus); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &PublicRegistrationResult{RegistrationID: existingID, EventID: eventID, Status: existingStatus}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	// signup_name / signup_contact — то, что человек ВПИСАЛ в форму. Для гостя
	// без аккаунта это совпадает с профилем-пустышкой, а вот у живого
	// пользователя Вшаге запись цепляется к его настоящему профилю, который мы
	// намеренно не переписываем, — и без этих колонок вписанное имя пропадало
	// бы совсем (см. миграцию 009).
	var registrationID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO event_registrations (event_id, profile_id, status, signup_name, signup_contact)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, eventID, profileID, status, name, contact).Scan(&registrationID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &PublicRegistrationResult{RegistrationID: registrationID, EventID: eventID, Status: status}, nil
}

func (r *Repository) query(ctx context.Context, sql string, args ...any) ([]PublicEvent, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PublicEvent, 0, 16)
	for rows.Next() {
		var ev PublicEvent
		if err := rows.Scan(&ev.ID, &ev.Title, &ev.ShortDescription, &ev.Description, &ev.Location, &ev.StartTime, &ev.EndTime,
			&ev.Status, &ev.Category, &ev.Tags,
			&ev.MaxAttendees, &ev.PhotoURL, &ev.AttendeeCount,
			&ev.RegistrationMode, &ev.ExternalRegURL, &ev.RegDeadline,
			&ev.PriceType, &ev.PriceMin, &ev.PriceMax, &ev.Currency,
			&ev.City, &ev.VenueName, &ev.Address, &ev.OnlineURL, &ev.AgeLimit, &ev.AttendeesNote,
			&ev.IsFeatured, &ev.FeaturedPosition,
			&ev.OrganizerName, &ev.OrganizerPhoto, &ev.Photos); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func normalizePublicContact(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '(' || r == ')' || r == '-' {
			return -1
		}
		return r
	}, s)
}

func publicContactHash(contact string) string {
	sum := sha256.Sum256([]byte(contact))
	return hex.EncodeToString(sum[:])[:56]
}

// classifyPublicContact routes the single free-form contact field
// («телефон / email / telegram») into the profile column it belongs to, so
// the organizer panel and its CSV export can show a reachable contact
// instead of nothing. Input is already normalized: lowercased, with
// spaces/()- stripped.
func classifyPublicContact(contact string) (email, phone, telegram string) {
	c := contact
	if at := strings.Index(c, "@"); at > 0 && strings.Count(c, "@") == 1 && strings.Contains(c[at:], ".") {
		return c, "", ""
	}
	digits := strings.TrimPrefix(c, "+")
	if len(digits) >= 5 && strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }) == -1 {
		return "", c, ""
	}
	for _, p := range []string{"https://t.me/", "http://t.me/", "t.me/", "@"} {
		c = strings.TrimPrefix(c, p)
	}
	if r := []rune(c); len(r) > 100 { // profiles.telegram_id is VARCHAR(100)
		c = string(r[:100])
	}
	return "", "", c
}

func upsertPublicProfile(ctx context.Context, tx pgx.Tx, deviceID, name, contact string) (string, error) {
	email, phone, telegram := classifyPublicContact(contact)
	// A repeat registration carries one contact type; NULLIF+COALESCE keeps
	// the previously saved types instead of blanking them.
	update := func(profileID string) error {
		_, err := tx.Exec(ctx, `
			UPDATE profiles
			SET name = $2,
			    email = COALESCE(NULLIF($3, ''), email),
			    phone = COALESCE(NULLIF($4, ''), phone),
			    telegram_id = COALESCE(NULLIF($5, ''), telegram_id),
			    updated_at = now()
			WHERE id = $1`, profileID, name, email, phone, telegram)
		return err
	}

	var profileID string
	err := tx.QueryRow(ctx, `
		SELECT id
		FROM profiles
		WHERE device_id = $1
		ORDER BY created_at DESC
		LIMIT 1
		FOR UPDATE
	`, deviceID).Scan(&profileID)
	if err == nil {
		return profileID, update(profileID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	// Вставка идёт под точкой сохранения. Любая ошибка внутри транзакции
	// гасит её целиком (SQLSTATE 25P02), поэтому без SAVEPOINT ветка
	// восстановления ниже падала сама — «current transaction is aborted», —
	// а настоящая причина при этом терялась. Так 30.08 ломалась запись на
	// событие у каждого, чья почта/телефон/телеграм уже есть в profiles,
	// то есть ровно у живых пользователей приложения.
	if _, err := tx.Exec(ctx, `SAVEPOINT sp_public_profile`); err != nil {
		return "", err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO profiles (device_id, name, email, phone, telegram_id)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''))
		RETURNING id
	`, deviceID, name, email, phone, telegram).Scan(&profileID)
	if err == nil {
		_, _ = tx.Exec(ctx, `RELEASE SAVEPOINT sp_public_profile`)
		return profileID, nil
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return "", err
	}

	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT sp_public_profile`); err != nil {
		return "", err
	}

	// Конфликт мог прийти НЕ по device_id: на profiles висят уникальные
	// индексы по email, phone и telegram_id (частичные, WHERE NOT NULL), и
	// человек с аккаунтом Вшаге наступает именно на них. Ищем по тому же
	// ключу, по которому конфликт и случился: поиск только по device_id
	// ничего не находил и запись проваливалась ровно у своих.
	var foundDevice string
	err = tx.QueryRow(ctx, `
		SELECT id, device_id
		FROM profiles
		WHERE device_id = $1
		   OR ($2 <> '' AND email = $2)
		   OR ($3 <> '' AND phone = $3)
		   OR ($4 <> '' AND telegram_id = $4)
		ORDER BY (device_id = $1) DESC, created_at DESC
		LIMIT 1
		FOR UPDATE
	`, deviceID, email, phone, telegram).Scan(&profileID, &foundDevice)
	if err != nil {
		return "", err
	}
	// Чужой профиль не переписываем: update() ставит name, и без этой
	// проверки запись на лекцию переименовала бы живого человека в
	// приложении тем именем, которое вписали в форму на сайте.
	if foundDevice != deviceID {
		return profileID, nil
	}
	return profileID, update(profileID)
}
