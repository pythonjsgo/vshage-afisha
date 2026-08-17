package webreg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns every SQL statement in this package. Written with raw pgx
// to match the rest of this backend (internal/events/repository.go) — the
// repo has no sqlc setup despite what CLAUDE.md claims.
type Repository struct {
	pool   *pgxpool.Pool
	ipSalt string
}

func NewRepository(pool *pgxpool.Pool, ipSalt string) *Repository {
	return &Repository{pool: pool, ipSalt: ipSalt}
}

// HashKey is the one-way transform applied to organizer manage keys before
// they touch the database.
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func (r *Repository) hashIP(ip string) string {
	if ip == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(r.ipSalt + "|" + ip))
	return hex.EncodeToString(sum[:])[:32]
}

const eventCols = `
	slug, title, COALESCE(tagline, ''), COALESCE(description, ''), COALESCE(cover_url, ''),
	starts_at, ends_at, timezone, venue, form, fields, affiliations, bridge,
	COALESCE(organizer_title, ''), capacity, registration_open,
	publish_afisha, publish_vshage, ticket_mode
`

// GetEvent returns the public event plus its live registration count.
// pgx.ErrNoRows when the slug is unknown.
func (r *Repository) GetEvent(ctx context.Context, slug string) (*Event, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+eventCols+`,
		       (SELECT COUNT(*) FROM webreg_registrations rg WHERE rg.event_slug = e.slug)
		FROM webreg_events e
		WHERE e.slug = $1
	`, slug)

	var (
		ev                                              Event
		venueRaw, formRaw, fieldsRaw, affRaw, bridgeRaw []byte
	)
	err := row.Scan(
		&ev.Slug, &ev.Title, &ev.Tagline, &ev.Description, &ev.CoverURL,
		&ev.StartsAt, &ev.EndsAt, &ev.Timezone, &venueRaw, &formRaw, &fieldsRaw, &affRaw, &bridgeRaw,
		&ev.OrganizerTitle, &ev.Capacity, &ev.RegistrationOpen,
		&ev.PublishAfisha, &ev.PublishVshage, &ev.TicketMode, &ev.RegisteredCount,
	)
	if err != nil {
		return nil, err
	}

	// A malformed JSONB blob must not take the page down: decode
	// best-effort and keep serving with the field empty.
	_ = json.Unmarshal(venueRaw, &ev.Venue)
	_ = json.Unmarshal(formRaw, &ev.Form)
	_ = json.Unmarshal(fieldsRaw, &ev.Fields)
	_ = json.Unmarshal(affRaw, &ev.Affiliations)
	_ = json.Unmarshal(bridgeRaw, &ev.Bridge)
	// Version 0 means the row predates the configurable form (or its JSON was
	// unreadable). Serving the legacy shape keeps a live event's form stable
	// through the deploy instead of silently emptying it.
	if ev.Form.Version == 0 {
		ev.Form = LegacyForm()
	}
	if ev.TicketMode == "" {
		ev.TicketMode = TicketOff
	}
	if ev.Fields == nil {
		ev.Fields = []Field{}
	}
	if ev.Affiliations == nil {
		ev.Affiliations = []string{}
	}
	if ev.Capacity != nil {
		left := *ev.Capacity - ev.RegisteredCount
		if left < 0 {
			left = 0
		}
		ev.SeatsLeft = &left
	}
	return &ev, nil
}

// Register inserts (or refreshes) one signup. It is idempotent on
// (event_slug, dedup_key): a double tap or a retried request updates the
// existing row instead of creating a duplicate, and reports back that the
// visitor was already on the list.
//
// The ticket code is issued once and never rotates — a returning visitor gets
// back the same code, because they may already have it open at the door.
func (r *Repository) Register(ctx context.Context, ev *Event, in *RegisterInput, ip, ua string) (*RegisterResult, error) {
	slug := ev.Slug
	display := in.Answers["__tg_display"]
	answers := make(map[string]string, len(in.Answers))
	for k, v := range in.Answers {
		if k == "__tg_display" {
			continue
		}
		answers[k] = v
	}
	answersJSON, err := json.Marshal(answers)
	if err != nil {
		return nil, fmt.Errorf("marshal answers: %w", err)
	}

	key, err := dedupKey(ev.Form, in)
	if err != nil {
		return nil, err
	}

	var (
		id      int64
		already bool
		ticket  *string
	)
	for attempt := 0; ; attempt++ {
		var code string
		if ev.TicketMode != TicketOff && ev.TicketMode != "" {
			if code, err = newTicketCode(); err != nil {
				return nil, err
			}
		}
		// xmax <> 0 distinguishes an UPDATE from an INSERT on a conflicting
		// upsert — Postgres sets it to the updating transaction id.
		err = r.pool.QueryRow(ctx, `
			INSERT INTO webreg_registrations
				(event_slug, name, full_name, email, phone, tg_username, tg_display,
				 affiliation, answers, consent, source, ip_hash, user_agent,
				 dedup_key, ticket_code)
			VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''),
			        NULLIF($7, ''), NULLIF($8, ''), $9, $10, NULLIF($11, ''), NULLIF($12, ''),
			        NULLIF($13, ''), $14, NULLIF($15, ''))
			-- COALESCE, not a plain overwrite: a second submission that omits
			-- a field must not erase what the visitor already gave us. Someone
			-- who registered with a phone on their laptop and re-submits from
			-- a phone without filling it in would otherwise silently lose it
			-- from the organizer's list. A value that IS supplied still wins.
			ON CONFLICT (event_slug, dedup_key) DO UPDATE SET
				-- name is NOT NULL, so an omitted one arrives as '' rather than
				-- NULL and needs the NULLIF to fall through to the stored value.
				name        = COALESCE(NULLIF(EXCLUDED.name, ''), webreg_registrations.name),
				full_name   = COALESCE(EXCLUDED.full_name, webreg_registrations.full_name),
				email       = COALESCE(EXCLUDED.email, webreg_registrations.email),
				phone       = COALESCE(EXCLUDED.phone, webreg_registrations.phone),
				tg_username = COALESCE(EXCLUDED.tg_username, webreg_registrations.tg_username),
				tg_display  = COALESCE(EXCLUDED.tg_display, webreg_registrations.tg_display),
				affiliation = COALESCE(EXCLUDED.affiliation, webreg_registrations.affiliation),
				answers     = webreg_registrations.answers || EXCLUDED.answers,
				consent     = EXCLUDED.consent,
				ticket_code = COALESCE(webreg_registrations.ticket_code, EXCLUDED.ticket_code),
				updated_at  = NOW()
			RETURNING id, (xmax <> 0), ticket_code
		`, slug, in.Name, in.FullName, in.Email, in.Phone, in.TGUsername, display,
			in.Affiliation, answersJSON, in.Consent, in.Source, r.hashIP(ip),
			truncate(ua, maxUALen), key, code).Scan(&id, &already, &ticket)
		if err == nil {
			break
		}
		// The only collision worth retrying is two visitors drawing the same
		// ticket code in the same event. Three tries at 32^7 makes that a
		// non-event; anything else is a real error and must surface.
		if attempt < 2 && isUniqueViolation(err, "idx_webreg_reg_ticket") {
			continue
		}
		return nil, err
	}

	var position int
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM webreg_registrations WHERE event_slug = $1 AND id <= $2
	`, slug, id).Scan(&position); err != nil {
		// The signup is already committed; a failed count must not turn a
		// successful registration into an error for the visitor.
		position = 0
	}

	res := &RegisterResult{
		ID:                id,
		EventSlug:         slug,
		Status:            "registered",
		AlreadyRegistered: already,
		Position:          position,
	}
	if ticket != nil {
		res.TicketCode = *ticket
	}
	return res, nil
}

// isUniqueViolation reports whether err is a Postgres 23505 raised by the
// named constraint. Matching the constraint name matters: this table has two
// unique indexes, and retrying the wrong one would loop on a real conflict.
func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

// GetTicket returns one entry pass by its code. The code is the only secret
// involved, so the lookup is scoped to the event to keep a code from one
// event from resolving against another.
func (r *Repository) GetTicket(ctx context.Context, slug, code string) (*Ticket, error) {
	var (
		t        Ticket
		venueRaw []byte
		fullName *string
	)
	err := r.pool.QueryRow(ctx, `
		SELECT e.slug, e.title, e.starts_at, e.timezone, e.venue,
		       r.name, r.full_name, r.ticket_code, r.checked_in_at
		FROM webreg_registrations r
		JOIN webreg_events e ON e.slug = r.event_slug
		WHERE r.event_slug = $1 AND UPPER(r.ticket_code) = UPPER($2)
	`, slug, code).Scan(&t.EventSlug, &t.EventTitle, &t.StartsAt, &t.Timezone, &venueRaw,
		&t.Name, &fullName, &t.Code, &t.CheckedInAt)
	if err != nil {
		return nil, err
	}
	if fullName != nil {
		t.FullName = *fullName
	}
	var venue VenueCard
	_ = json.Unmarshal(venueRaw, &venue)
	t.VenueName, t.VenueAddr = venue.Name, venue.Address
	return &t, nil
}

// CheckIn marks a ticket used. Idempotent: scanning the same code twice keeps
// the first arrival time rather than overwriting it, so the door log stays
// truthful when a phone gets scanned again on the way back in.
func (r *Repository) CheckIn(ctx context.Context, slug, key, code string) (*Ticket, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE webreg_registrations r
		SET checked_in_at = COALESCE(r.checked_in_at, NOW())
		FROM webreg_events e
		WHERE e.slug = r.event_slug
		  AND r.event_slug = $1
		  AND e.manage_key_hash = $2
		  AND UPPER(r.ticket_code) = UPPER($3)
	`, slug, HashKey(key), code)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, pgx.ErrNoRows
	}
	return r.GetTicket(ctx, slug, code)
}

// ManageList returns the organizer view. The key is compared as a hash in
// SQL, so a wrong key is indistinguishable from a missing event.
func (r *Repository) ManageList(ctx context.Context, slug, key string) (*ManageList, error) {
	hash := HashKey(key)
	return r.manageList(ctx, slug, &hash)
}

// AdminList is the same view reached with the admin token: no per-event key.
// `owner` ограничивает выдачу кабинетом организатора; nil — админ, видит всё.
func (r *Repository) AdminList(ctx context.Context, slug string, owner *string) (*ManageList, error) {
	if err := r.assertOwner(ctx, slug, owner); err != nil {
		return nil, err
	}
	return r.manageList(ctx, slug, nil)
}

// assertOwner отвечает pgx.ErrNoRows, если события нет ИЛИ оно чужое —
// намеренно одинаково: чужой слаг не должен отличаться от несуществующего,
// иначе список чужих событий перебирается по кодам ответа.
func (r *Repository) assertOwner(ctx context.Context, slug string, owner *string) error {
	if owner == nil {
		return nil
	}
	var one int
	err := r.pool.QueryRow(ctx,
		`SELECT 1 FROM webreg_events WHERE slug = $1 AND owner_slug = $2`,
		slug, *owner).Scan(&one)
	return err
}

func (r *Repository) manageList(ctx context.Context, slug string, keyHash *string) (*ManageList, error) {
	var (
		out                ManageList
		formRaw, fieldsRaw []byte
	)
	err := r.pool.QueryRow(ctx, `
		SELECT slug, title, starts_at, timezone, form, fields, capacity, ticket_mode
		FROM webreg_events
		WHERE slug = $1 AND ($2::text IS NULL OR manage_key_hash = $2)
	`, slug, keyHash).Scan(&out.Slug, &out.Title, &out.StartsAt, &out.Timezone,
		&formRaw, &fieldsRaw, &out.Capacity, &out.TicketMode)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(formRaw, &out.Form)
	_ = json.Unmarshal(fieldsRaw, &out.Fields)
	if out.Form.Version == 0 {
		out.Form = LegacyForm()
	}
	if out.Fields == nil {
		out.Fields = []Field{}
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, name, full_name, email, phone, tg_username, tg_display,
		       affiliation, answers, ticket_code, checked_in_at, created_at
		FROM webreg_registrations
		WHERE event_slug = $1
		ORDER BY created_at ASC, id ASC
	`, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out.Registrations = []Registration{}
	for rows.Next() {
		var (
			reg                                                       Registration
			answersRaw                                                []byte
			fullName, email, phone, tgUser, tgDisp, affiliation, code *string
		)
		if err := rows.Scan(&reg.ID, &reg.Name, &fullName, &email, &phone, &tgUser, &tgDisp,
			&affiliation, &answersRaw, &code, &reg.CheckedInAt, &reg.CreatedAt); err != nil {
			return nil, err
		}
		reg.FullName = deref(fullName)
		reg.Email = deref(email)
		reg.Phone = deref(phone)
		reg.TGUsername = deref(tgUser)
		reg.TGDisplay = deref(tgDisp)
		reg.Affiliation = deref(affiliation)
		reg.TicketCode = deref(code)
		_ = json.Unmarshal(answersRaw, &reg.Answers)
		if reg.Answers == nil {
			reg.Answers = map[string]string{}
		}
		if reg.CheckedInAt != nil {
			out.CheckedIn++
		}
		out.Registrations = append(out.Registrations, reg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out.Total = len(out.Registrations)
	return &out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// AddWaitlist records an Android (or fallback iOS) visitor who wants the app
// when it lands. Idempotent per platform+username.
func (r *Repository) AddWaitlist(ctx context.Context, slug, platform, tgKey, tgDisplay, name string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO webreg_waitlist (event_slug, platform, tg_username, tg_display, name)
		VALUES (NULLIF($1, ''), $2, $3, $4, NULLIF($5, ''))
		ON CONFLICT (platform, tg_username) DO UPDATE SET
			event_slug = COALESCE(webreg_waitlist.event_slug, EXCLUDED.event_slug),
			tg_display = EXCLUDED.tg_display,
			name       = COALESCE(EXCLUDED.name, webreg_waitlist.name)
	`, slug, platform, tgKey, tgDisplay, name)
	return err
}

// UpsertEvent creates or replaces an event config. This is the "конфиг
// руками" path — no UI yet, by design.
// UpsertEvent пишет конфиг события. `owner` — кабинет, от имени которого
// пришёл запрос; nil означает админа.
//
// Слаг — это адрес страницы, и он общий на всю платформу. Значит организатор,
// набравший чужой слаг, без защиты переписал бы чужое событие целиком: у
// апсерта нет разницы между «создать своё» и «затереть соседнее». Поэтому
// ветка UPDATE выполняется только при совпадении владельца, а ноль изменённых
// строк превращается в отказ «слаг занят».
func (r *Repository) UpsertEvent(ctx context.Context, in *EventUpsert, owner *string) error {
	if err := validateUpsert(in); err != nil {
		return err
	}

	venueJSON, _ := json.Marshal(in.Venue)
	fieldsJSON, _ := json.Marshal(defaultSlice(in.Fields, []Field{}))
	affJSON, _ := json.Marshal(defaultSlice(in.Affiliations, []string{}))
	bridgeJSON, _ := json.Marshal(in.Bridge)

	// An omitted form means "create it with the sane default"; an explicitly
	// sent one is stamped with the current version so the reader takes it
	// literally instead of falling back to the legacy shape.
	form := DefaultForm()
	if in.Form != nil {
		form = *in.Form
		form.Version = formVersion
	}
	formJSON, _ := json.Marshal(form)

	tz := in.Timezone
	if tz == "" {
		tz = "Europe/Moscow"
	}
	open := true
	if in.RegistrationOpen != nil {
		open = *in.RegistrationOpen
	}
	pubAfisha := true
	if in.PublishAfisha != nil {
		pubAfisha = *in.PublishAfisha
	}
	pubVshage := true
	if in.PublishVshage != nil {
		pubVshage = *in.PublishVshage
	}
	ticketMode := in.TicketMode
	if ticketMode == "" {
		ticketMode = TicketQR
	}

	// An empty manage_key on update keeps the existing one, so re-running the
	// config to fix a typo does not silently rotate the organizer's link.
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO webreg_events
			(slug, title, tagline, description, cover_url, starts_at, ends_at, timezone,
			 venue, form, fields, affiliations, bridge, organizer_title, capacity,
			 registration_open, publish_afisha, publish_vshage, ticket_mode, manage_key_hash,
			 owner_slug)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8,
		        $9, $10, $11, $12, $13, NULLIF($14, ''), $15, $16, $17, $18, $19, $20, $21)
		ON CONFLICT (slug) DO UPDATE SET
			title             = EXCLUDED.title,
			tagline           = EXCLUDED.tagline,
			description       = EXCLUDED.description,
			cover_url         = EXCLUDED.cover_url,
			starts_at         = EXCLUDED.starts_at,
			ends_at           = EXCLUDED.ends_at,
			timezone          = EXCLUDED.timezone,
			venue             = EXCLUDED.venue,
			form              = EXCLUDED.form,
			fields            = EXCLUDED.fields,
			affiliations      = EXCLUDED.affiliations,
			bridge            = EXCLUDED.bridge,
			organizer_title   = EXCLUDED.organizer_title,
			capacity          = EXCLUDED.capacity,
			registration_open = EXCLUDED.registration_open,
			publish_afisha    = EXCLUDED.publish_afisha,
			publish_vshage    = EXCLUDED.publish_vshage,
			ticket_mode       = EXCLUDED.ticket_mode,
			manage_key_hash   = COALESCE(NULLIF(EXCLUDED.manage_key_hash, ''), webreg_events.manage_key_hash),
			updated_at        = NOW()
		WHERE $21::text IS NULL OR webreg_events.owner_slug = $21
	`, in.Slug, in.Title, in.Tagline, in.Description, in.CoverURL, in.StartsAt, in.EndsAt, tz,
		venueJSON, formJSON, fieldsJSON, affJSON, bridgeJSON, in.OrganizerTitle, in.Capacity,
		open, pubAfisha, pubVshage, ticketMode, keyHashOrEmpty(in.ManageKey), owner)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// Конфликт слага с чужим событием: ветка UPDATE отфильтрована по
		// владельцу. Формулировка ровно про адрес — чьё именно событие там
		// лежит, организатору знать не нужно.
		return &APIError{Status: http.StatusConflict, Code: "slug_taken",
			Message: "Этот адрес уже занят другим событием — придумайте другой"}
	}
	return nil
}

// ListEvents is the admin index: every event with the numbers that decide
// what to do next, newest start first. `owner` — кабинет организатора; nil
// означает админа, который видит всё, включая события без владельца.
func (r *Repository) ListEvents(ctx context.Context, owner *string) ([]EventSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT e.slug, e.title, e.starts_at, e.timezone, e.registration_open,
		       e.publish_afisha, e.publish_vshage, e.capacity,
		       (SELECT COUNT(*) FROM webreg_registrations r WHERE r.event_slug = e.slug),
		       (SELECT COUNT(*) FROM webreg_registrations r
		         WHERE r.event_slug = e.slug AND r.checked_in_at IS NOT NULL)
		FROM webreg_events e
		WHERE $1::text IS NULL OR e.owner_slug = $1
		ORDER BY e.starts_at DESC
		LIMIT 200
	`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []EventSummary{}
	for rows.Next() {
		var s EventSummary
		if err := rows.Scan(&s.Slug, &s.Title, &s.StartsAt, &s.Timezone, &s.RegistrationOpen,
			&s.PublishAfisha, &s.PublishVshage, &s.Capacity, &s.Registered, &s.CheckedIn); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetEventConfig returns the full editable config for the admin editor,
// including the fields the public payload deliberately omits.
func (r *Repository) GetEventConfig(ctx context.Context, slug string, owner *string) (*EventUpsert, error) {
	if err := r.assertOwner(ctx, slug, owner); err != nil {
		return nil, err
	}
	var (
		out                                             EventUpsert
		venueRaw, formRaw, fieldsRaw, affRaw, bridgeRaw []byte
		open, pubA, pubV                                bool
	)
	err := r.pool.QueryRow(ctx, `
		SELECT slug, title, COALESCE(tagline, ''), COALESCE(description, ''),
		       COALESCE(cover_url, ''), starts_at, ends_at, timezone,
		       venue, form, fields, affiliations, bridge,
		       COALESCE(organizer_title, ''), capacity,
		       registration_open, publish_afisha, publish_vshage, ticket_mode
		FROM webreg_events WHERE slug = $1
	`, slug).Scan(&out.Slug, &out.Title, &out.Tagline, &out.Description, &out.CoverURL,
		&out.StartsAt, &out.EndsAt, &out.Timezone, &venueRaw, &formRaw, &fieldsRaw,
		&affRaw, &bridgeRaw, &out.OrganizerTitle, &out.Capacity,
		&open, &pubA, &pubV, &out.TicketMode)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(venueRaw, &out.Venue)
	_ = json.Unmarshal(fieldsRaw, &out.Fields)
	_ = json.Unmarshal(affRaw, &out.Affiliations)
	_ = json.Unmarshal(bridgeRaw, &out.Bridge)

	var form FormConfig
	_ = json.Unmarshal(formRaw, &form)
	if form.Version == 0 {
		form = LegacyForm()
	}
	out.Form = &form
	out.RegistrationOpen, out.PublishAfisha, out.PublishVshage = &open, &pubA, &pubV
	if out.Fields == nil {
		out.Fields = []Field{}
	}
	if out.Affiliations == nil {
		out.Affiliations = []string{}
	}
	return &out, nil
}

// SetManageKey rotates the organizer's secret link. Returns pgx.ErrNoRows for
// an unknown slug so the caller can answer 404 rather than a silent success.
func (r *Repository) SetManageKey(ctx context.Context, slug, key string, owner *string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE webreg_events SET manage_key_hash = $2, updated_at = NOW()
		  WHERE slug = $1 AND ($3::text IS NULL OR owner_slug = $3)`,
		slug, HashKey(key), owner)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func keyHashOrEmpty(key string) string {
	if key == "" {
		return ""
	}
	return HashKey(key)
}

func validateUpsert(in *EventUpsert) error {
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	if in.Slug == "" {
		return &APIError{Status: http.StatusBadRequest, Code: "slug_required", Message: "slug required"}
	}
	if !validSlug(in.Slug) {
		return &APIError{Status: http.StatusBadRequest, Code: "slug_invalid",
			Message: "slug must be 2-64 chars of a-z, 0-9, '-'"}
	}
	if strings.TrimSpace(in.Title) == "" {
		return &APIError{Status: http.StatusBadRequest, Code: "title_required", Message: "title required"}
	}
	if in.StartsAt.IsZero() {
		return &APIError{Status: http.StatusBadRequest, Code: "starts_at_required", Message: "starts_at required"}
	}
	if in.Timezone != "" {
		if _, err := time.LoadLocation(in.Timezone); err != nil {
			return &APIError{Status: http.StatusBadRequest, Code: "timezone_invalid",
				Message: fmt.Sprintf("unknown timezone %q", in.Timezone)}
		}
	}
	switch in.TicketMode {
	case "", TicketQR, TicketCode, TicketOff:
	default:
		return &APIError{Status: http.StatusBadRequest, Code: "ticket_mode_invalid",
			Message: "ticket_mode must be qr|code|off"}
	}
	if in.Form != nil && in.Form.Email.Enabled && !in.Form.Email.Required && in.TicketMode != TicketOff {
		// Not an error — an organizer may genuinely want optional email and
		// on-screen-only tickets — but it is worth naming, because "билет
		// придёт на почту" and "почта необязательна" together mean some
		// attendees get no email at all.
		log.Printf("webreg: event %q issues tickets but email is optional — those attendees get the code on screen only", in.Slug)
	}
	for i, f := range in.Fields {
		if strings.TrimSpace(f.Key) == "" || strings.TrimSpace(f.Label) == "" {
			return &APIError{Status: http.StatusBadRequest, Code: "field_invalid",
				Message: fmt.Sprintf("fields[%d]: key and label are required", i)}
		}
		if strings.HasPrefix(f.Key, "__") {
			return &APIError{Status: http.StatusBadRequest, Code: "field_invalid",
				Message: fmt.Sprintf("fields[%d]: keys starting with __ are reserved", i)}
		}
		switch f.Type {
		case "select", "text", "textarea", "checkbox":
		default:
			return &APIError{Status: http.StatusBadRequest, Code: "field_invalid",
				Message: fmt.Sprintf("fields[%d]: type must be select|text|textarea|checkbox", i)}
		}
	}
	return nil
}

func validSlug(s string) bool {
	if len(s) < 2 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			continue
		}
		return false
	}
	return true
}

func defaultSlice[T any](v []T, fallback []T) []T {
	if v == nil {
		return fallback
	}
	return v
}

// IsNotFound reports whether err means "no such row".
func IsNotFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
