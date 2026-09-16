package events

import (
	"context"
	"testing"
	"time"
)

func TestDateOnlyNativeEventPresentationSQL(t *testing.T) {
	p := boardPool(t)
	ctx := context.Background()
	_, err := p.Exec(ctx, `
ALTER TABLE organizer_event_details ADD COLUMN start_time_known boolean, ADD COLUMN cover_fit text, ADD COLUMN organizer_url text;
INSERT INTO events(id,title,start_time,status,category) VALUES
 ('00000000-0000-0000-0000-000000000001','Minsk demo','2026-09-12 00:00+03','published','campus');
INSERT INTO organizer_event_details(event_id,city,visibility,start_time_known,cover_fit,organizer_url,registration_mode,external_registration_url) VALUES
 ('00000000-0000-0000-0000-000000000001','Минск','public',false,'contain','https://t.me/FAMCS_channel','auto','https://t.me/FAMCS_channel/756');`)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(p)
	// A future Minsk event is excluded by city, independently of expiry.
	before := time.Date(2026, 9, 11, 12, 0, 0, 0, mskZone)
	list, err := repo.List(ctx, ListQuery{Limit: 100, Filter: Filter{City: DefaultCity(), At: before}})
	if err != nil || list.Total != 0 {
		t.Fatalf("Minsk leaked into Moscow: %+v %v", list, err)
	}
	ev, err := repo.GetByID(ctx, "00000000-0000-0000-0000-000000000001", before.Add(72*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if ev.StartTimeKnown == nil || *ev.StartTimeKnown || ev.CoverFit == nil || *ev.CoverFit != "contain" || ev.City == nil || *ev.City != "Минск" {
		t.Fatalf("lost presentation facts: %+v", ev)
	}
	if ev.OrganizerURL == nil || *ev.OrganizerURL != "https://t.me/FAMCS_channel" || ev.SourceURL == nil || *ev.SourceURL != "https://t.me/FAMCS_channel/756" || ev.RegistrationMode == nil || *ev.RegistrationMode != "auto" {
		t.Fatalf("lost native registration or source links: %+v", ev)
	}
}
