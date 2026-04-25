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
