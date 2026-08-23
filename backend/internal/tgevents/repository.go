package tgevents

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNoCover — у события нет обложки (или самого события): хендлер отвечает 404.
var ErrNoCover = errors.New("обложки нет")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// UpsertBulk идемпотентно записывает партию карточек: конфликт по id —
// обновление всех витринных полей. hidden НЕ трогается: снятое с витрины
// руками не должно возвращаться следующим импортом.
//
// Партия пишется одной транзакцией: наполовину применённый импорт хуже
// непримёнённого — витрина показала бы смесь старой и новой партии.
func (r *Repository) UpsertBulk(ctx context.Context, cards []Card) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	n := 0
	for _, c := range cards {
		if err := c.Validate(); err != nil {
			return 0, fmt.Errorf("карточка %s: %w", c.ID, err)
		}
		payload := []byte("{}")
		if c.Payload != nil {
			var err error
			if payload, err = json.Marshal(c.Payload); err != nil {
				return 0, fmt.Errorf("карточка %s: payload: %w", c.ID, err)
			}
		}
		var cover []byte
		var coverMime *string
		if c.CoverB64 != nil && *c.CoverB64 != "" {
			var err error
			if cover, err = base64.StdEncoding.DecodeString(*c.CoverB64); err != nil {
				return 0, fmt.Errorf("карточка %s: cover_b64 не base64: %w", c.ID, err)
			}
			if len(cover) > maxCoverBytes {
				return 0, fmt.Errorf("карточка %s: обложка %d байт > %d", c.ID, len(cover), maxCoverBytes)
			}
			coverMime = c.CoverMime
		}
		// cover через COALESCE: переимпорт БЕЗ обложек (или с недокачанной
		// одной) не должен стирать уже лежащие байты — тот же урок, что у
		// webreg-апсерта с EXCLUDED.x по телефону гостя.
		if _, err := tx.Exec(ctx, `
			INSERT INTO afisha_tg_events
				(id, title, annonce, date, date_end, time_start, city,
				 place_name, address, online, price_raw, is_free,
				 registration_url, access_level, segment, org_name,
				 source_url, payload, cover, cover_mime, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			        $13, $14, $15, $16, $17, $18, $19, $20, NOW())
			ON CONFLICT (id) DO UPDATE SET
				title            = EXCLUDED.title,
				annonce          = EXCLUDED.annonce,
				date             = EXCLUDED.date,
				date_end         = EXCLUDED.date_end,
				time_start       = EXCLUDED.time_start,
				city             = EXCLUDED.city,
				place_name       = EXCLUDED.place_name,
				address          = EXCLUDED.address,
				online           = EXCLUDED.online,
				price_raw        = EXCLUDED.price_raw,
				is_free          = EXCLUDED.is_free,
				registration_url = EXCLUDED.registration_url,
				access_level     = EXCLUDED.access_level,
				segment          = EXCLUDED.segment,
				org_name         = EXCLUDED.org_name,
				source_url       = EXCLUDED.source_url,
				payload          = EXCLUDED.payload,
				cover            = COALESCE(EXCLUDED.cover, afisha_tg_events.cover),
				cover_mime       = COALESCE(EXCLUDED.cover_mime, afisha_tg_events.cover_mime),
				updated_at       = NOW()`,
			c.ID, c.Title, c.Annonce, c.Date, c.DateEnd, c.TimeStart, c.City,
			c.PlaceName, c.Address, c.Online, c.PriceRaw, c.IsFree,
			c.RegistrationURL, c.AccessLevel, c.Segment, c.OrgName,
			c.SourceURL, payload, cover, coverMime,
		); err != nil {
			return 0, fmt.Errorf("карточка %s: %w", c.ID, err)
		}
		n++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return n, nil
}

// ListUpcoming — предстоящие нескрытые события: идёт сегодня или позже
// (для многодневных смотрим date_end). Сортировка — ближайшие первыми.
func (r *Repository) ListUpcoming(ctx context.Context, now time.Time, limit int) ([]Card, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	today := now.Format(dateLayout)
	rows, err := r.pool.Query(ctx, `
		SELECT id, title, annonce,
		       to_char(date, 'YYYY-MM-DD'),
		       to_char(date_end, 'YYYY-MM-DD'),
		       time_start, city, place_name, address, online,
		       price_raw, is_free, registration_url, access_level,
		       segment, org_name, source_url,
		       (cover IS NOT NULL) AS has_cover
		FROM afisha_tg_events
		WHERE NOT hidden
		  AND COALESCE(date_end, date) >= $1
		ORDER BY date, time_start NULLS LAST, id
		LIMIT $2`, today, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Card{}
	for rows.Next() {
		var c Card
		var hasCover bool
		if err := rows.Scan(&c.ID, &c.Title, &c.Annonce, &c.Date, &c.DateEnd,
			&c.TimeStart, &c.City, &c.PlaceName, &c.Address, &c.Online,
			&c.PriceRaw, &c.IsFree, &c.RegistrationURL, &c.AccessLevel,
			&c.Segment, &c.OrgName, &c.SourceURL, &hasCover); err != nil {
			return nil, err
		}
		if hasCover {
			// Свой origin, не CDN телеги: тот протухает за дни (замер 23.08).
			c.CoverURL = "/api/tg-events/" + c.ID + "/cover"
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Cover — байты обложки для отдачи браузеру. ErrNoCover, если события нет,
// оно скрыто или обложка не заливалась.
func (r *Repository) Cover(ctx context.Context, id string) ([]byte, string, error) {
	var data []byte
	var mime *string
	err := r.pool.QueryRow(ctx, `
		SELECT cover, cover_mime FROM afisha_tg_events
		WHERE id = $1 AND NOT hidden AND cover IS NOT NULL`, id).Scan(&data, &mime)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNoCover
	}
	if err != nil {
		return nil, "", err
	}
	m := "image/jpeg"
	if mime != nil && *mime != "" {
		m = *mime
	}
	return data, m, nil
}
