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

// FieldToggle is one built-in field's setting: shown at all, and if shown,
// mandatory. Two booleans rather than a three-state enum because that is what
// the organizer is actually deciding — «спрашивать?» and «без этого не
// пускать?» — and a tri-state collapses those into one control that reads
// ambiguously in a settings list.
type FieldToggle struct {
	Enabled  bool `json:"enabled"`
	Required bool `json:"required"`
}

// FormConfig is the built-in identity block of the signup form. Everything
// beyond it is an organizer-defined Field.
//
// Version exists to tell "configured as all-off" apart from "written before
// this feature existed". A row from migration 003 decodes to the zero value,
// where every toggle is off — a form with no fields at all. LegacyForm()
// supplies the old hardcoded shape for those; anything the admin writes
// carries Version >= 1 and is taken literally.
type FormConfig struct {
	Version     int         `json:"v"`
	Name        FieldToggle `json:"name"`
	FullName    FieldToggle `json:"full_name"`
	Email       FieldToggle `json:"email"`
	Phone       FieldToggle `json:"phone"`
	TG          FieldToggle `json:"tg"`
	Affiliation FieldToggle `json:"affiliation"`
	// PassNote explains why the passport-form name is being asked for
	// («нужно для пропуска в БЦ»). Shown under the ФИО field. Asking for a
	// document name without saying why is the single biggest reason a
	// visitor abandons the form.
	PassNote string `json:"pass_note,omitempty"`
}

const formVersion = 1

// LegacyForm is the hardcoded shape every event had before 004: name,
// Telegram and вуз/статус, all three mandatory, no email. Applied to rows
// whose form config predates the feature so a live event's form does not
// change shape under the people filling it in.
func LegacyForm() FormConfig {
	on := FieldToggle{Enabled: true, Required: true}
	return FormConfig{Version: formVersion, Name: on, TG: on, Affiliation: on}
}

// DefaultForm is what a newly created event starts with: a name and a working
// email address, because the email is what carries the ticket and every later
// reminder. Telegram is asked for but not demanded — a missing username is a
// worse reason to lose an attendee than a missing chat invite.
func DefaultForm() FormConfig {
	return FormConfig{
		Version: formVersion,
		Name:    FieldToggle{Enabled: true, Required: true},
		Email:   FieldToggle{Enabled: true, Required: true},
		TG:      FieldToggle{Enabled: true},
	}
}

// identity returns the field whose value deduplicates registrations for this
// event, most stable first. Email wins when collected: a person can change
// their Telegram username, and a second signup would otherwise land as a
// second row on the door list.
func (f FormConfig) identity() string {
	switch {
	case f.Email.Enabled:
		return "email"
	case f.TG.Enabled:
		return "tg"
	case f.Phone.Enabled:
		return "phone"
	default:
		return ""
	}
}

// Ticket modes.
const (
	TicketQR   = "qr"   // QR code on screen + in the email
	TicketCode = "code" // short code only, no QR
	TicketOff  = "off"  // no entry pass at all
)

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
	// VshageURL is where «Открыть во Вшаге» points. Empty until the app has a
	// screen for a web-registration event; the page then falls back to the
	// install link rather than rendering a button that goes nowhere.
	VshageURL string `json:"vshage_url,omitempty"`
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
	Form             FormConfig `json:"form"`
	Fields           []Field    `json:"fields"`
	Affiliations     []string   `json:"affiliations"`
	Bridge           Bridge     `json:"bridge"`
	OrganizerTitle   string     `json:"organizer_title,omitempty"`
	Capacity         *int       `json:"capacity,omitempty"`
	RegistrationOpen bool       `json:"registration_open"`
	PublishAfisha    bool       `json:"publish_afisha"`
	PublishVshage    bool       `json:"publish_vshage"`
	TicketMode       string     `json:"ticket_mode"`
	RegisteredCount  int        `json:"registered_count"`
	// SeatsLeft is nil when the event has no capacity limit.
	SeatsLeft *int `json:"seats_left,omitempty"`
}

// RegisterInput is the signup payload. Custom answers are keyed by Field.Key.
type RegisterInput struct {
	Name        string            `json:"name"`
	FullName    string            `json:"full_name"`
	Email       string            `json:"email"`
	Phone       string            `json:"phone"`
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
	// TicketCode is empty when the event issues no entry pass.
	TicketCode string `json:"ticket_code,omitempty"`
}

// Registration is one row as the organizer sees it.
type Registration struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	FullName    string            `json:"full_name,omitempty"`
	Email       string            `json:"email,omitempty"`
	Phone       string            `json:"phone,omitempty"`
	TGUsername  string            `json:"tg_username,omitempty"`
	TGDisplay   string            `json:"tg_display,omitempty"`
	Affiliation string            `json:"affiliation,omitempty"`
	Answers     map[string]string `json:"answers"`
	TicketCode  string            `json:"ticket_code,omitempty"`
	CheckedInAt *time.Time        `json:"checked_in_at,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

// ManageList is the payload behind /e/<slug>/manage?key=…
type ManageList struct {
	Slug          string         `json:"slug"`
	Title         string         `json:"title"`
	StartsAt      time.Time      `json:"starts_at"`
	Timezone      string         `json:"timezone"`
	Form          FormConfig     `json:"form"`
	Fields        []Field        `json:"fields"`
	TicketMode    string         `json:"ticket_mode"`
	Capacity      *int           `json:"capacity,omitempty"`
	Total         int            `json:"total"`
	CheckedIn     int            `json:"checked_in"`
	Registrations []Registration `json:"registrations"`
}

// EventSummary is one row of the admin event index.
type EventSummary struct {
	Slug             string    `json:"slug"`
	Title            string    `json:"title"`
	StartsAt         time.Time `json:"starts_at"`
	Timezone         string    `json:"timezone"`
	RegistrationOpen bool      `json:"registration_open"`
	PublishAfisha    bool      `json:"publish_afisha"`
	PublishVshage    bool      `json:"publish_vshage"`
	Capacity         *int      `json:"capacity,omitempty"`
	Registered       int       `json:"registered"`
	CheckedIn        int       `json:"checked_in"`
}

// Ticket is the entry pass as its holder sees it at /e/<slug>/t/<code>.
type Ticket struct {
	EventSlug   string     `json:"event_slug"`
	EventTitle  string     `json:"event_title"`
	StartsAt    time.Time  `json:"starts_at"`
	Timezone    string     `json:"timezone"`
	VenueName   string     `json:"venue_name,omitempty"`
	VenueAddr   string     `json:"venue_address,omitempty"`
	Name        string     `json:"name"`
	FullName    string     `json:"full_name,omitempty"`
	Code        string     `json:"code"`
	CheckedInAt *time.Time `json:"checked_in_at,omitempty"`
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
	Slug             string      `json:"slug"`
	Title            string      `json:"title"`
	Tagline          string      `json:"tagline"`
	Description      string      `json:"description"`
	CoverURL         string      `json:"cover_url"`
	StartsAt         time.Time   `json:"starts_at"`
	EndsAt           *time.Time  `json:"ends_at"`
	Timezone         string      `json:"timezone"`
	Venue            VenueCard   `json:"venue"`
	Form             *FormConfig `json:"form"`
	Fields           []Field     `json:"fields"`
	Affiliations     []string    `json:"affiliations"`
	Bridge           Bridge      `json:"bridge"`
	OrganizerTitle   string      `json:"organizer_title"`
	Capacity         *int        `json:"capacity"`
	RegistrationOpen *bool       `json:"registration_open"`
	PublishAfisha    *bool       `json:"publish_afisha"`
	PublishVshage    *bool       `json:"publish_vshage"`
	TicketMode       string      `json:"ticket_mode"`
	ManageKey        string      `json:"manage_key"`
}
