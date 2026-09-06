package events

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Прогон общего стора против НАСТОЯЩЕГО постгреса. Здесь самый сложный SQL
// ленты: нормализация унаследованных категорий кабинета, пересечение
// интервалов по таймстампам и предикат видимости. Читать этот текст глазами и
// решать, что он делает, — гадание; единственный судья тут Postgres.
//
// Схема заводится СВОЯ и минимальная: боевые `events` и соседние таблицы
// принадлежат core-api, их миграций у афиши нет вовсе, и подкладывать полную
// копию значило бы завести вторую схему, которая разъедется с первой.
// Проверяется поэтому ровно то, что проверяет запрос: колонки, которые он
// называет. Пропуск теста без переменной окружения — не «зелёный».
//
//	AFISHA_TEST_DATABASE_URL=postgres://…/afisha_z1 go test ./internal/events/ -run SQL
const boardSchema = "afisha_board_test"

func boardPool(t *testing.T) *pgxpool.Pool {
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
	if _, err := boot.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+boardSchema); err != nil {
		boot.Close()
		t.Fatalf("создание схемы: %v", err)
	}
	boot.Close()

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("разбор строки подключения: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = boardSchema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("подключение к схеме: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, `
		DROP TABLE IF EXISTS events, organizer_event_details, profiles, providers,
			afisha_featured, afisha_event_photos, event_registrations;
		CREATE TABLE events (
			id            UUID PRIMARY KEY,
			title         TEXT NOT NULL,
			description   TEXT,
			location      TEXT,
			start_time    TIMESTAMPTZ NOT NULL,
			end_time      TIMESTAMPTZ,
			status        TEXT NOT NULL,
			category      TEXT,
			tags          JSONB,
			max_attendees INTEGER,
			photo_url     TEXT,
			organizer_id  UUID
		);
		CREATE TABLE organizer_event_details (
			event_id                  UUID PRIMARY KEY,
			short_description         TEXT,
			registration_mode         TEXT,
			external_registration_url TEXT,
			registration_deadline     TIMESTAMPTZ,
			price_type                TEXT,
			price_min                 INTEGER,
			price_max                 INTEGER,
			currency                  TEXT,
			city                      TEXT,
			venue_name                TEXT,
			address                   TEXT,
			online_url                TEXT,
			age_limit                 TEXT,
			attendees_note            TEXT,
			visibility                TEXT,
			reg_form                  JSONB,
			reg_fields                JSONB
		);
		CREATE TABLE profiles (id UUID PRIMARY KEY, name TEXT, photo_url TEXT);
		CREATE TABLE providers (profile_id UUID, display_name TEXT, avatar_url TEXT);
		CREATE TABLE afisha_featured (event_id UUID PRIMARY KEY, position INTEGER);
		CREATE TABLE afisha_event_photos (event_id UUID, url TEXT, position INTEGER);
		CREATE TABLE event_registrations (event_id UUID, status TEXT)`); err != nil {
		t.Fatalf("создание схемы стенда: %v", err)
	}
	return pool
}

// Витрина на 2026-09-07 (понедельник); выходные этой недели — 12 и 13-е.
type boardFixture struct {
	label      string
	id         string
	start      string
	end        *string
	category   *string
	city       *string
	priceType  string
	status     string
	visibility string
}

func boardSeed(t *testing.T, pool *pgxpool.Pool) map[string]string {
	t.Helper()
	str := func(s string) *string { return &s }
	msk := str("Москва")
	rows := []boardFixture{
		// Ночное событие: начинается сегодня, кончается завтра под утро.
		// Категория — унаследованное значение кабинета: в базе `meetup`,
		// в разделе обязано найтись как «нетворкинг».
		{"ночное", "00000000-0000-0000-0000-00000000000a", "2026-09-07 19:00", str("2026-09-08 02:00"), str("meetup"), msk, "paid", "published", "public"},
		{"сегодня", "00000000-0000-0000-0000-00000000000b", "2026-09-07 12:00", nil, str("concert"), msk, "free", "published", "public"},
		{"суббота", "00000000-0000-0000-0000-00000000000c", "2026-09-12 12:00", nil, str("exhibition"), msk, "paid", "published", "public"},
		{"завтра без категории", "00000000-0000-0000-0000-00000000000d", "2026-09-08 12:00", nil, nil, msk, "paid", "published", "public"},
		// Скрытое с доски, но живое по ссылке.
		{"unlisted", "00000000-0000-0000-0000-00000000000e", "2026-09-07 13:00", nil, str("concert"), msk, "paid", "published", "unlisted"},
		{"другой город", "00000000-0000-0000-0000-00000000000f", "2026-09-07 14:00", nil, str("concert"), str("Санкт-Петербург"), "paid", "published", "public"},
		{"черновик", "00000000-0000-0000-0000-000000000010", "2026-09-07 15:00", nil, str("concert"), msk, "paid", "draft", "public"},
		// Деталей кабинета нет вовсе: ни города, ни цены. Такое событие
		// остаётся на доске города по умолчанию и считается бесплатным —
		// ровно так же, как его показывает карточка.
		{"без деталей", "00000000-0000-0000-0000-000000000011", "2026-09-09 12:00", nil, str("lecture"), nil, "", "published", ""},
	}
	ctx := context.Background()
	byLabel := map[string]string{}
	for _, r := range rows {
		if _, err := pool.Exec(ctx, `
			INSERT INTO events (id, title, start_time, end_time, status, category, tags)
			VALUES ($1, $2, $3::timestamptz, $4::timestamptz, $5, $6, '[]'::jsonb)`,
			r.id, "Событие "+r.label, r.start+" MSK", nullableTS(r.end), r.status, r.category); err != nil {
			t.Fatalf("вставка %s: %v", r.label, err)
		}
		if r.visibility != "" || r.city != nil || r.priceType != "" {
			if _, err := pool.Exec(ctx, `
				INSERT INTO organizer_event_details (event_id, city, price_type, visibility)
				VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''))`,
				r.id, r.city, r.priceType, r.visibility); err != nil {
				t.Fatalf("вставка деталей %s: %v", r.label, err)
			}
		}
		byLabel[r.id] = r.label
	}
	return byLabel
}

func nullableTS(v *string) any {
	if v == nil {
		return nil
	}
	return *v + " MSK"
}

func boardLabels(byID map[string]string, list []PublicEvent) []string {
	out := make([]string, 0, len(list))
	for _, e := range list {
		out = append(out, byID[e.ID])
	}
	sort.Strings(out)
	return out
}

func boardSame(got []string, want ...string) bool {
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

func TestBoardFilterSQLAgainstPostgres(t *testing.T) {
	pool := boardPool(t)
	byID := boardSeed(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	now := time.Date(2026, 9, 7, 12, 0, 0, 0, mskZone)
	since := time.Date(2026, 9, 6, 12, 0, 0, 0, mskZone)
	base := func(mod func(*Filter)) Filter {
		f := Filter{City: DefaultCity(), At: now}
		if mod != nil {
			mod(&f)
		}
		return f
	}

	cases := []struct {
		name string
		f    Filter
		want []string
	}{
		{"вся доска города", base(nil),
			[]string{"ночное", "сегодня", "суббота", "завтра без категории", "без деталей"}},
		{"сегодня", base(func(f *Filter) { f.When = WhenToday }), []string{"ночное", "сегодня"}},
		// Ночное событие кончается завтра в 02:00 — значит идёт и завтра.
		{"завтра", base(func(f *Filter) { f.When = WhenTomorrow }), []string{"ночное", "завтра без категории"}},
		{"выходные", base(func(f *Filter) { f.When = WhenWeekend }), []string{"суббота"}},
		// Унаследованное `meetup` обязано найтись в «нетворкинге»: в базе
		// ничего не бэкфилилось, и без нормализации раздел был бы пуст.
		{"нетворкинг (унаследованное meetup)", base(func(f *Filter) { f.Category = "networking" }), []string{"ночное"}},
		{"концерты", base(func(f *Filter) { f.Category = "concert" }), []string{"сегодня"}},
		// Событие БЕЗ строки деталей кабинета в «Бесплатно» НЕ попадает.
		// Прежде здесь стояло обратное ожидание — зеркало подстановки
		// COALESCE(d.price_type,'free') в предикате и в карточке. Обе
		// подстановки сняты 06.09: колонка price_type — NOT NULL DEFAULT
		// 'free', значит NULL тут означает ровно «строки деталей нет», а
		// такое событие доска держит намеренно. «Не знаю» и «бесплатно» —
		// разные утверждения, и второе мы бы говорили от своего имени: и
		// человеку в разделе «вход свободный, без билета», и поисковику
		// через offers.price: 0.
		{"бесплатно", base(func(f *Filter) { f.Free = true }), []string{"сегодня"}},
	}

	for _, c := range cases {
		res, err := repo.List(ctx, ListQuery{Limit: 100, Since: since, Filter: c.f})
		if err != nil {
			t.Fatalf("%s: список: %v", c.name, err)
		}
		if got := boardLabels(byID, res.All); !boardSame(got, c.want...) {
			t.Errorf("%s: получили %v, ожидали %v", c.name, got, c.want)
		}
		if res.Total != len(res.All) {
			t.Errorf("%s: total=%d при списке из %d — подпись «показано N из M» соврёт", c.name, res.Total, len(res.All))
		}
		if !c.f.IsEmpty() && len(res.Featured) != 0 {
			t.Errorf("%s: в разделе показано закреплённое (%d) — закрепление относится к доске города", c.name, len(res.Featured))
		}
	}
}

// Фасеты общего стора обязаны совпасть со списком под тем же фильтром.
func TestBoardFacetsSQLMatchTheListing(t *testing.T) {
	pool := boardPool(t)
	boardSeed(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	now := time.Date(2026, 9, 7, 12, 0, 0, 0, mskZone)
	since := time.Date(2026, 9, 6, 12, 0, 0, 0, mskZone)

	for _, b := range []struct {
		name string
		f    Filter
	}{
		{"вся доска", Filter{City: DefaultCity(), At: now}},
		{"сегодня", Filter{City: DefaultCity(), At: now, When: WhenToday}},
		{"бесплатно", Filter{City: DefaultCity(), At: now, Free: true}},
	} {
		facets, err := repo.Facets(ctx, since, b.f)
		if err != nil {
			t.Fatalf("%s: фасеты: %v", b.name, err)
		}
		total := func(f Filter) int {
			res, err := repo.List(ctx, ListQuery{Limit: 100, Since: since, Filter: f})
			if err != nil {
				t.Fatal(err)
			}
			return res.Total
		}
		if n := total(b.f); facets.Total != n {
			t.Errorf("%s: фасеты обещают %d, список отдаёт %d", b.name, facets.Total, n)
		}
		if n := total(b.f); facets.Cities[DefaultCitySlug] != n {
			t.Errorf("%s: город обещает %d, список отдаёт %d", b.name, facets.Cities[DefaultCitySlug], n)
		}
		for _, code := range FeedCategories {
			sub := b.f
			sub.Category = code
			if n := total(sub); facets.Categories[code] != n {
				t.Errorf("%s: плитка %q обещает %d, раздел отдаёт %d", b.name, code, facets.Categories[code], n)
			}
		}
		for _, code := range WhenCodes {
			sub := b.f
			sub.When = code
			if n := total(sub); facets.When[code] != n {
				t.Errorf("%s: переключатель %q обещает %d, список отдаёт %d", b.name, code, facets.When[code], n)
			}
		}
		sub := b.f
		sub.Free = true
		if n := total(sub); facets.Free != n {
			t.Errorf("%s: «бесплатно» обещает %d, список отдаёт %d", b.name, facets.Free, n)
		}
	}
}
