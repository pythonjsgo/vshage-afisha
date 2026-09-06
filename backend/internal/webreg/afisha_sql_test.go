package webreg

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pythonjsgo/vshage-afisha/internal/events"
)

// Прогон против настоящего постгреса. У веб-регистрации нет ни города, ни
// категории, и условия фильтра поэтому вырождаются в TRUE/FALSE — ровно тот
// случай, когда «да тут же нечему ломаться» и проверять никто не идёт. Ломается
// при этом синтаксис запроса целиком, а события веб-регистрации живые: это
// доска, на которую садится наша же регистрация ШАГа.
//
//	AFISHA_TEST_DATABASE_URL=postgres://…/afisha_z1 go test ./internal/webreg/ -run SQL
const sqlTestSchema = "webreg_filter_test"

func sqlTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("AFISHA_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("AFISHA_TEST_DATABASE_URL не задан — прогон против настоящего постгреса пропущен")
	}
	ctx := context.Background()
	boot, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("подключение: %v", err)
	}
	if _, err := boot.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+sqlTestSchema); err != nil {
		boot.Close()
		t.Fatalf("создание схемы: %v", err)
	}
	boot.Close()

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("разбор строки подключения: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = sqlTestSchema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("подключение к схеме: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, `
		DROP TABLE IF EXISTS webreg_registrations, webreg_events;
		CREATE TABLE webreg_events (
			slug            TEXT PRIMARY KEY,
			title           TEXT NOT NULL,
			tagline         TEXT,
			description     TEXT,
			cover_url       TEXT,
			starts_at       TIMESTAMPTZ NOT NULL,
			ends_at         TIMESTAMPTZ,
			venue           JSONB NOT NULL DEFAULT '{}'::jsonb,
			organizer_title TEXT,
			capacity        INTEGER,
			publish_afisha  BOOLEAN NOT NULL DEFAULT FALSE
		);
		CREATE TABLE webreg_registrations (id BIGSERIAL PRIMARY KEY, event_slug TEXT NOT NULL)`); err != nil {
		t.Fatalf("создание таблиц: %v", err)
	}
	return pool
}

func sqlSeed(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	rows := []struct {
		slug    string
		start   string
		end     *string
		publish bool
	}{
		{"w-today", "2026-09-07 12:00", nil, true},
		// Многодневная программа: идёт и сегодня, и завтра, и на выходных.
		{"w-week", "2026-09-07 10:00", strptr("2026-09-13 20:00"), true},
		{"w-tomorrow", "2026-09-08 12:00", nil, true},
		{"w-unpublished", "2026-09-07 18:00", nil, false},
		{"w-past", "2026-09-01 12:00", nil, true},
	}
	for _, r := range rows {
		var end any
		if r.end != nil {
			end = *r.end + " MSK"
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO webreg_events (slug, title, starts_at, ends_at, publish_afisha)
			VALUES ($1, $2, $3::timestamptz, $4::timestamptz, $5)`,
			r.slug, "Событие "+r.slug, r.start+" MSK", end, r.publish); err != nil {
			t.Fatalf("вставка %s: %v", r.slug, err)
		}
	}
}

func strptr(s string) *string { return &s }

func slugs(list []events.PublicEvent) []string {
	out := make([]string, 0, len(list))
	for _, e := range list {
		out = append(out, e.WebregSlug)
	}
	sort.Strings(out)
	return out
}

func same(got []string, want ...string) bool {
	sort.Strings(want)
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestWebregFilterSQLAgainstPostgres(t *testing.T) {
	pool := sqlTestPool(t)
	sqlSeed(t, pool)
	repo := NewRepository(pool, "salt")
	ctx := context.Background()

	msk := time.FixedZone("MSK", 3*60*60)
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, msk)
	since := time.Date(2026, 9, 6, 12, 0, 0, 0, msk)
	base := func(mod func(*events.Filter)) events.Filter {
		f := events.Filter{City: events.DefaultCity(), At: now}
		if mod != nil {
			mod(&f)
		}
		return f
	}
	spb := events.City{Slug: "spb", Name: "Санкт-Петербург"}

	cases := []struct {
		name string
		f    events.Filter
		want []string
	}{
		{"вся доска", base(nil), []string{"w-today", "w-week", "w-tomorrow"}},
		{"сегодня", base(func(f *events.Filter) { f.When = events.WhenToday }), []string{"w-today", "w-week"}},
		{"завтра", base(func(f *events.Filter) { f.When = events.WhenTomorrow }), []string{"w-week", "w-tomorrow"}},
		{"выходные", base(func(f *events.Filter) { f.When = events.WhenWeekend }), []string{"w-week"}},
		// Все события веб-регистрации бесплатны по устройству страницы.
		{"бесплатно", base(func(f *events.Filter) { f.Free = true }), []string{"w-today", "w-week", "w-tomorrow"}},
		// Категории у стора нет вовсе — значит ни в один раздел он не попадает.
		{"раздел «концерты»", base(func(f *events.Filter) { f.Category = "concert" }), nil},
		// И на доску другого города тоже: события заводит наша команда, и они
		// московские.
		{"другой город", base(func(f *events.Filter) { f.City = spb }), nil},
	}

	for _, c := range cases {
		list, err := repo.UpcomingForAfisha(ctx, since, c.f, 100, 0)
		if err != nil {
			t.Fatalf("%s: выборка: %v", c.name, err)
		}
		if got := slugs(list); !same(got, c.want...) {
			t.Errorf("%s: получили %v, ожидали %v", c.name, got, c.want)
		}
		n, err := repo.CountUpcomingForAfisha(ctx, since, c.f)
		if err != nil {
			t.Fatalf("%s: счётчик: %v", c.name, err)
		}
		if n != len(list) {
			t.Errorf("%s: счётчик %d при списке из %d", c.name, n, len(list))
		}
	}
}

func TestWebregFacetsSQLMatchTheListing(t *testing.T) {
	pool := sqlTestPool(t)
	sqlSeed(t, pool)
	repo := NewRepository(pool, "salt")
	ctx := context.Background()

	msk := time.FixedZone("MSK", 3*60*60)
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, msk)
	since := time.Date(2026, 9, 6, 12, 0, 0, 0, msk)
	f := events.Filter{City: events.DefaultCity(), At: now}

	facets, err := repo.FacetsForAfisha(ctx, since, f)
	if err != nil {
		t.Fatalf("фасеты: %v", err)
	}
	count := func(f events.Filter) int {
		n, err := repo.CountUpcomingForAfisha(ctx, since, f)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(f); facets.Total != n {
		t.Errorf("фасеты обещают %d, список отдаёт %d", facets.Total, n)
	}
	if n := count(f); facets.Cities[events.DefaultCitySlug] != n {
		t.Errorf("город обещает %d, список отдаёт %d", facets.Cities[events.DefaultCitySlug], n)
	}
	if len(facets.Categories) != 0 {
		t.Errorf("стор без категории насчитал разделы: %v", facets.Categories)
	}
	for _, code := range events.WhenCodes {
		sub := f
		sub.When = code
		if n := count(sub); facets.When[code] != n {
			t.Errorf("переключатель %q обещает %d, список отдаёт %d", code, facets.When[code], n)
		}
	}
	if n := count(f); facets.Free != n {
		t.Errorf("«бесплатно» обещает %d, список отдаёт %d", facets.Free, n)
	}
}
