package webreg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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
	starts_at, ends_at, timezone, venue, fields, affiliations, bridge,
	COALESCE(organizer_title, ''), capacity, registration_open
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
		ev                                     Event
		venueRaw, fieldsRaw, affRaw, bridgeRaw []byte
	)
	err := row.Scan(
		&ev.Slug, &ev.Title, &ev.Tagline, &ev.Description, &ev.CoverURL,
		&ev.StartsAt, &ev.EndsAt, &ev.Timezone, &venueRaw, &fieldsRaw, &affRaw, &bridgeRaw,
		&ev.OrganizerTitle, &ev.Capacity, &ev.RegistrationOpen, &ev.RegisteredCount,
	)
	if err != nil {
		return nil, err
	}

	// A malformed JSONB blob must not take the page down: decode
	// best-effort and keep serving with the field empty.
	_ = json.Unmarshal(venueRaw, &ev.Venue)
	_ = json.Unmarshal(fieldsRaw, &ev.Fields)
	_ = json.Unmarshal(affRaw, &ev.Affiliations)
	_ = json.Unmarshal(bridgeRaw, &ev.Bridge)
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
// (event_slug, tg_username): a double tap or a retried request updates the
// existing row instead of creating a duplicate, and reports back that the
// visitor was already on the list.
func (r *Repository) Register(ctx context.Context, slug string, in *RegisterInput, ip, ua string) (*RegisterResult, error) {
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

	var (
		id      int64
		already bool
	)
	// xmax <> 0 distinguishes an UPDATE from an INSERT on a conflicting
	// upsert — Postgres sets it to the updating transaction id.
	err = r.pool.QueryRow(ctx, `
		INSERT INTO webreg_registrations
			(event_slug, name, tg_username, tg_display, affiliation, answers, consent, source, ip_hash, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''))
		ON CONFLICT (event_slug, tg_username) DO UPDATE SET
			name        = EXCLUDED.name,
			tg_display  = EXCLUDED.tg_display,
			affiliation = EXCLUDED.affiliation,
			answers     = EXCLUDED.answers,
			consent     = EXCLUDED.consent,
			updated_at  = NOW()
		RETURNING id, (xmax <> 0)
	`, slug, in.Name, in.TGUsername, display, in.Affiliation, answersJSON,
		in.Consent, in.Source, r.hashIP(ip), truncate(ua, maxUALen)).Scan(&id, &already)
	if err != nil {
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

	return &RegisterResult{
		ID:                id,
		EventSlug:         slug,
		Status:            "registered",
		AlreadyRegistered: already,
		Position:          position,
	}, nil
}

// ManageList returns the organizer view. The key is compared as a hash in
// SQL, so a wrong key is indistinguishable from a missing event.
func (r *Repository) ManageList(ctx context.Context, slug, key string) (*ManageList, error) {
	var (
		out       ManageList
		fieldsRaw []byte
	)
	err := r.pool.QueryRow(ctx, `
		SELECT slug, title, starts_at, timezone, fields, capacity
		FROM webreg_events
		WHERE slug = $1 AND manage_key_hash = $2
	`, slug, HashKey(key)).Scan(&out.Slug, &out.Title, &out.StartsAt, &out.Timezone, &fieldsRaw, &out.Capacity)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(fieldsRaw, &out.Fields)
	if out.Fields == nil {
		out.Fields = []Field{}
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, name, tg_username, tg_display, affiliation, answers, created_at
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
			reg        Registration
			answersRaw []byte
		)
		if err := rows.Scan(&reg.ID, &reg.Name, &reg.TGUsername, &reg.TGDisplay,
			&reg.Affiliation, &answersRaw, &reg.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(answersRaw, &reg.Answers)
		if reg.Answers == nil {
			reg.Answers = map[string]string{}
		}
		out.Registrations = append(out.Registrations, reg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out.Total = len(out.Registrations)
	return &out, nil
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
func (r *Repository) UpsertEvent(ctx context.Context, in *EventUpsert) error {
	if err := validateUpsert(in); err != nil {
		return err
	}

	venueJSON, _ := json.Marshal(in.Venue)
	fieldsJSON, _ := json.Marshal(defaultSlice(in.Fields, []Field{}))
	affJSON, _ := json.Marshal(defaultSlice(in.Affiliations, []string{}))
	bridgeJSON, _ := json.Marshal(in.Bridge)

	tz := in.Timezone
	if tz == "" {
		tz = "Europe/Moscow"
	}
	open := true
	if in.RegistrationOpen != nil {
		open = *in.RegistrationOpen
	}

	// An empty manage_key on update keeps the existing one, so re-running the
	// config to fix a typo does not silently rotate the organizer's link.
	_, err := r.pool.Exec(ctx, `
		INSERT INTO webreg_events
			(slug, title, tagline, description, cover_url, starts_at, ends_at, timezone,
			 venue, fields, affiliations, bridge, organizer_title, capacity,
			 registration_open, manage_key_hash)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8,
		        $9, $10, $11, $12, NULLIF($13, ''), $14, $15, $16)
		ON CONFLICT (slug) DO UPDATE SET
			title             = EXCLUDED.title,
			tagline           = EXCLUDED.tagline,
			description       = EXCLUDED.description,
			cover_url         = EXCLUDED.cover_url,
			starts_at         = EXCLUDED.starts_at,
			ends_at           = EXCLUDED.ends_at,
			timezone          = EXCLUDED.timezone,
			venue             = EXCLUDED.venue,
			fields            = EXCLUDED.fields,
			affiliations      = EXCLUDED.affiliations,
			bridge            = EXCLUDED.bridge,
			organizer_title   = EXCLUDED.organizer_title,
			capacity          = EXCLUDED.capacity,
			registration_open = EXCLUDED.registration_open,
			manage_key_hash   = COALESCE(NULLIF(EXCLUDED.manage_key_hash, ''), webreg_events.manage_key_hash),
			updated_at        = NOW()
	`, in.Slug, in.Title, in.Tagline, in.Description, in.CoverURL, in.StartsAt, in.EndsAt, tz,
		venueJSON, fieldsJSON, affJSON, bridgeJSON, in.OrganizerTitle, in.Capacity, open,
		keyHashOrEmpty(in.ManageKey))
	return err
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
