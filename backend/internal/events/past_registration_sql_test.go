package events

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPastRegistrationAdminOverrideSQL(t *testing.T) {
	pool := boardPool(t)
	ctx := context.Background()
	const id = "00000000-0000-0000-0000-000000000001"
	exec := func(query string) {
		t.Helper()
		if _, err := pool.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO events(id,title,start_time,status) VALUES ('` + id + `','Past demo',now()-interval '2 days','published');
      INSERT INTO organizer_event_details(event_id,visibility,registration_mode) VALUES ('` + id + `','unlisted','auto');`)
	repo := NewRepository(pool)
	wantCode := func(code string) {
		t.Helper()
		_, err := repo.RegisterPublic(ctx, id, PublicRegistrationInput{})
		var registrationErr *RegistrationError
		if !errors.As(err, &registrationErr) || registrationErr.Code != code {
			t.Fatalf("want %s, got %v", code, err)
		}
	}
	// Both the old schema and a missing opt-in preserve the cutoff.
	wantCode("registration_closed")
	exec(`ALTER TABLE organizer_event_details ADD COLUMN allow_past_registration boolean;`)
	wantCode("registration_closed")
	exec(`UPDATE organizer_event_details SET allow_past_registration=true;`)
	wantCode("invalid_name") // Reaches form validation without creating a participant.
	card, err := repo.GetByID(ctx, id, time.Now())
	if err != nil || !card.AllowPastRegistration {
		t.Fatalf("override missing from card: %v %+v", err, card)
	}
	// The opt-in cannot bypass any other registration gate.
	exec(`UPDATE organizer_event_details SET registration_deadline=now()-interval '1 hour';`)
	wantCode("registration_closed")
	exec(`UPDATE organizer_event_details SET registration_deadline=NULL; UPDATE events SET max_attendees=1;
      INSERT INTO event_registrations(event_id,status) VALUES ('` + id + `','registered');`)
	wantCode("sold_out")
	exec(`UPDATE organizer_event_details SET visibility='hidden';`)
	wantCode("event_not_found")
	exec(`UPDATE organizer_event_details SET visibility='unlisted',registration_mode='external';`)
	wantCode("external_registration")
	exec(`UPDATE events SET status='cancelled';`)
	wantCode("registration_unavailable")
}
