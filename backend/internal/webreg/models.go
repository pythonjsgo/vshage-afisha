// Package webreg serves the standalone web registration flow behind
// vshage.app/e/<slug>: a public event page, a no-account signup form, and a
// secret-link attendee list for the organizer.
//
// It is intentionally self-contained — see pkg/db/migrations/003_webreg.sql
// for why it does not reuse the shared events/event_registrations tables.
package webreg

import "time"

// VenueCard is the denormalised place card shown on the event page. It is
// copied out of the geo catalog when the event is created (scripts/venue-card.mjs),
// never fetched at request time: an announcement burst must not depend on a
// second service, and the catalog file is 11MB.
//
// Degrades in this order: full card (Name set) → address only (Address set) →
// online (OnlineURL set) → block hidden entirely.
type VenueCard struct {
	VID         string   `json:"vid,omitempty"`
	Name        string   `json:"name,omitempty"`
	Address     string   `json:"address,omitempty"`
	District    string   `json:"district,omitempty"`
	Lat         float64  `json:"lat,omitempty"`
	Lon         float64  `json:"lon,omitempty"`
	RatingAvg   *float64 `json:"rating_avg,omitempty"`
	RatingCount int      `json:"rating_count,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	MapsURL     string   `json:"maps_url,omitempty"`
	Note        string   `json:"note,omitempty"`
	OnlineURL   string   `json:"online_url,omitempty"`
}

// Field is one organizer-defined question. Types: "select", "text",
// "textarea", "checkbox".
type Field struct {
	Key        string   `json:"key"`
	Label      string   `json:"label"`
	Type       string   `json:"type"`
	Required   bool     `json:"required,omitempty"`
	Options    []string `json:"options,omitempty"`
	AllowOther bool     `json:"allow_other,omitempty"`
	Hint       string   `json:"hint,omitempty"`
	MaxLen     int      `json:"max_len,omitempty"`
}

// Bridge configures the post-registration screen — the hand-off from a web
// signup into the network.
type Bridge struct {
	// "testflight" | "app_store" | "waitlist" | "off"
	IOSMode         string `json:"ios_mode,omitempty"`
	TestFlightURL   string `json:"testflight_url,omitempty"`
	AppStoreURL     string `json:"app_store_url,omitempty"`
	InviteCode      string `json:"invite_code,omitempty"`
	AndroidWaitlist bool   `json:"android_waitlist,omitempty"`
	TGChannelURL    string `json:"tg_channel_url,omitempty"`
	TGChatURL       string `json:"tg_chat_url,omitempty"`
	PrivacyURL      string `json:"privacy_url,omitempty"`
	InstallURL      string `json:"install_url,omitempty"`
}

// Event is the public shape of an event page. manage_key_hash never leaves
// the database layer — it has no field here on purpose.
type Event struct {
	Slug             string     `json:"slug"`
	Title            string     `json:"title"`
	Tagline          string     `json:"tagline,omitempty"`
	Description      string     `json:"description,omitempty"`
	CoverURL         string     `json:"cover_url,omitempty"`
	StartsAt         time.Time  `json:"starts_at"`
	EndsAt           *time.Time `json:"ends_at,omitempty"`
	Timezone         string     `json:"timezone"`
	Venue            VenueCard  `json:"venue"`
	Fields           []Field    `json:"fields"`
	Affiliations     []string   `json:"affiliations"`
	Bridge           Bridge     `json:"bridge"`
	OrganizerTitle   string     `json:"organizer_title,omitempty"`
	Capacity         *int       `json:"capacity,omitempty"`
	RegistrationOpen bool       `json:"registration_open"`
	RegisteredCount  int        `json:"registered_count"`
	// SeatsLeft is nil when the event has no capacity limit.
	SeatsLeft *int `json:"seats_left,omitempty"`
}

// RegisterInput is the signup payload. Custom answers are keyed by Field.Key.
type RegisterInput struct {
	Name        string            `json:"name"`
	TGUsername  string            `json:"tg_username"`
	Affiliation string            `json:"affiliation"`
	Answers     map[string]string `json:"answers"`
	Consent     bool              `json:"consent"`
	Source      string            `json:"source,omitempty"`
}

type RegisterResult struct {
	ID                int64  `json:"id"`
	EventSlug         string `json:"event_slug"`
	Status            string `json:"status"`
	AlreadyRegistered bool   `json:"already_registered,omitempty"`
	// Position is 1-based arrival order, shown as «ты N-й» on the done screen.
	Position int `json:"position"`
}

// Registration is one row as the organizer sees it.
type Registration struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	TGUsername  string            `json:"tg_username"`
	TGDisplay   string            `json:"tg_display"`
	Affiliation string            `json:"affiliation"`
	Answers     map[string]string `json:"answers"`
	CreatedAt   time.Time         `json:"created_at"`
}

// ManageList is the payload behind /e/<slug>/manage?key=…
type ManageList struct {
	Slug          string         `json:"slug"`
	Title         string         `json:"title"`
	StartsAt      time.Time      `json:"starts_at"`
	Timezone      string         `json:"timezone"`
	Fields        []Field        `json:"fields"`
	Capacity      *int           `json:"capacity,omitempty"`
	Total         int            `json:"total"`
	Registrations []Registration `json:"registrations"`
}

// WaitlistInput is the Android «встань в лист ожидания» payload.
type WaitlistInput struct {
	Platform   string `json:"platform"`
	TGUsername string `json:"tg_username"`
	Name       string `json:"name,omitempty"`
}

// APIError is the single error shape every webreg endpoint returns. Code is
// machine-readable; Message is shown to the visitor as-is (Russian).
type APIError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
	Status  int               `json:"-"`
}

func (e *APIError) Error() string { return e.Message }

// EventUpsert is the admin-side event config (the "конфиг руками"). It
// carries the plaintext manage key; only its hash is persisted.
type EventUpsert struct {
	Slug             string     `json:"slug"`
	Title            string     `json:"title"`
	Tagline          string     `json:"tagline"`
	Description      string     `json:"description"`
	CoverURL         string     `json:"cover_url"`
	StartsAt         time.Time  `json:"starts_at"`
	EndsAt           *time.Time `json:"ends_at"`
	Timezone         string     `json:"timezone"`
	Venue            VenueCard  `json:"venue"`
	Fields           []Field    `json:"fields"`
	Affiliations     []string   `json:"affiliations"`
	Bridge           Bridge     `json:"bridge"`
	OrganizerTitle   string     `json:"organizer_title"`
	Capacity         *int       `json:"capacity"`
	RegistrationOpen *bool      `json:"registration_open"`
	ManageKey        string     `json:"manage_key"`
}
