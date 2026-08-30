package events

import (
	"encoding/json"
	"time"
)

type PublicEvent struct {
	ID               string          `json:"id"`
	Title            string          `json:"title"`
	ShortDescription *string         `json:"short_description,omitempty"`
	Description      *string         `json:"description,omitempty"`
	Location         *string         `json:"location,omitempty"`
	StartTime        time.Time       `json:"start_time"`
	EndTime          *time.Time      `json:"end_time,omitempty"`
	Category         *string         `json:"category,omitempty"`
	Tags             json.RawMessage `json:"tags"`
	MaxAttendees     *int            `json:"max_attendees,omitempty"`
	AttendeeCount    int             `json:"attendee_count"`
	PhotoURL         *string         `json:"photo_url,omitempty"`
	Status           string          `json:"status"`
	RegistrationMode *string         `json:"registration_mode,omitempty"`
	ExternalRegURL   *string         `json:"external_registration_url,omitempty"`
	RegDeadline      *time.Time      `json:"registration_deadline,omitempty"`
	PriceType        *string         `json:"price_type,omitempty"`
	PriceMin         *int            `json:"price_min,omitempty"`
	PriceMax         *int            `json:"price_max,omitempty"`
	Currency         *string         `json:"currency,omitempty"`
	City             *string         `json:"city,omitempty"`
	VenueName        *string         `json:"venue_name,omitempty"`
	Address          *string         `json:"address,omitempty"`
	OnlineURL        *string         `json:"online_url,omitempty"`
	AgeLimit         *string         `json:"age_limit,omitempty"`
	AttendeesNote    *string         `json:"attendees_note,omitempty"`
	IsFeatured       bool            `json:"is_featured"`
	FeaturedPosition *int            `json:"featured_position,omitempty"`
	OrganizerName    *string         `json:"organizer_name,omitempty"`
	OrganizerPhoto   *string         `json:"organizer_photo,omitempty"`
	Photos           []string        `json:"photos"`
	// WebregSlug помечает событие, живущее в собственных таблицах
	// веб-регистрации (см. internal/webreg). Фронт по нему строит ссылку
	// на /e/<slug> вместо /<uuid>.
	WebregSlug string `json:"webreg_slug,omitempty"`
	// SourceURL — ссылка на первоисточник анонса (см. internal/tgevents).
	// Для импортированных событий это не украшение, а условие, на котором
	// мы их вообще показываем: наш текст плюс подписанный источник.
	SourceURL *string `json:"source_url,omitempty"`
	// AccessLevel — «open» / «university» / «invite» у импортированных
	// событий. Человеку важно до клика понимать, пустят ли его.
	AccessLevel *string `json:"access_level,omitempty"`
	// Source — откуда событие: пусто у наших, "tg" у импортированных.
	// ЯВНЫЙ дискриминатор, а не вывод из наличия source_url: тот nullable,
	// и карточка без него превращалась бы в «наше» событие с нашей кнопкой
	// записи на чужое мероприятие.
	Source *string `json:"source,omitempty"`
	// StartTimeKnown=false означает «дата известна, времени нет». Без этого
	// признака полночь неотличима от настоящего начала в 00:00, а фронт
	// печатает её как время события.
	StartTimeKnown *bool `json:"start_time_known,omitempty"`
	// VenueLat / VenueLon / VenueMetro — гео места из кураторского venue
	// (см. internal/tgevents, миграция 008). Опциональны и у большинства
	// событий отсутствуют: курация проставляет их вручную, и потребитель
	// обязан переживать их отсутствие, а не считать нулём.
	VenueLat   *float64 `json:"venue_lat,omitempty"`
	VenueLon   *float64 `json:"venue_lon,omitempty"`
	VenueMetro *string  `json:"venue_metro,omitempty"`
}

type ListQuery struct {
	OnlyFeatured bool
	Limit        int
	Offset       int
	Since        time.Time
}

type ListResult struct {
	Featured []PublicEvent `json:"featured"`
	All      []PublicEvent `json:"all"`
	Total    int           `json:"total"`
	// Degraded перечисляет источники ленты, которые не ответили. Пустой
	// список — все живы. Без этого поля отказ источника выглядит как «в нём
	// ничего нет»: ответ 200, лента непустая, просто без половины событий, и
	// отличить одно от другого нечем — логи PROD в Loki не доезжают.
	Degraded []string `json:"degraded,omitempty"`
}

type PublicRegistrationInput struct {
	Name    string `json:"name"`
	Contact string `json:"contact"`
}

type PublicRegistrationResult struct {
	RegistrationID    string `json:"registration_id"`
	EventID           string `json:"event_id"`
	Status            string `json:"status"`
	AlreadyRegistered bool   `json:"already_registered,omitempty"`
}

type RegistrationError struct {
	Code        string  `json:"code"`
	Message     string  `json:"message"`
	Status      int     `json:"-"`
	ExternalURL *string `json:"external_registration_url,omitempty"`
}

func (e *RegistrationError) Error() string {
	return e.Message
}
