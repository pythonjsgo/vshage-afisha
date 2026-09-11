package tgevents

import (
	"context"
	"github.com/pythonjsgo/vshage-afisha/internal/events"
	"testing"
	"time"
)

func TestSQLExpiredCalendarEventsExcludedFromListAndCounts(t *testing.T) {
	pool := sqlTestPool(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO afisha_tg_events(id,title,annonce,date,date_end,city,category) VALUES
('ev_yesterday','yesterday','text','2026-09-10',NULL,'Москва','exhibition'),
('ev_finished','finished exhibition','text','2026-09-01','2026-09-10','Москва','exhibition'),
('ev_today','today','text','2026-09-11',NULL,'Москва','exhibition'),
('ev_running','running exhibition','text','2026-09-01','2026-09-12','Москва','exhibition'),
('ev_future','future','text','2026-09-22',NULL,'Москва','exhibition')`)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 11, 0, 1, 0, 0, msk)
	f := events.Filter{City: events.DefaultCity(), At: at}
	repo := NewRepository(pool)
	rows, err := repo.UpcomingForAfisha(ctx, at, f, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expired calendar event included: %+v", rows)
	}
	for _, row := range rows {
		if row.ID == "ev_yesterday" || row.ID == "ev_finished" {
			t.Fatal("yesterday survived")
		}
	}
	n, err := repo.CountUpcomingForAfisha(ctx, at, f)
	if err != nil || n != 3 {
		t.Fatalf("count=%d err=%v", n, err)
	}
	counts, err := repo.FacetsForAfisha(ctx, at, f)
	if err != nil || counts.Total != 3 {
		t.Fatalf("facets=%+v err=%v", counts, err)
	}
}
