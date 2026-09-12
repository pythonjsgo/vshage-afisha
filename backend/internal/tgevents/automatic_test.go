package tgevents

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func automaticPool(t *testing.T) *pgxpool.Pool {
	p := sqlTestPool(t)
	_, err := p.Exec(context.Background(), `ALTER TABLE afisha_tg_events
		ADD feed boolean NOT NULL DEFAULT false, ADD anchor boolean NOT NULL DEFAULT false,
		ADD featured boolean NOT NULL DEFAULT false, ADD featured_until timestamptz,
		ADD hide_reason text, ADD hidden_by text, ADD hidden_at timestamptz,
		ADD cover_mime text, ADD source_key text;
		DROP TABLE IF EXISTS afisha_curation_log;
		CREATE TABLE afisha_curation_log(event_id text, actor text, action text, reason text, changes jsonb);`)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAutomaticDecisionsSQL(t *testing.T) {
	p := automaticPool(t)
	ctx := context.Background()
	repo := NewRepository(p)
	_, err := p.Exec(ctx, `INSERT INTO afisha_tg_events(id,title,annonce,date,hidden,listed,feed,hide_reason,hidden_by) VALUES
	 ('queue','Queue','Text','2026-10-01',true,true,false,NULL,NULL),
	 ('live','Live','Text','2026-10-01',false,true,true,NULL,NULL),
	 ('manual','Manual','Text','2026-10-01',true,true,false,'duplicate','curator:founder'),
	 ('unlisted','Unlisted','Text','2026-10-01',false,false,false,NULL,NULL),
	 ('auto','Auto','Text','2026-10-01',false,false,false,'auto:children','pipeline:auto-events')`)
	if err != nil {
		t.Fatal(err)
	}
	decisions := []AutomaticDecision{{"queue", true, "eligible"}, {"live", false, "children"},
		{"manual", true, "eligible"}, {"unlisted", true, "eligible"}, {"auto", true, "eligible"}, {"missing", true, "eligible"}}
	out, err := repo.AutomaticDecisions(ctx, decisions)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Published) != 2 || len(out.Excluded) != 1 || len(out.Protected) != 2 || len(out.Missing) != 1 {
		t.Fatalf("%+v", out)
	}
	var logs int
	_ = p.QueryRow(ctx, "SELECT count(*) FROM afisha_curation_log").Scan(&logs)
	if logs != 3 {
		t.Fatalf("logs=%d", logs)
	}
	var before time.Time
	_ = p.QueryRow(ctx, "SELECT updated_at FROM afisha_tg_events WHERE id='queue'").Scan(&before)
	out, err = repo.AutomaticDecisions(ctx, decisions)
	if err != nil {
		t.Fatal(err)
	}
	var after time.Time
	_ = p.QueryRow(ctx, "SELECT updated_at FROM afisha_tg_events WHERE id='queue'").Scan(&after)
	if !before.Equal(after) || len(out.Unchanged) != 3 {
		t.Fatalf("retry mutated data: %+v", out)
	}
	if _, err = repo.GetByID(ctx, "live", time.Now()); err != nil {
		t.Fatalf("existing direct link lost: %v", err)
	}
	_, _ = p.Exec(ctx, "DROP TABLE afisha_curation_log")
	if _, err = repo.AutomaticDecisions(ctx, []AutomaticDecision{{"queue", false, "ad"}}); err == nil {
		t.Fatal("missing audit log must fail")
	}
	var feed bool
	_ = p.QueryRow(ctx, "SELECT feed FROM afisha_tg_events WHERE id='queue'").Scan(&feed)
	if !feed {
		t.Fatal("failed audit transaction changed publication")
	}
}

func TestImportLastmodSQL(t *testing.T) {
	p := automaticPool(t)
	ctx := context.Background()
	repo := NewRepository(p)
	source := "https://example.test/concert"
	c := Card{ID: "ev_123456789abc", Title: "Concert", Annonce: "Live jazz", Date: "2026-10-01", AccessLevel: "open", SourceURL: &source, Payload: map[string]any{"extracted_at": "first"}}
	if _, err := repo.UpsertBulk(ctx, []Card{c}); err != nil {
		t.Fatal(err)
	}
	read := func() time.Time {
		var at time.Time
		if err := p.QueryRow(ctx, "SELECT updated_at FROM afisha_tg_events WHERE id=$1", c.ID).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}
	before := read()
	c.Payload["extracted_at"] = "next crawl"
	if _, err := repo.UpsertBulk(ctx, []Card{c}); err != nil {
		t.Fatal(err)
	}
	if !read().Equal(before) {
		t.Fatal("crawl-only change faked lastmod")
	}
	c.Title = "Updated concert"
	if _, err := repo.UpsertBulk(ctx, []Card{c}); err != nil {
		t.Fatal(err)
	}
	if !read().After(before) {
		t.Fatal("real title change did not update lastmod")
	}
	before = read()
	c.Payload["seo"] = map[string]any{"description": "Jazz in Moscow"}
	if _, err := repo.UpsertBulk(ctx, []Card{c}); err != nil {
		t.Fatal(err)
	}
	if !read().After(before) {
		t.Fatal("SEO change did not update lastmod")
	}
	before = read()
	c.Payload["time_end"] = "21:00"
	if _, err := repo.UpsertBulk(ctx, []Card{c}); err != nil {
		t.Fatal(err)
	}
	if !read().After(before) {
		t.Fatal("structured fact change did not update lastmod")
	}
	before = read()
	if _, err := repo.UpsertBulk(ctx, []Card{c}); err != nil {
		t.Fatal(err)
	}
	if !read().Equal(before) {
		t.Fatal("identical structured facts changed lastmod")
	}
	ev, err := repo.GetByID(ctx, c.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if ev.SEODescription == nil || *ev.SEODescription != "Jazz in Moscow" || ev.UpdatedAt == nil {
		t.Fatalf("SEO metadata not exposed: %+v", ev)
	}
}

func TestHumanCanTakeOwnershipOfAutomaticExclusionSQL(t *testing.T) {
	p := automaticPool(t)
	ctx := context.Background()
	repo := NewRepository(p)
	_, err := p.Exec(ctx, `INSERT INTO afisha_tg_events(id,title,annonce,date,listed,hide_reason,hidden_by)
	 VALUES ('auto','Test','Text','2026-10-01',false,'auto:children','pipeline:auto-events')`)
	if err != nil {
		t.Fatal(err)
	}
	no := false
	if _, err = repo.AdminSetFlags(ctx, "auto", AdminFlags{Listed: &no, Actor: "founder"}); err != nil {
		t.Fatal(err)
	}
	out, err := repo.AutomaticDecisions(ctx, []AutomaticDecision{{"auto", true, "eligible"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Protected) != 1 {
		t.Fatalf("human hide was reversed: %+v", out)
	}
}

func TestAutomaticEndpointRequiresAuthenticationAndValidBatch(t *testing.T) {
	h := NewHandler(nil, "secret")
	for _, test := range []struct {
		token, body string
		want        int
	}{
		{"", `{"decisions":[]}`, 401},
		{"secret", `{"decisions":[]}`, 400},
		{"secret", `{"decisions":[{"id":"ev_abc123","publish":true,"reason":"eligible"},{"id":"ev_abc123","publish":false,"reason":"ad"}]}`, 400},
		{"secret", `{"decisions":[{"id":"ev_abc123","publish":true}]}`, 400},
		{"secret", `{"decisions":[{"id":"ev_abc123","reason":"eligible"}]}`, 400},
	} {
		r := httptest.NewRequest("POST", "/api/tg-events/admin/decisions", strings.NewReader(test.body))
		r.Header.Set("X-Admin-Token", test.token)
		w := httptest.NewRecorder()
		h.AdminAutomatic(w, r)
		if w.Code != test.want {
			t.Fatalf("got %d want %d: %s", w.Code, test.want, w.Body.String())
		}
	}
}
