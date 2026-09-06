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
		// Ниже — грани полосы. Все три начались или кончились так, что до
		// 07.09 их на доске не было бы вовсе (база гейтила по НАЧАЛУ), либо
		// они попадали бы в полосу времени вместе с сегодняшним концертом.
		//
		// Идущая программа: −5 дней … +5 дней. Единственная running-строка
		// стенда и единственная, ради которой правился boardBase.
		{"идущая программа", "00000000-0000-0000-0000-000000000012", "2026-09-02 10:00", str("2026-09-12 20:00"), str("exhibition"), msk, "paid", "published", "public"},
		// Кончилась вчера вечером: доска держит вчерашнее ещё сутки, так что
		// строка на доске есть, но полоса у неё временная — идти ей уже
		// нечем. Без гейта по концу её бы тоже не было (начало вне окна).
		{"кончилась вчера", "00000000-0000-0000-0000-000000000013", "2026-09-01 10:00", str("2026-09-06 20:00"), nil, msk, "paid", "published", "public"},
		// Многодневная, но ещё не открылась: полоса времени, признак
		// multiday при этом стоит. Проверяет, что полосу не вывели из
		// «многодневности».
		{"неделя с завтра", "00000000-0000-0000-0000-000000000014", "2026-09-08 10:00", str("2026-09-12 20:00"), nil, msk, "paid", "published", "public"},
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
		// Три последние строки добавлены 07.09 вместе с полосой. «Кончилась
		// вчера» попала на доску не сама собой: её начало вне суточного окна,
		// и до правки boardBase (гейт по концу вместо начала) её здесь не
		// было. Ожидание расширено намеренно — это и есть то, ради чего
		// правка делалась, а не побочный эффект.
		{"вся доска города", base(nil),
			[]string{"ночное", "сегодня", "суббота", "завтра без категории", "без деталей",
				"идущая программа", "кончилась вчера", "неделя с завтра"}},
		{"сегодня", base(func(f *Filter) { f.When = WhenToday }),
			[]string{"ночное", "сегодня", "идущая программа"}},
		// Ночное событие кончается завтра в 02:00 — значит идёт и завтра.
		{"завтра", base(func(f *Filter) { f.When = WhenTomorrow }),
			[]string{"ночное", "завтра без категории", "идущая программа", "неделя с завтра"}},
		{"выходные", base(func(f *Filter) { f.When = WhenWeekend }),
			[]string{"суббота", "идущая программа", "неделя с завтра"}},
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
		// База с ВЫБРАННОЙ полосой — самая важная из четырёх: каждое
		// измерение считается с прочими условиями фильтра, но без своего, и
		// на `?kind=running` вторая пилюля обязана показать своё настоящее
		// число, а не ноль.
		{"идёт сейчас", Filter{City: DefaultCity(), At: now, Kind: KindRunning}},
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
		for _, code := range KindCodes {
			sub := b.f
			sub.Kind = code
			if n := total(sub); facets.Kind[code] != n {
				t.Errorf("%s: полоса %q обещает %d, список отдаёт %d", b.name, code, facets.Kind[code], n)
			}
		}
		sub := b.f
		sub.Free = true
		if n := total(sub); facets.Free != n {
			t.Errorf("%s: «бесплатно» обещает %d, список отдаёт %d", b.name, facets.Free, n)
		}
	}
}

// ГЛАВНЫЙ ПРИБОР ПОЛОСЫ: SQL-предикат и Go-классификатор обязаны говорить одно
// и то же. Их двое, и они разные по устройству — условие в запросе и правило в
// маппере, — а разъехаться могут молча: карточка приедет в полосу «идёт
// сейчас» с меткой `timed`, фронт нарисует ей подпись другой полосы, и
// заметить это можно только сверив глазами.
//
// Проверяются три утверждения сразу, потому что порознь каждое проходит и при
// сломанном разбиении: полоса каждой карточки совпадает с запрошенной, две
// выдачи в объединении дают доску целиком, и в пересечении — пусто.
func TestKindSQLСовпадаетСClassify(t *testing.T) {
	pool := boardPool(t)
	byID := boardSeed(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	now := time.Date(2026, 9, 7, 12, 0, 0, 0, mskZone)
	since := time.Date(2026, 9, 6, 12, 0, 0, 0, mskZone)
	list := func(kind string) []PublicEvent {
		t.Helper()
		res, err := repo.List(ctx, ListQuery{Limit: 100, Since: since,
			Filter: Filter{City: DefaultCity(), At: now, Kind: kind}})
		if err != nil {
			t.Fatalf("kind=%q: %v", kind, err)
		}
		if res.Total != len(res.All) {
			t.Errorf("kind=%q: total=%d при списке из %d — подпись «показано N из M» соврёт",
				kind, res.Total, len(res.All))
		}
		return res.All
	}

	running, timed, whole := list(KindRunning), list(KindTimed), list("")
	for _, c := range []struct {
		kind string
		got  []PublicEvent
	}{{KindRunning, running}, {KindTimed, timed}} {
		for _, e := range c.got {
			if e.Kind != c.kind {
				t.Errorf("в полосе %q приехала карточка %q с меткой %q — SQL и классификатор разошлись",
					c.kind, byID[e.ID], e.Kind)
			}
		}
	}

	// Разбиение полное: сумма полос — вся доска. Меньше значит, что карточка
	// не попала никуда и не показывается нигде; больше — что она в обеих.
	union := map[string]bool{}
	for _, e := range append(append([]PublicEvent{}, running...), timed...) {
		if union[e.ID] {
			t.Errorf("карточка %q приехала в ОБЕ полосы", byID[e.ID])
		}
		union[e.ID] = true
	}
	if len(union) != len(whole) {
		t.Errorf("в полосах %d карточек, на доске %d — разбиение не покрывает доску", len(union), len(whole))
	}
	for _, e := range whole {
		if !union[e.ID] {
			t.Errorf("карточка %q (%s) не попала ни в одну полосу — она не показывается нигде", byID[e.ID], e.Kind)
		}
	}

	// Ожидания по именам, а не только по инвариантам: инварианты выполнились
	// бы и у разбиения, объявившего running пустым.
	if got := boardLabels(byID, running); !boardSame(got, "идущая программа") {
		t.Errorf("полоса «идёт сейчас»: %v, ожидали только идущую программу", got)
	}
	if got := boardLabels(byID, timed); !boardSame(got, "ночное", "сегодня", "суббота",
		"завтра без категории", "без деталей", "кончилась вчера", "неделя с завтра") {
		t.Errorf("полоса «по дате и времени»: %v", got)
	}

	// Признак multiday не выводится из полосы и наоборот: программа, которая
	// откроется завтра, многодневна и при этом стоит в полосе времени.
	for _, e := range timed {
		if byID[e.ID] == "неделя с завтра" && !e.Multiday {
			t.Error("многодневная программа впереди не помечена multiday — подпись напишет один день вместо периода")
		}
	}
}

// Полоса «идёт сейчас» обещает показать идущее. До 07.09 общий стор не мог
// отдать в неё НИ ОДНОЙ строки: база гейтила по началу (`e.start_time >= $1`),
// и программа, начавшаяся раньше суточного окна, отсутствовала на доске вовсе
// — не была спрятана фильтром, а не существовала для него.
//
// Проверяется фикстурой, а не живой базой: затронуто 0 строк и на DEV, и на
// PROD (замер 07.09), то есть прогон против боя показал бы зелёное при любой
// реализации.
func TestИдущееМногодневноеВидноОбщимСтором(t *testing.T) {
	pool := boardPool(t)
	byID := boardSeed(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	now := time.Date(2026, 9, 7, 12, 0, 0, 0, mskZone)
	since := time.Date(2026, 9, 6, 12, 0, 0, 0, mskZone)
	res, err := repo.List(ctx, ListQuery{Limit: 100, Since: since,
		Filter: Filter{City: DefaultCity(), At: now}})
	if err != nil {
		t.Fatal(err)
	}
	var found *PublicEvent
	for i, e := range res.All {
		if byID[e.ID] == "идущая программа" {
			found = &res.All[i]
		}
	}
	if found == nil {
		t.Fatalf("программы −5…+5 дней нет на доске: %v", boardLabels(byID, res.All))
	}
	if found.Kind != KindRunning {
		t.Errorf("идущая программа помечена %q", found.Kind)
	}
	if !found.Multiday {
		t.Error("идущая программа не помечена multiday")
	}
}

// Порядок в полосе «идёт сейчас» — по концу: сверху то, что закрывается
// раньше. Полоса отвечает на вопрос «успею ли», и порядок по началу отвечал бы
// на другой — «кто открылся раньше», то есть ставил бы наверх программу,
// которая идёт с марта и будет идти ещё год.
func TestПорядокИдущихПоКонцу(t *testing.T) {
	pool := boardPool(t)
	boardSeed(t, pool)
	ctx := context.Background()
	repo := NewRepository(pool)

	// Своя тройка идущих программ: в общем стенде идущая одна, и порядок на
	// ней не проверить. Начала намеренно в обратном порядке к концам —
	// сортировка по началу дала бы ровно обратный список.
	for _, r := range []struct{ id, start, end string }{
		{"00000000-0000-0000-0000-0000000000f1", "2026-09-05 10:00", "2026-09-25 20:00"},
		{"00000000-0000-0000-0000-0000000000f2", "2026-09-04 10:00", "2026-09-15 20:00"},
		{"00000000-0000-0000-0000-0000000000f3", "2026-09-03 10:00", "2026-09-09 20:00"},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO events (id, title, start_time, end_time, status, tags)
			VALUES ($1, $2, $3::timestamptz, $4::timestamptz, 'published', '[]'::jsonb)`,
			r.id, "Программа "+r.id[len(r.id)-2:], r.start+" MSK", r.end+" MSK"); err != nil {
			t.Fatalf("вставка %s: %v", r.id, err)
		}
	}

	now := time.Date(2026, 9, 7, 12, 0, 0, 0, mskZone)
	since := time.Date(2026, 9, 6, 12, 0, 0, 0, mskZone)
	res, err := repo.List(ctx, ListQuery{Limit: 100, Since: since,
		Filter: Filter{City: DefaultCity(), At: now, Kind: KindRunning}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.All) < 2 {
		t.Fatalf("в полосе %d карточек — порядок проверять не на чем", len(res.All))
	}
	prev := time.Time{}
	for _, e := range res.All {
		end := e.StartTime
		if e.EndTime != nil {
			end = *e.EndTime
		}
		if !prev.IsZero() && end.Before(prev) {
			t.Errorf("порядок нарушен: %s закрывается раньше предыдущей", e.ID)
		}
		prev = end
	}
	// Контроль: тот же набор в полосе времени идёт по НАЧАЛУ, то есть в
	// обратном порядке. Без него тест прошёл бы и на списке из одной строки.
	if res.All[0].EndTime == nil || res.All[len(res.All)-1].EndTime == nil {
		t.Fatal("у крайних карточек полосы нет конца — фикстура не та")
	}
	if !res.All[0].EndTime.Before(*res.All[len(res.All)-1].EndTime) {
		t.Error("первая карточка закрывается не раньше последней — порядок по концу не применился")
	}
}

// Закрепление принадлежит доске ГОРОДА, а не полосе — и проверять это надо
// закреплённой ИДУЩЕЙ программой, потому что на проде закреплено точечное
// событие и разница там не видна вовсе.
//
// Главная просит основную ленту как `kind=timed` и берёт `featured` ровно из
// этого ответа. Значит полоса, применённая к запросу закреплённого, вынесла
// бы закреплённую выставку из шапки главной — молча и только в тот день,
// когда куратор закрепит программу, а не концерт.
func TestPinnedRunningEventSurvivesTheTimedLane(t *testing.T) {
	pool := boardPool(t)
	boardSeed(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	// «идущая программа»: 02.09 → 12.09, то есть полоса running.
	const running = "00000000-0000-0000-0000-000000000012"
	if _, err := pool.Exec(ctx,
		`INSERT INTO afisha_featured (event_id, position) VALUES ($1, 1)`, running); err != nil {
		t.Fatalf("закрепление: %v", err)
	}

	now := time.Date(2026, 9, 7, 12, 0, 0, 0, mskZone)
	since := time.Date(2026, 9, 6, 12, 0, 0, 0, mskZone)
	timed := Filter{City: DefaultCity(), At: now, Kind: KindTimed}

	res, err := repo.List(ctx, ListQuery{Limit: 100, Since: since, Filter: timed})
	if err != nil {
		t.Fatalf("список: %v", err)
	}
	if len(res.Featured) != 1 || res.Featured[0].ID != running {
		t.Errorf("на kind=timed закреплённая идущая программа не пришла: %d карточек %v",
			len(res.Featured), boardLabels(map[string]string{running: "идущая программа"}, res.Featured))
	}
	// При этом САМА полоса её не содержит — иначе «идёт сейчас» протекло бы
	// во «по дате и времени», и тест выше проходил бы по другой причине.
	for _, e := range res.All {
		if e.ID == running {
			t.Error("идущая программа попала в полосу «по дате и времени» — фильтр полосы не работает")
		}
	}

	// Раздел закреплённого по-прежнему не показывает: правка касается полосы,
	// а не отменяет правило.
	section := timed
	section.Category = "concert"
	sec, err := repo.List(ctx, ListQuery{Limit: 100, Since: since, Filter: section})
	if err != nil {
		t.Fatalf("раздел: %v", err)
	}
	if len(sec.Featured) != 0 {
		t.Errorf("в разделе показано закреплённое (%d)", len(sec.Featured))
	}
}

// Тот же прибор, что и TestKindSQLСовпадаетСClassify, но НА СЛЕДУЮЩИЙ ДЕНЬ
// после начала событий. Отдельным тестом, а не ещё одним случаем в том —
// потому что дефект, который он ловит, существует ТОЛЬКО в этот день, а все
// три файла тестов брали `now` в день начала и потому были к нему слепы.
//
// Вчерашняя вечеринка 19:00→02:00, увиденная 08-го, проходит оба сравнения
// полосы («началась вчера», «кончилась сегодня в два ночи») и попадает в
// «идёт сейчас». Порядок в полосе — по концу возрастанием, значит она встаёт
// ПЕРВОЙ; подпись у неё «7 СЕН · 19:00», потому что многодневной она не
// считается. Метка «ИДЁТ СЕЙЧАС» над вчерашней датой держится с двух ночи до
// полуночи, и ни один прежний прибор этого не видел: разбиение оставалось
// полным и непересекающимся.
func TestKindSQLСовпадаетСClassifyНаСледующийДень(t *testing.T) {
	pool := boardPool(t)
	byID := boardSeed(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()

	// 08.09, полдень. Вечеринка «ночное» кончилась в 02:00 этого дня.
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, mskZone)
	since := time.Date(2026, 9, 7, 12, 0, 0, 0, mskZone)
	list := func(kind string) []PublicEvent {
		t.Helper()
		res, err := repo.List(ctx, ListQuery{Limit: 100, Since: since,
			Filter: Filter{City: DefaultCity(), At: now, Kind: kind}})
		if err != nil {
			t.Fatalf("kind=%q: %v", kind, err)
		}
		return res.All
	}
	running, timed, whole := list(KindRunning), list(KindTimed), list("")

	inLane := func(lane []PublicEvent, label string) bool {
		for _, e := range lane {
			if byID[e.ID] == label {
				return true
			}
		}
		return false
	}
	if inLane(running, "ночное") {
		t.Error("вчерашняя вечеринка 19:00→02:00 стоит в полосе «идёт сейчас» — и первой, потому что её конец самый ранний")
	}
	if !inLane(timed, "ночное") {
		t.Errorf("вчерашняя вечеринка не попала и в «по дате и времени»: %v", boardLabels(byID, timed))
	}
	// КОНТРОЛЬ: настоящая идущая программа в этот день по-прежнему идёт.
	// Без него тест прошёл бы и при полосе, которая опустела целиком.
	if !inLane(running, "идущая программа") {
		t.Errorf("настоящая идущая программа выпала из полосы: %v", boardLabels(byID, running))
	}

	// Инварианты разбиения обязаны держаться и в этот день.
	for _, c := range []struct {
		kind string
		got  []PublicEvent
	}{{KindRunning, running}, {KindTimed, timed}} {
		for _, e := range c.got {
			if e.Kind != c.kind {
				t.Errorf("в полосе %q карточка %q с меткой %q", c.kind, byID[e.ID], e.Kind)
			}
		}
	}
	if len(running)+len(timed) != len(whole) {
		t.Errorf("в полосах %d карточек, на доске %d", len(running)+len(timed), len(whole))
	}
}
