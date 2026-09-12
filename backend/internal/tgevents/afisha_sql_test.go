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
			hidden           BOOLEAN NOT NULL DEFAULT FALSE,
			listed           BOOLEAN NOT NULL DEFAULT TRUE,
			payload          JSONB NOT NULL DEFAULT '{}',
			updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
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
		// Грани полосы. ev_expo выше — тоже идущая; здесь добавлены те, на
		// которых полоса ошибается охотнее всего.
		//
		// Бессрочная: date_end — сентинел «конца нет» из конвейера. В полосе
		// «идёт сейчас» ей место в самом конце (см. TestПорядокИдущихПоКонцу).
		{id: "ev_forever", date: "2026-03-01", dateEnd: str("9999-01-01"), city: msk, category: str("exhibition"), isFree: &no},
		// Кончилась вчера: витрина держит вчерашнее ещё сутки, сдвига старта
		// нет, полоса временная.
		{id: "ev_over", date: "2026-09-01", dateEnd: str("2026-09-06"), city: msk, category: str("exhibition"), isFree: &no},
		// Многодневная, но открывается завтра: полоса времени при multiday.
		{id: "ev_ahead", date: "2026-09-08", dateEnd: str("2026-09-20"), city: msk, category: str("exhibition"), isFree: &no},
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
		// Ожидания расширены 07.09 вместе с фикстурами полосы: ev_forever
		// (бессрочная), ev_over (кончилась вчера) и ev_ahead (открывается
		// завтра). Условия «когда» не менялись — изменился стенд, и каждое
		// новое вхождение объяснимо интервалом: бессрочная накрывает любой
		// день, ev_ahead идёт с 8-го по 20-е, ev_over — только 1–6 сентября.
		{"вся доска города", base(nil),
			[]string{"ev_expo", "ev_today", "ev_tomorrow", "ev_weekend", "ev_nocat", "ev_nocity",
				"ev_forever", "ev_over", "ev_ahead"}},
		{"сегодня", base(func(f *events.Filter) { f.When = events.WhenToday }),
			[]string{"ev_expo", "ev_today", "ev_nocat", "ev_forever"}},
		{"завтра", base(func(f *events.Filter) { f.When = events.WhenTomorrow }),
			[]string{"ev_expo", "ev_tomorrow", "ev_forever", "ev_ahead"}},
		{"выходные", base(func(f *events.Filter) { f.When = events.WhenWeekend }),
			[]string{"ev_expo", "ev_weekend", "ev_forever", "ev_ahead"}},
		{"раздел «выставки»", base(func(f *events.Filter) { f.Category = "exhibition" }),
			[]string{"ev_expo", "ev_forever", "ev_over", "ev_ahead"}},
		{"раздел «концерты»", base(func(f *events.Filter) { f.Category = "concert" }),
			[]string{"ev_today"}},
		{"бесплатно", base(func(f *events.Filter) { f.Free = true }), []string{"ev_today"}},
		{"выставки на выходных", base(func(f *events.Filter) {
			f.Category = "exhibition"
			f.When = events.WhenWeekend
		}), []string{"ev_expo", "ev_forever", "ev_ahead"}},
		{"концерты завтра — пусто", base(func(f *events.Filter) {
			f.Category = "concert"
			f.When = events.WhenTomorrow
		}), nil},
		// Полоса — такое же условие фильтра, как и остальные, и счётчик под
		// ней обязан сойтись со списком (проверяется общим циклом ниже).
		{"идёт сейчас", base(func(f *events.Filter) { f.Kind = events.KindRunning }),
			[]string{"ev_expo", "ev_forever"}},
		{"по дате и времени", base(func(f *events.Filter) { f.Kind = events.KindTimed }),
			[]string{"ev_today", "ev_tomorrow", "ev_weekend", "ev_nocat", "ev_nocity",
				"ev_over", "ev_ahead"}},
		// Полоса пересекается с разделом, а не подменяет его: идущих выставок
		// на витрине две, и «идёт сейчас» внутри раздела обязано дать их же.
		{"идущие выставки", base(func(f *events.Filter) {
			f.Category = "exhibition"
			f.Kind = events.KindRunning
		}), []string{"ev_expo", "ev_forever"}},
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
		// База с ВЫБРАННОЙ полосой — самая важная: каждое измерение считается
		// с прочими условиями фильтра, но без своего, и на `?kind=running`
		// вторая пилюля обязана показать своё настоящее число, а не ноль.
		{"идёт сейчас", events.Filter{City: city, At: now, Kind: events.KindRunning}},
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
		for _, code := range events.KindCodes {
			sub := b.f
			sub.Kind = code
			if n := count(t, sub); facets.Kind[code] != n {
				t.Errorf("%s: полоса %q обещает %d, список отдаёт %d", b.name, code, facets.Kind[code], n)
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

// SQL-предикат полосы и Go-классификатор (events.Classify) обязаны говорить
// одно и то же. Их двое, они разного устройства, и разъезжаются молча:
// карточка приезжает в полосу «идёт сейчас» с меткой `timed`, фронт рисует ей
// подпись другой полосы, и заметить это можно только глазами.
//
// Витрина tg — самый опасный из трёх сторов: только у неё старт СДВИНУТ на
// сегодня ради сортировки, то есть SQL и классификатор смотрят на разные
// колонки (`date` против StartTime/ActualStartDate).
func TestKindSQLСовпадаетСClassify(t *testing.T) {
	pool := sqlTestPool(t)
	sqlSeed(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	now := time.Date(2026, 9, 7, 12, 0, 0, 0, msk)
	since := time.Date(2026, 9, 6, 12, 0, 0, 0, msk)
	list := func(kind string) []events.PublicEvent {
		t.Helper()
		out, err := repo.UpcomingForAfisha(ctx, since,
			events.Filter{City: events.DefaultCity(), At: now, Kind: kind}, 100, 0)
		if err != nil {
			t.Fatalf("kind=%q: %v", kind, err)
		}
		return out
	}

	running, timed, whole := list(events.KindRunning), list(events.KindTimed), list("")
	for _, c := range []struct {
		kind string
		got  []events.PublicEvent
	}{{events.KindRunning, running}, {events.KindTimed, timed}} {
		for _, e := range c.got {
			if e.Kind != c.kind {
				t.Errorf("в полосе %q приехала карточка %s с меткой %q — SQL и классификатор разошлись",
					c.kind, e.ID, e.Kind)
			}
		}
	}

	union := map[string]bool{}
	for _, e := range append(append([]events.PublicEvent{}, running...), timed...) {
		if union[e.ID] {
			t.Errorf("карточка %s приехала в ОБЕ полосы", e.ID)
		}
		union[e.ID] = true
	}
	if len(union) != len(whole) {
		t.Errorf("в полосах %d карточек, на витрине %d — разбиение не покрывает витрину", len(union), len(whole))
	}
	for _, e := range whole {
		if !union[e.ID] {
			t.Errorf("карточка %s (%s) не попала ни в одну полосу — она не показывается нигде", e.ID, e.Kind)
		}
	}
}

// Порядок в полосе «идёт сейчас» — по концу: сверху то, что закрывается
// раньше. Полоса отвечает на «успею ли», и порядок по началу отвечал бы на
// другой вопрос — ставил бы наверх программу, которая идёт с марта и будет
// идти ещё год. Бессрочная уходит в конец сама, сентинелом, без спецслучая.
func TestПорядокИдущихПоКонцу(t *testing.T) {
	pool := sqlTestPool(t)
	sqlSeed(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	// Третья идущая программа: с двумя порядок не отличить от случайного.
	// Начало позже, чем у ev_expo, конец раньше — сортировка по началу дала
	// бы другой список.
	if _, err := pool.Exec(ctx, `
		INSERT INTO afisha_tg_events (id, title, annonce, date, date_end, city, category, is_free, access_level, source_url)
		VALUES ('ev_soon', 'Скоро закроется', 'Наш текст.', '2026-08-20'::date, '2026-09-10'::date,
		        'Москва', 'exhibition', FALSE, 'open', 'https://t.me/x/2')`); err != nil {
		t.Fatalf("вставка ev_soon: %v", err)
	}

	now := time.Date(2026, 9, 7, 12, 0, 0, 0, msk)
	since := time.Date(2026, 9, 6, 12, 0, 0, 0, msk)
	got, err := repo.UpcomingForAfisha(ctx, since,
		events.Filter{City: events.DefaultCity(), At: now, Kind: events.KindRunning}, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	order := make([]string, 0, len(got))
	for _, e := range got {
		order = append(order, e.ID)
	}
	// ev_soon закрывается 10.09, ev_expo 30.09, ev_forever не закрывается.
	if len(order) != 3 || order[0] != "ev_soon" || order[1] != "ev_expo" || order[2] != "ev_forever" {
		t.Fatalf("порядок полосы %v, ожидали [ev_soon ev_expo ev_forever]", order)
	}

	// Контроль: тот же набор в полосе времени идёт по НАЧАЛУ показа, то есть
	// ev_forever первой. Без него тест прошёл бы и при полностью
	// проигнорированном kind — например если бы порядок по концу совпал с
	// порядком по началу случайно.
	all, err := repo.UpcomingForAfisha(ctx, since, events.Filter{City: events.DefaultCity(), At: now}, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	first := ""
	for _, e := range all {
		if e.Kind == events.KindRunning {
			first = e.ID
			break
		}
	}
	if first == "ev_soon" {
		t.Error("общая лента идёт тем же порядком, что и полоса — переключения ключа не произошло")
	}
}

// Бессрочная карточка не должна нести end_time и через SQL-путь тоже: маппер
// один, но выборка своя, и «проверили на юнит-тесте» тут не считается.
func TestБессрочноеБезEndTimeЧерезВыборку(t *testing.T) {
	pool := sqlTestPool(t)
	sqlSeed(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	now := time.Date(2026, 9, 7, 12, 0, 0, 0, msk)
	since := time.Date(2026, 9, 6, 12, 0, 0, 0, msk)
	list, err := repo.UpcomingForAfisha(ctx, since, events.Filter{City: events.DefaultCity(), At: now}, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	seenForever, seenNormal := false, false
	for _, e := range list {
		switch e.ID {
		case "ev_forever":
			seenForever = true
			if !e.OpenEnded {
				t.Error("ev_forever не помечена open_ended")
			}
			if e.EndTime != nil {
				t.Errorf("у ev_forever есть end_time (%v) — карточка нарисует «до 1 ЯНВ»", e.EndTime)
			}
		case "ev_expo":
			// Контроль: у обычной многодневной конец обязан быть. Иначе тест
			// выше прошёл бы при маппере, потерявшем end_time у всех.
			seenNormal = true
			if e.EndTime == nil {
				t.Error("у ev_expo пропал end_time — сломан маппер, а не сентинел")
			}
		}
	}
	if !seenForever || !seenNormal {
		t.Fatalf("фикстуры не доехали до выдачи (forever=%v, expo=%v)", seenForever, seenNormal)
	}
}

// Утверждение, на котором держится ОТСУТСТВИЕ порога длительности в ветке
// `Dates` (см. events.StoreSQL.KindCond): у карточки на датах размах в полосе
// «идёт сейчас» не бывает меньше суток, поэтому порог она проходит всегда, и
// добавлять его сюда не нужно — а нужно нельзя: `date - date` в Postgres даёт
// integer, и сравнение с interval падает типом.
//
// Проверяется САМЫМ ТЕСНЫМ возможным случаем: начало вчера в 23:59 (позже в
// формате HH:MM не бывает), конец — сегодня. Если SQL и классификатор
// разойдутся где-то, то здесь.
func TestТеснейшаяИдущаяКарточкаПроходитПорог(t *testing.T) {
	pool := sqlTestPool(t)
	sqlSeed(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `
		INSERT INTO afisha_tg_events
			(id, title, annonce, date, time_start, date_end, city, is_free, access_level, source_url)
		VALUES ('ev_tight', 'Впритык', 'Наш текст.', '2026-09-06'::date, '23:59',
		        '2026-09-07'::date, 'Москва', FALSE, 'open', 'https://t.me/x/3')`); err != nil {
		t.Fatalf("вставка ev_tight: %v", err)
	}

	now := time.Date(2026, 9, 7, 12, 0, 0, 0, msk)
	since := time.Date(2026, 9, 6, 12, 0, 0, 0, msk)
	got, err := repo.UpcomingForAfisha(ctx, since,
		events.Filter{City: events.DefaultCity(), At: now, Kind: events.KindRunning}, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	var tight *events.PublicEvent
	for i, e := range got {
		if e.ID == "ev_tight" {
			tight = &got[i]
		}
	}
	if tight == nil {
		t.Fatalf("теснейшая карточка не попала в полосу «идёт сейчас»: %v", sqlIDs(got))
	}
	// Классификатор обязан согласиться с SQL: иначе она приедет в полосу с
	// чужой меткой, и подпись на карточке будет от другой полосы.
	if tight.Kind != events.KindRunning || !tight.Multiday {
		t.Errorf("теснейшая карточка: kind=%q multiday=%v — SQL и классификатор разошлись",
			tight.Kind, tight.Multiday)
	}
	// И назвать размах числом, а не «он большой»: у карточки на датах начало
	// берётся из actual_start_date, то есть с ПОЛУНОЧИ первого дня, а конец —
	// 23:59 последнего. Даже в этом, теснейшем случае это 47:59.
	if tight.ActualStartDate == nil || tight.EndTime == nil {
		t.Fatalf("у теснейшей карточки нет actual_start_date или end_time — фикстура не та")
	}
	start, err := time.ParseInLocation("2006-01-02", *tight.ActualStartDate, msk)
	if err != nil {
		t.Fatal(err)
	}
	if span := tight.EndTime.Sub(start); span < 20*time.Hour {
		t.Errorf("размах теснейшей идущей карточки %v — порог 20ч она НЕ проходит, значит ветке Dates порог всё-таки нужен", span)
	} else {
		t.Logf("размах теснейшей идущей карточки: %v (порог 20ч проходит с запасом)", span)
	}
}

// Одиночная карточка классифицируется относительно ПЕРЕДАННОГО момента, а не
// собственных часов репозитория. Раньше часов внутри GetByID было двое — один
// на сдвиг eff_date в запросе, второй на Classify, — и между ними могла пройти
// полночь: карточка получала бы дату показа одного дня и полосу другого.
//
// Тест возможен только благодаря аргументу: с внутренним time.Now() написать
// «эта выставка идёт 7-го и кончилась к 1 октября» нечем.
func TestGetByIDКлассифицируетПоПереданномуМоменту(t *testing.T) {
	pool := sqlTestPool(t)
	sqlSeed(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	// ev_expo: 01.08 → 30.09.
	for _, c := range []struct {
		name string
		now  time.Time
		kind string
	}{
		{"7 сентября — идёт", time.Date(2026, 9, 7, 12, 0, 0, 0, msk), events.KindRunning},
		{"1 октября — кончилась", time.Date(2026, 10, 1, 12, 0, 0, 0, msk), events.KindTimed},
		// 30 сентября — последний день, программа ещё идёт. Граница, на
		// которой «кончилась» и «идёт» отличаются одним днём.
		{"30 сентября — последний день", time.Date(2026, 9, 30, 12, 0, 0, 0, msk), events.KindRunning},
	} {
		ev, err := repo.GetByID(ctx, "ev_expo", c.now)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if ev.Kind != c.kind {
			t.Errorf("%s: полоса %q, ожидали %q", c.name, ev.Kind, c.kind)
		}
	}
}
