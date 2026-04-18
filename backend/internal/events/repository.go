package events

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(p *pgxpool.Pool) *Repository {
	return &Repository{pool: p}
}

const selectCols = `
	e.id, e.title, e.description, e.location, e.start_time, e.end_time,
	e.status, e.category, COALESCE(e.tags, '[]'::jsonb),
	e.max_attendees, e.photo_url,
	COALESCE((SELECT COUNT(*) FROM event_registrations r
	          WHERE r.event_id = e.id AND r.status = 'confirmed'), 0),
	f.position IS NOT NULL,
	f.position,
	p.name, p.photo_url
`

// Joins referenced by selectCols. Used by every SELECT in this file.
const selectFrom = `
	FROM events e
	LEFT JOIN profiles p ON p.id = e.organizer_id
`

func (r *Repository) List(ctx context.Context, q ListQuery) (ListResult, error) {
	since := q.Since
	if since.IsZero() {
		since = time.Now().Add(-24 * time.Hour)
	}

	featured, err := r.query(ctx, `
		SELECT `+selectCols+selectFrom+`
		INNER JOIN afisha_featured f ON f.event_id = e.id
		WHERE e.status = 'published' AND e.start_time >= $1
		ORDER BY f.position ASC, e.start_time ASC
		LIMIT 10
	`, since)
	if err != nil {
		return ListResult{}, err
	}

	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	all, err := r.query(ctx, `
		SELECT `+selectCols+selectFrom+`
		LEFT JOIN afisha_featured f ON f.event_id = e.id
		WHERE e.status = 'published' AND e.start_time >= $1
		ORDER BY e.start_time ASC
		LIMIT $2 OFFSET $3
	`, since, limit, q.Offset)
	if err != nil {
		return ListResult{}, err
	}

	var total int
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM events
		WHERE status = 'published' AND start_time >= $1
	`, since).Scan(&total); err != nil {
		return ListResult{}, err
	}

	return ListResult{Featured: featured, All: all, Total: total}, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (*PublicEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+selectCols+selectFrom+`
		LEFT JOIN afisha_featured f ON f.event_id = e.id
		WHERE e.id = $1 AND e.status = 'published'
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, pgx.ErrNoRows
	}
	var ev PublicEvent
	if err := rows.Scan(&ev.ID, &ev.Title, &ev.Description, &ev.Location, &ev.StartTime, &ev.EndTime,
		&ev.Status, &ev.Category, &ev.Tags,
		&ev.MaxAttendees, &ev.PhotoURL, &ev.AttendeeCount, &ev.IsFeatured, &ev.FeaturedPosition,
		&ev.OrganizerName, &ev.OrganizerPhoto); err != nil {
		return nil, err
	}
	return &ev, nil
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
		if err := rows.Scan(&ev.ID, &ev.Title, &ev.Description, &ev.Location, &ev.StartTime, &ev.EndTime,
			&ev.Status, &ev.Category, &ev.Tags,
			&ev.MaxAttendees, &ev.PhotoURL, &ev.AttendeeCount, &ev.IsFeatured, &ev.FeaturedPosition,
			&ev.OrganizerName, &ev.OrganizerPhoto); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}
