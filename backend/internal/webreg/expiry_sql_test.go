package webreg

import (
	"context"
	"github.com/pythonjsgo/vshage-afisha/internal/events"
	"testing"
	"time"
)

func TestSQLExpiredWebRegistrationExcludedFromListAndCounts(t *testing.T) {
	pool := sqlTestPool(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO webreg_events(slug,title,starts_at,ends_at,publish_afisha) VALUES
('yesterday','yesterday','2026-09-10 18:00+03',NULL,TRUE),
('ended','ended','2026-09-11 09:00+03','2026-09-11 10:00+03',TRUE),
('today','today','2026-09-11 09:00+03',NULL,TRUE),
('running','running','2026-09-10 18:00+03','2026-09-11 13:00+03',TRUE),
('future','future','2026-09-22 18:00+03',NULL,TRUE)`)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 11, 12, 0, 0, 0, time.FixedZone("MSK", 3*3600))
	f := events.Filter{City: events.DefaultCity(), At: at}
	repo := NewRepository(pool, "https://example.test")
	rows, err := repo.UpcomingForAfisha(ctx, at, f, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expired registration event included: %+v", rows)
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
