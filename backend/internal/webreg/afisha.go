package webreg

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pythonjsgo/vshage-afisha/internal/events"
)

// UpcomingForAfisha maps live web-registration events into the shape the
// afisha board renders, so an event created through the config endpoint shows
// up in the public listing as well as on its own /e/<slug> page (founder
// directive 2026-08-17).
//
// It implements events.ExtraSource. The dependency points this way — webreg
// imports events, never the reverse — so the board stays unaware that a second
// source exists beyond the interface.
//
// The filter is publish_afisha, not registration_open: an event whose seats
// are gone is still a real event happening in the city, and dropping it off
// the board the moment it fills up hides exactly the events worth seeing.
func (r *Repository) UpcomingForAfisha(ctx context.Context, since time.Time) ([]events.PublicEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT slug, title, tagline, description, cover_url, starts_at, ends_at,
		       venue, organizer_title, capacity,
		       (SELECT COUNT(*) FROM webreg_registrations rg WHERE rg.event_slug = e.slug)
		FROM webreg_events e
		WHERE publish_afisha AND starts_at >= $1
		ORDER BY starts_at ASC
		LIMIT 50
	`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []events.PublicEvent{}
	for rows.Next() {
		var (
			slug, title                    string
			tagline, description, coverURL *string
			startsAt                       time.Time
			endsAt                         *time.Time
			venueRaw                       []byte
			organizerTitle                 *string
			capacity                       *int
			registered                     int
		)
		if err := rows.Scan(&slug, &title, &tagline, &description, &coverURL,
			&startsAt, &endsAt, &venueRaw, &organizerTitle, &capacity, &registered); err != nil {
			return nil, err
		}

		var venue VenueCard
		_ = json.Unmarshal(venueRaw, &venue)

		ev := events.PublicEvent{
			// Prefixed so the id can never collide with a UUID from the
			// shared table — the frontend uses it as a list key.
			ID:               "webreg:" + slug,
			WebregSlug:       slug,
			Title:            title,
			ShortDescription: tagline,
			Description:      description,
			StartTime:        startsAt,
			EndTime:          endsAt,
			Status:           "published",
			Tags:             json.RawMessage("[]"),
			AttendeeCount:    registered,
			MaxAttendees:     capacity,
			PhotoURL:         nonEmpty(coverURL),
			OrganizerName:    organizerTitle,
			Photos:           []string{},
			PriceType:        strPtr("free"),
			Currency:         strPtr("RUB"),
			RegistrationMode: strPtr("auto"),
		}
		if venue.Name != "" {
			ev.VenueName = &venue.Name
		}
		if venue.Address != "" {
			ev.Address = &venue.Address
			ev.Location = &venue.Address
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func nonEmpty(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

func strPtr(s string) *string { return &s }
