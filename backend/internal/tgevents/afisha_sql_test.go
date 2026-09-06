package tgevents

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pythonjsgo/vshage-afisha/internal/events"
)

// Прогон против НАСТОЯЩЕГО постгреса: условия фильтра — это SQL, и проверять
// их чтением текста запроса значит проверять собственную догадку о том, как
// Postgres его выполнит. Тест поднимает свою СХЕМУ (не базу целиком) и живёт
// в ней: чужие таблицы он не видит и не трогает.
//
// Запускать:
//
//	AFISHA_TEST_DATABASE_URL=postgres://postgres@localhost:55432/afisha_z1 \
//	  go test ./internal/tgevents/ -run SQL
//
// Без переменной тест пропускается: в CI постгреса нет, и падать там ему не
// на чем. Пропуск — не «зелёный»: сверять список с фасетами больше нечем,
// и это сказано в отчёте зоны.
const sqlTestSchema = "afisha_filter_test"

func sqlTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("AFISHA_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("AFISHA_TEST_DATABASE_URL не задан — прогон против настоящего постгреса пропущен")
	}
	ctx := context.Background()

	// Схема заводится отдельным подключением: пул уже открывается с
	// search_path на неё, а схемы ещё нет.
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

	// Колонки — те, которые читает витрина (selectCard) и по которым
	// фильтрует доска. Таблица пересоздаётся в СВОЕЙ схеме, поэтому боевая
	// afisha_tg_events этим прогоном не затрагивается.
	if _, err := pool.Exec(ctx, `
		DROP TABLE IF EXISTS afisha_tg_events;
		CREATE TABLE afisha_tg_events (
			id               TEXT PRIMARY KEY,
			title            TEXT NOT NULL,
			annonce          TEXT NOT NULL,
			date             DATE NOT NULL,
			date_end         DATE,
			time_start       TEXT,
			city             TEXT,
			place_name       TEXT,
			address          TEXT,
			online           BOOLEAN NOT NULL DEFAULT FALSE,
			price_raw        TEXT,
			is_free          BOOLEAN,
			registration_url TEXT,
			access_level     TEXT NOT NULL DEFAULT 'unknown',
			segment          TEXT,
			category         TEXT,
			org_name         TEXT,
			source_url       TEXT,
			venue            JSONB,
			cover            BYTEA,
			hidden           BOOLEAN NOT NULL DEFAULT FALSE
		)`); err != nil {
		t.Fatalf("создание таблицы: %v", err)
	}
	return pool
}

// Витрина на 2026-09-07 (понедельник). Выходные этой недели — 12 и 13-е.
type sqlFixture struct {
	id       string
	date     string
	dateEnd  *string
	city     *string
	category *string
	isFree   *bool
	hidden   bool
}

func sqlSeed(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	str := func(s string) *string { return &s }
	yes, no := true, false
	msk := str("Москва")
	rows := []sqlFixture{
		// Многодневная программа: идёт и сегодня, и завтра, и на выходных.
		{id: "ev_expo", date: "2026-08-01", dateEnd: str("2026-09-30"), city: msk, category: str("exhibition"), isFree: &no},
		{id: "ev_today", date: "2026-09-07", city: msk, category: str("concert"), isFree: &yes},
		{id: "ev_tomorrow", date: "2026-09-08", city: msk, category: str("lecture"), isFree: &no},
		{id: "ev_weekend", date: "2026-09-12", city: msk, category: str("party"), isFree: &no},
		// Прошедшее: за окном витрины (since = 06.09).
		{id: "ev_past", date: "2026-09-01", city: msk, category: str("concert"), isFree: &no},
		// Другой город: на московскую доску попасть не должен.
		{id: "ev_spb", date: "2026-09-07", city: str("Санкт-Петербург"), category: str("concert"), isFree: &no},
		// Без категории: в ленте есть, в разделах нет.
		{id: "ev_nocat", date: "2026-09-07", city: msk, isFree: &no},
		// Без города: неизвестность, а не другой город — доска по умолчанию.
		{id: "ev_nocity", date: "2026-09-09", category: str("market"), isFree: &no},
		// Снятое с витрины.
		{id: "ev_hidden", date: "2026-09-07", city: msk, category: str("concert"), isFree: &no, hidden: true},
	}
	ctx := context.Background()
	for _, r := range rows {
		if _, err := pool.Exec(ctx, `
			INSERT INTO afisha_tg_events
				(id, title, annonce, date, date_end, city, category, is_free, hidden, access_level, source_url)
			VALUES ($1, $2, $3, $4::date, $5::date, $6, $7, $8, $9, 'open', 'https://t.me/x/1')`,
			r.id, "Событие "+r.id, "Наш текст анонса.", r.date, r.dateEnd,
			r.city, r.category, r.isFree, r.hidden); err != nil {
			t.Fatalf("вставка %s: %v", r.id, err)
		}
	}
}

func sqlIDs(list []events.PublicEvent) []string {
	out := make([]string, 0, len(list))
	for _, e := range list {
		out = append(out, e.ID)
	}
	sort.Strings(out)
	return out
}

func sameIDs(got []string, want ...string) bool {
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

// Главная проверка семантики «когда»: идущая многодневная программа обязана
// попасть И в «сегодня», И в «завтра», И в «выходные». Фильтр на ключе
// сортировки (eff_date = D) ответил бы «завтра её нет», хотя завтра она идёт,
// и на однодневных событиях вёл бы себя правильно — то есть врал бы ровно там,
// где выставки, и только там.
func TestFilterSQLAgainstPostgres(t *testing.T) {
	pool := sqlTestPool(t)
	sqlSeed(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	now := time.Date(2026, 9, 7, 12, 0, 0, 0, msk) // понедельник
	since := time.Date(2026, 9, 6, 12, 0, 0, 0, msk)
	base := func(mod func(*events.Filter)) events.Filter {
		f := events.Filter{City: events.DefaultCity(), At: now}
		if mod != nil {
			mod(&f)
		}
		return f
	}

	cases := []struct {
		name string
		f    events.Filter
		want []string
	}{
		{"вся доска города", base(nil),
			[]string{"ev_expo", "ev_today", "ev_tomorrow", "ev_weekend", "ev_nocat", "ev_nocity"}},
		{"сегодня", base(func(f *events.Filter) { f.When = events.WhenToday }),
			[]string{"ev_expo", "ev_today", "ev_nocat"}},
		{"завтра", base(func(f *events.Filter) { f.When = events.WhenTomorrow }),
			[]string{"ev_expo", "ev_tomorrow"}},
		{"выходные", base(func(f *events.Filter) { f.When = events.WhenWeekend }),
			[]string{"ev_expo", "ev_weekend"}},
		{"раздел «выставки»", base(func(f *events.Filter) { f.Category = "exhibition" }),
			[]string{"ev_expo"}},
		{"раздел «концерты»", base(func(f *events.Filter) { f.Category = "concert" }),
			[]string{"ev_today"}},
		{"бесплатно", base(func(f *events.Filter) { f.Free = true }), []string{"ev_today"}},
		{"выставки на выходных", base(func(f *events.Filter) {
			f.Category = "exhibition"
			f.When = events.WhenWeekend
		}), []string{"ev_expo"}},
		{"концерты завтра — пусто", base(func(f *events.Filter) {
			f.Category = "concert"
			f.When = events.WhenTomorrow
		}), nil},
	}

	for _, c := range cases {
		list, err := repo.UpcomingForAfisha(ctx, since, c.f, 100, 0)
		if err != nil {
			t.Fatalf("%s: выборка: %v", c.name, err)
		}
		if got := sqlIDs(list); !sameIDs(got, c.want...) {
			t.Errorf("%s: получили %v, ожидали %v", c.name, got, c.want)
		}
		// Счётчик обязан совпасть со списком: расхождение — это подпись
		// «показано N из M», которая врёт ровно на разницу.
		n, err := repo.CountUpcomingForAfisha(ctx, since, c.f)
		if err != nil {
			t.Fatalf("%s: счётчик: %v", c.name, err)
		}
		if n != len(list) {
			t.Errorf("%s: счётчик %d при списке из %d", c.name, n, len(list))
		}
	}
}

// Число на плитке — обещание: столько карточек откроется по клику. Тест
// проверяет его единственным честным способом — открывает каждую плитку и
// пересчитывает.
//
// Проверяется не только пустой фильтр. Каждое измерение считается с ПРОЧИМИ
// условиями, но без своего: на странице «сегодня» плитка «Выставки» обязана
// обещать выставки СЕГОДНЯ, а переключатель «завтра» — всё завтрашнее той же
// категории. Фасеты, посчитанные по всей доске, выглядят достовернее всего
// именно там, где они врут.
func TestFacetsSQLMatchTheListing(t *testing.T) {
	pool := sqlTestPool(t)
	sqlSeed(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	now := time.Date(2026, 9, 7, 12, 0, 0, 0, msk)
	since := time.Date(2026, 9, 6, 12, 0, 0, 0, msk)
	city := events.DefaultCity()

	bases := []struct {
		name string
		f    events.Filter
	}{
		{"вся доска", events.Filter{City: city, At: now}},
		{"сегодня", events.Filter{City: city, At: now, When: events.WhenToday}},
		{"бесплатно", events.Filter{City: city, At: now, Free: true}},
		{"раздел «выставки»", events.Filter{City: city, At: now, Category: "exhibition"}},
	}

	count := func(t *testing.T, f events.Filter) int {
		t.Helper()
		n, err := repo.CountUpcomingForAfisha(ctx, since, f)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	for _, b := range bases {
		facets, err := repo.FacetsForAfisha(ctx, since, b.f)
		if err != nil {
			t.Fatalf("%s: фасеты: %v", b.name, err)
		}
		if n := count(t, b.f); facets.Total != n {
			t.Errorf("%s: фасеты обещают %d событий, список отдаёт %d", b.name, facets.Total, n)
		}
		if n := count(t, b.f); facets.Cities[events.DefaultCitySlug] != n {
			t.Errorf("%s: город обещает %d, список отдаёт %d", b.name, facets.Cities[events.DefaultCitySlug], n)
		}
		for _, code := range events.FeedCategories {
			sub := b.f
			sub.Category = code
			if n := count(t, sub); facets.Categories[code] != n {
				t.Errorf("%s: плитка %q обещает %d, раздел отдаёт %d", b.name, code, facets.Categories[code], n)
			}
		}
		for _, code := range events.WhenCodes {
			sub := b.f
			sub.When = code
			if n := count(t, sub); facets.When[code] != n {
				t.Errorf("%s: переключатель %q обещает %d, список отдаёт %d", b.name, code, facets.When[code], n)
			}
		}
		sub := b.f
		sub.Free = true
		if n := count(t, sub); facets.Free != n {
			t.Errorf("%s: «бесплатно» обещает %d, список отдаёт %d", b.name, facets.Free, n)
		}
	}

	// Карточка без категории не приписана ни одному разделу: сумма плиток
	// меньше общего числа ровно на неё. Пустое честнее неверного.
	facets, err := repo.FacetsForAfisha(ctx, since, events.Filter{City: city, At: now})
	if err != nil {
		t.Fatal(err)
	}
	sum := 0
	for _, n := range facets.Categories {
		sum += n
	}
	if sum >= facets.Total {
		t.Errorf("сумма плиток %d при общем %d — карточка без категории куда-то приписана", sum, facets.Total)
	}
}
