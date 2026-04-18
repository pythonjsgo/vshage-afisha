package events

import (
	"encoding/json"
	"time"
)

type PublicEvent struct {
	ID               string          `json:"id"`
	Title            string          `json:"title"`
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
	IsFeatured       bool            `json:"is_featured"`
	FeaturedPosition *int            `json:"featured_position,omitempty"`
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
