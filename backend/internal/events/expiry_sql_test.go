package events

import (
	"context"
	"testing"
	"time"
)

func TestSQLExpiredEventsExcludedBeforePinsAndCounts(t *testing.T) {
	pool := boardPool(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
INSERT INTO events(id,title,start_time,end_time,status,category,tags)
SELECT ('00000000-0000-0000-0000-'||LPAD(n::text,12,'0'))::uuid,title,s::timestamptz,e::timestamptz,'published','networking','[]'::jsonb
FROM (VALUES
(1,'yesterday pin','2026-09-10 18:00+03',NULL),
(2,'ended today','2026-09-11 09:00+03','2026-09-11 10:00+03'),
(3,'ends exactly now','2026-09-11 09:00+03','2026-09-11 12:00+03'),
(4,'still running','2026-09-10 18:00+03','2026-09-11 13:00+03'),
(5,'today unknown end','2026-09-11 09:00+03',NULL),
(6,'future pin','2026-09-18 18:00+03',NULL)) AS v(n,title,s,e);
INSERT INTO organizer_event_details(event_id,city,visibility) SELECT id,'Москва','public' FROM events;
INSERT INTO afisha_featured SELECT id,-200 FROM events WHERE title='yesterday pin';
INSERT INTO afisha_featured SELECT id,-100 FROM events WHERE title='future pin';`)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, mskZone)
	f := Filter{City: DefaultCity(), At: now}
	repo := NewRepository(pool)
	got, err := repo.List(ctx, ListQuery{Limit: 100, Filter: f})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 3 || len(got.All) != 3 || len(got.Featured) != 1 || got.All[0].Title != "future pin" || got.Featured[0].Title != "future pin" {
		t.Fatalf("expired pin or event survived: %+v", got)
	}
	for _, e := range got.All {
		if e.Title != "future pin" && e.Title != "still running" && e.Title != "today unknown end" {
			t.Fatalf("unexpected %s", e.Title)
		}
	}
	counts, err := repo.Facets(ctx, time.Time{}, f)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 3 {
		t.Fatalf("facets still count ended events: %+v", counts)
	}
	old, err := repo.GetByID(ctx, "00000000-0000-0000-0000-000000000001", now)
	if err != nil || old == nil || old.Title != "yesterday pin" {
		t.Fatalf("past event link was lost: %v", err)
	}
	// Moscow day changes at 21:00 UTC, independent of the database timezone.
	for _, tc := range []struct {
		at   string
		want bool
	}{{"2026-09-10T20:59:00Z", true}, {"2026-09-10T21:01:00Z", false}} {
		at, _ := time.Parse(time.RFC3339, tc.at)
		var active bool
		err = pool.QueryRow(ctx, "SELECT "+NotEndedSQL("start_time", "end_time", "$1")+" FROM events WHERE title='yesterday pin'", at).Scan(&active)
		if err != nil || active != tc.want {
			t.Fatalf("Moscow midnight %s: active=%v error=%v", tc.at, active, err)
		}
	}
}
