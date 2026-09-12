package events

import (
	"context"
	"testing"
	"time"
)

func TestUnlistedCardOpensWithoutSearchIndexingSQL(t *testing.T) {
	p := boardPool(t)
	ctx := context.Background()
	_, err := p.Exec(ctx, `INSERT INTO events(id,title,start_time,status) VALUES
	 ('00000000-0000-0000-0000-000000000001','Link only','2026-10-01','published');
	 INSERT INTO organizer_event_details(event_id,visibility) VALUES
	 ('00000000-0000-0000-0000-000000000001','unlisted')`)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := NewRepository(p).GetByID(ctx, "00000000-0000-0000-0000-000000000001", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if ev.Indexable == nil || *ev.Indexable {
		t.Fatal("unlisted card must explicitly disable indexing")
	}
}
