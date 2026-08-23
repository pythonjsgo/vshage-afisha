package tgevents

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
		if _, err := tx.Exec(ctx, `
			INSERT INTO afisha_tg_events
				(id, title, annonce, date, date_end, time_start, city,
				 place_name, address, online, price_raw, is_free,
				 registration_url, access_level, segment, org_name,
				 source_url, payload, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			        $13, $14, $15, $16, $17, $18, NOW())
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
				updated_at       = NOW()`,
			c.ID, c.Title, c.Annonce, c.Date, c.DateEnd, c.TimeStart, c.City,
			c.PlaceName, c.Address, c.Online, c.PriceRaw, c.IsFree,
			c.RegistrationURL, c.AccessLevel, c.Segment, c.OrgName,
			c.SourceURL, payload,
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
		       segment, org_name, source_url
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
		if err := rows.Scan(&c.ID, &c.Title, &c.Annonce, &c.Date, &c.DateEnd,
			&c.TimeStart, &c.City, &c.PlaceName, &c.Address, &c.Online,
			&c.PriceRaw, &c.IsFree, &c.RegistrationURL, &c.AccessLevel,
			&c.Segment, &c.OrgName, &c.SourceURL); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
