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
// afishaStore — какими колонками веб-регистрация отвечает на вопросы фильтра
// ленты (см. events.StoreSQL).
//
// Города и категории у неё НЕТ ВОВСЕ, и пустая строка здесь значит именно
// это, а не «поле пустое». Следствия названы явно, потому что оба неочевидны:
//   - город: событие заводит наша команда, и оно московское, поэтому такие
//     карточки принадлежат городу по умолчанию; на доске второго города они
//     не покажутся, а не покажутся везде;
//   - категория: раздела у такого события нет, и ни в одну плитку оно не
//     попадает — в общей ленте при этом остаётся. Пустое честнее неверного:
//     приписать его «Другому» значило бы наполнить раздел событиями, которые
//     туда никто не относил.
//
// Free — TRUE безусловно: страница веб-регистрации бесплатна по устройству,
// и читатель ниже ставит price_type='free' каждой карточке. Условие написано
// явно, чтобы не выглядело забытым.
var afishaStore = events.StoreSQL{
	City:     "",
	Category: "",
	Free:     "TRUE",
	Start:    "starts_at",
	End:      "COALESCE(ends_at, starts_at)",
}

// afishaBase — предикат витрины: опубликовано на афишу и ещё не прошло. Один
// текст на список, счётчик и фасеты.
//
// Условие — publish_afisha, а не registration_open: событие, у которого
// кончились места, всё ещё происходит в городе, и снятие его с доски в
// момент заполнения прячет ровно те события, ради которых на доску и заходят.
func afishaBase(since string) string {
	return "publish_afisha AND starts_at >= " + since
}

func (r *Repository) UpcomingForAfisha(ctx context.Context, since time.Time, f events.Filter, limit, offset int) ([]events.PublicEvent, error) {
	// Клампинг, а не откат: просили больше потолка — отдаём потолок.
	// Откат к 30 означал бы «страница набрана», когда она не набрана.
	if limit <= 0 {
		limit = 30
	}
	if limit > events.MaxWindow {
		limit = events.MaxWindow
	}
	if offset < 0 {
		offset = 0
	}
	// Фильтр уезжает В SQL по той же причине, что и у остальных сторов: окно
	// режется до фильтра, и отсев в Go отдал бы разделу обрезки страницы.
	a := events.NewSQLArgs()
	base := afishaBase(a.Add(since))
	where := afishaStore.Where(f, a)
	rows, err := r.pool.Query(ctx, `
		SELECT slug, title, tagline, description, cover_url, starts_at, ends_at,
		       venue, organizer_title, capacity,
		       (SELECT COUNT(*) FROM webreg_registrations rg WHERE rg.event_slug = e.slug)
		FROM webreg_events e
		WHERE `+base+` AND (`+where+`)
		ORDER BY starts_at ASC
		LIMIT `+a.Add(limit)+` OFFSET `+a.Add(offset), a.All()...)
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

// CountUpcomingForAfisha — сколько всего событий веб-регистрации попадёт в
// ленту под текущим фильтром. Тот же WHERE, что и в выборке: разойдутся
// условия — разойдётся «показано N из M», и заметит это не тест, а человек,
// долиставший до конца.
func (r *Repository) CountUpcomingForAfisha(ctx context.Context, since time.Time, f events.Filter) (int, error) {
	a := events.NewSQLArgs()
	base := afishaBase(a.Add(since))
	where := afishaStore.Where(f, a)
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM webreg_events e
		WHERE `+base+` AND (`+where+`)
	`, a.All()...).Scan(&n)
	return n, err
}

// FacetsForAfisha — счётчики под текущим фильтром. Считает их общая
// events.CountFacets тем же описанием стора и тем же предикатом витрины, что
// и список.
func (r *Repository) FacetsForAfisha(ctx context.Context, since time.Time, f events.Filter) (events.Facets, error) {
	return events.CountFacets(ctx, events.FacetQuery{
		Pool:  r.pool,
		Store: afishaStore,
		From:  "FROM webreg_events e",
		Base:  afishaBase("$1"),
		Seed:  []any{since},
		Name:  r.AfishaSourceName(),
	}, f)
}

// AfishaSourceName — как источник называется в поле degraded ленты.
func (r *Repository) AfishaSourceName() string { return "webreg" }

func nonEmpty(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

func strPtr(s string) *string { return &s }
