package events

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"
)

// mskDate — день МСК как момент времени. Тесты фиксируют «сейчас» руками:
// «сегодня» у ленты считается по МСК, и прогон в 23:30 по Москве иначе давал
// бы другой ответ, чем в 09:00, — флака, которая читается как регрессия.
func mskDate(y int, m time.Month, d, hh int) time.Time {
	return time.Date(y, m, d, hh, 0, 0, 0, mskZone)
}

func mustParse(t *testing.T, raw string) Filter {
	t.Helper()
	q, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatal(err)
	}
	f, err := ParseFilter(q)
	if err != nil {
		t.Fatalf("ParseFilter(%q): %v", raw, err)
	}
	return f
}

// Пустой запрос — доска города по умолчанию, а не «городов нет». Фильтр по
// городу применяется всегда, даже пока город один: иначе первая же карточка
// второго города приедет на московскую доску молча.
func TestParseFilterDefaultsToTheDefaultCity(t *testing.T) {
	f := mustParse(t, "")
	if f.City.Slug != DefaultCitySlug {
		t.Errorf("город по умолчанию %q, ожидали %q", f.City.Slug, DefaultCitySlug)
	}
	if f.City.In == "" || f.City.Of == "" {
		t.Error("падежи города не заполнены — фронту нечем писать заголовок")
	}
	if !f.IsEmpty() {
		t.Error("пустой запрос обязан считаться «раздел не выбран»")
	}
}

// Неизвестное значение — ОТКАЗ. Молчаливый сброс фильтра выглядит как успех:
// страница раздела с опечаткой отдала бы полную доску и подпись «417
// событий», и признаком ошибки осталась бы одна чужая карточка.
func TestParseFilterRefusesUnknownValues(t *testing.T) {
	for _, raw := range []string{
		"city=spb",
		"category=koncert", // латиницей мимо словаря
		"category=Concert", // регистр значим: код словаря в нижнем
		"when=tonight",
		"when=today2",
		"free=true",
		"free=yes",
	} {
		q, err := url.ParseQuery(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseFilter(q); err == nil {
			t.Errorf("ParseFilter(%q) принял значение вне словаря — фильтр сбросился молча", raw)
		}
	}
}

// Пустое значение параметра означает «параметр не задан» — одинаково для всех
// четырёх. Правило одно на все параметры сознательно: `q.Get` не различает
// «ключа нет» и «ключ пустой», и разное поведение у соседних параметров
// пришлось бы держать в голове каждому, кто собирает адрес руками.
func TestEmptyParameterMeansNotGiven(t *testing.T) {
	f := mustParse(t, "city=&category=&when=&free=")
	if f.City.Slug != DefaultCitySlug || f.Category != "" || f.When != "" || f.Free {
		t.Errorf("пустые параметры дали фильтр %+v", f)
	}
}

func TestParseFilterAcceptsTheWholeDictionary(t *testing.T) {
	for _, code := range FeedCategories {
		f := mustParse(t, "category="+code)
		if f.Category != code {
			t.Errorf("категория %q не доехала до фильтра", code)
		}
		if f.IsEmpty() {
			t.Errorf("фильтр с категорией %q считается пустым — доска покажет закреплённое в разделе", code)
		}
	}
	for _, code := range WhenCodes {
		if f := mustParse(t, "when="+code); f.When != code {
			t.Errorf("when %q не доехал до фильтра", code)
		}
	}
	if f := mustParse(t, "free=1"); !f.Free {
		t.Error("free=1 не включил фильтр")
	}
	if f := mustParse(t, "free=0"); f.Free {
		t.Error("free=0 включил фильтр")
	}
}

// Город в фильтр входит, но «непустым фильтром» не считается: доска города —
// это доска, а не раздел, и закреплённое на ней законно.
func TestCityAloneKeepsTheBoardPinned(t *testing.T) {
	if f := mustParse(t, "city=msk"); !f.IsEmpty() {
		t.Error("выбор города прочитан как раздел — закреплённое исчезнет с главной")
	}
}

// «Сегодня» и «завтра» — ровно один день, выходные — суббота и воскресенье,
// причём в субботу и воскресенье ТЕКУЩИЕ, а не следующие.
func TestWhenRangeCoversTheRightDays(t *testing.T) {
	// 2026-09-06 — воскресенье; неделя ниже разбирается по дням от понедельника.
	cases := []struct {
		now                time.Time
		weekday            time.Weekday
		wantSat, wantSun   string
		wantToday, wantTom string
	}{
		{mskDate(2026, 9, 7, 12), time.Monday, "2026-09-12", "2026-09-13", "2026-09-07", "2026-09-08"},
		{mskDate(2026, 9, 11, 12), time.Friday, "2026-09-12", "2026-09-13", "2026-09-11", "2026-09-12"},
		{mskDate(2026, 9, 12, 12), time.Saturday, "2026-09-12", "2026-09-13", "2026-09-12", "2026-09-13"},
		{mskDate(2026, 9, 13, 12), time.Sunday, "2026-09-12", "2026-09-13", "2026-09-13", "2026-09-14"},
	}
	for _, c := range cases {
		if got := c.now.Weekday(); got != c.weekday {
			t.Fatalf("образец %s: день недели %v, а тест написан про %v", c.now.Format("2006-01-02"), got, c.weekday)
		}
		today, ok := whenRange(WhenToday, c.now)
		if !ok || today.FirstDate() != c.wantToday || today.LastDate() != c.wantToday {
			t.Errorf("%v: сегодня = %s..%s, ожидали %s", c.weekday, today.FirstDate(), today.LastDate(), c.wantToday)
		}
		tom, _ := whenRange(WhenTomorrow, c.now)
		if tom.FirstDate() != c.wantTom || tom.LastDate() != c.wantTom {
			t.Errorf("%v: завтра = %s..%s, ожидали %s", c.weekday, tom.FirstDate(), tom.LastDate(), c.wantTom)
		}
		wknd, _ := whenRange(WhenWeekend, c.now)
		if wknd.FirstDate() != c.wantSat || wknd.LastDate() != c.wantSun {
			t.Errorf("%v: выходные = %s..%s, ожидали %s..%s",
				c.weekday, wknd.FirstDate(), wknd.LastDate(), c.wantSat, c.wantSun)
		}
	}
	if _, ok := whenRange("", mskDate(2026, 9, 7, 12)); ok {
		t.Error("пустой when дал отрезок — фильтра быть не должно")
	}
}

// «Сегодня» считается по МСК, а контейнер живёт в UTC: в 01:00 по Москве
// UTC-дата ещё вчерашняя, и CURRENT_DATE спрятал бы сегодняшние события.
func TestTodayIsCountedInMoscowTime(t *testing.T) {
	night := time.Date(2026, 9, 6, 22, 30, 0, 0, time.UTC) // 07.09 01:30 МСК
	r, ok := whenRange(WhenToday, night)
	if !ok || r.FirstDate() != "2026-09-07" {
		t.Errorf("в 01:30 МСК «сегодня» = %s, ожидали 2026-09-07", r.FirstDate())
	}
}

// САМОЕ ВАЖНОЕ МЕСТО. Событие попадает в день D, если ИНТЕРВАЛ события
// накрывает D: date <= D <= COALESCE(date_end, date). Это НЕ eff_date = D:
// eff_date сдвигает идущую многодневную программу на сегодня ради сортировки,
// и фильтр «завтра» на нём потерял бы выставку, которая идёт и завтра.
func TestWhenFiltersByIntervalNotBySortKey(t *testing.T) {
	store := StoreSQL{Start: "date", End: "COALESCE(date_end, date)", Dates: true}
	a := NewSQLArgs()
	cond := store.WhenCond(WhenTomorrow, mskDate(2026, 9, 7, 12), a)

	if strings.Contains(cond, "eff_date") || strings.Contains(cond, "eff_time") {
		t.Fatalf("фильтр опирается на ключ сортировки: %s", cond)
	}
	if !strings.Contains(cond, "date <=") || !strings.Contains(cond, "COALESCE(date_end, date) >=") {
		t.Fatalf("условие не похоже на пересечение интервалов: %s", cond)
	}
	args := a.All()
	if len(args) != 2 {
		t.Fatalf("аргументов %d, ожидали два конца отрезка: %v", len(args), args)
	}
	// Начало события сравнивается с ПОСЛЕДНИМ днём отрезка, конец — с ПЕРВЫМ.
	// Перепутанные местами границы дают условие, которое на однодневных
	// событиях ведёт себя правильно и врёт ровно на многодневных.
	if args[0] != "2026-09-08" || args[1] != "2026-09-08" {
		t.Errorf("границы отрезка %v, ожидали 2026-09-08 дважды", args)
	}

	aw := NewSQLArgs()
	store.WhenCond(WhenWeekend, mskDate(2026, 9, 7, 12), aw)
	wargs := aw.All()
	if wargs[0] != "2026-09-13" || wargs[1] != "2026-09-12" {
		t.Errorf("выходные: начало сравнивается с %v, конец с %v; ожидали 13-е и 12-е", wargs[0], wargs[1])
	}
}

// Ключ кэша обязан различать ВСЕ параметры фильтра. Пока он был
// `<limit>:<offset>`, страница раздела получила бы закэшированную полную
// ленту — 200, карточки есть, просто чужие, и на минуту одинаково у всех.
func TestCacheKeySeparatesEveryFilter(t *testing.T) {
	now := mskDate(2026, 9, 7, 12)
	base := Filter{City: DefaultCity(), At: now}
	other := City{Slug: "spb", Name: "Санкт-Петербург", In: "в Санкт-Петербурге", Of: "Санкт-Петербурга"}

	variants := map[string]string{
		"базовый":     base.CacheKey(30, 0),
		"страница 2":  base.CacheKey(30, 30),
		"иной размер": base.CacheKey(60, 0),
		"город":       Filter{City: other, At: now}.CacheKey(30, 0),
		"категория":   Filter{City: DefaultCity(), Category: "concert", At: now}.CacheKey(30, 0),
		"другая кат.": Filter{City: DefaultCity(), Category: "lecture", At: now}.CacheKey(30, 0),
		"сегодня":     Filter{City: DefaultCity(), When: WhenToday, At: now}.CacheKey(30, 0),
		"завтра":      Filter{City: DefaultCity(), When: WhenTomorrow, At: now}.CacheKey(30, 0),
		"выходные":    Filter{City: DefaultCity(), When: WhenWeekend, At: now}.CacheKey(30, 0),
		"бесплатно":   Filter{City: DefaultCity(), Free: true, At: now}.CacheKey(30, 0),
	}
	seen := map[string]string{}
	for name, key := range variants {
		if prev, dup := seen[key]; dup {
			t.Errorf("%q и %q делят запись кэша: %s", prev, name, key)
		}
		seen[key] = name
	}

	// «Сегодня», посчитанное до полуночи, после полуночи означает другой
	// день, а запись кэша полночь переживает.
	today := Filter{City: DefaultCity(), When: WhenToday, At: now}
	tomorrow := Filter{City: DefaultCity(), When: WhenToday, At: now.AddDate(0, 0, 1)}
	if today.CacheKey(30, 0) == tomorrow.CacheKey(30, 0) {
		t.Error("«сегодня» вчера и сегодня делят запись кэша — после полуночи лента отдаст вчерашний день")
	}
}

// Список и счётчик обязаны фильтровать ОДНИМ выражением: разойдутся — и
// подпись «показано N из M» соврёт ровно на разницу.
func TestListAndCountShareOneExpression(t *testing.T) {
	f := Filter{City: DefaultCity(), Category: "exhibition", When: WhenWeekend, Free: true,
		At: mskDate(2026, 9, 7, 12)}
	a1, a2 := NewSQLArgs("since"), NewSQLArgs("since")
	if mainStore.Where(f, a1) != mainStore.Where(f, a2) {
		t.Fatal("два вызова Where дали разный текст — сверять список со счётчиком нечем")
	}
	if len(a1.All()) != len(a2.All()) {
		t.Fatal("два вызова Where заняли разное число аргументов")
	}
	// Первым номером идёт аргумент базового предиката, фильтр занимает
	// следующие: перепутанная нумерация даёт «дату вместо лимита».
	if a1.All()[0] != "since" {
		t.Errorf("аргумент базового предиката съехал: %v", a1.All())
	}
}

// Пустой фильтр не должен превращаться в пустое условие: вызывающий пишет
// `AND (` + Where + `)` без ветвления, и пустая строка была бы синтаксисом.
func TestEmptyFilterIsTrueNotEmpty(t *testing.T) {
	a := NewSQLArgs()
	if got := mainStore.Where(Filter{}, a); !strings.Contains(got, "TRUE") {
		t.Errorf("пустой фильтр дал %q", got)
	}
	if n := len(a.All()); n != 0 {
		t.Errorf("пустой фильтр занял %d аргументов", n)
	}
}

// Унаследованные значения кабинета в базе не бэкфилились, и каждый читатель
// нормализует сам. Без этого раздел «Нетворкинг» потерял бы всё, что
// сохранено как `meetup`, — и потерял бы тихо.
func TestCategoryExprMapsLegacyPanelValues(t *testing.T) {
	expr := mainStore.CategoryExpr()
	for _, pair := range legacyCategories {
		if !strings.Contains(expr, "'"+pair[0]+"' THEN '"+pair[1]+"'") {
			t.Errorf("унаследованное %q не переводится в %q", pair[0], pair[1])
		}
		if !IsFeedCategory(pair[1]) {
			t.Errorf("унаследованное %q переводится в %q вне словаря ленты", pair[0], pair[1])
		}
	}
	if strings.Contains(expr, "$") {
		t.Error("в выражении категории появился аргумент — оно повторяется в запросе дважды, и нумерация разъедется")
	}
}

// Стор, у которого поля нет вовсе, не попадает НИ В ОДИН раздел и не
// смешивается с чужим городом. Молчаливое TRUE отдало бы в раздел «Выставки»
// события веб-регистрации, у которых категории нет.
func TestStoreWithoutAFieldMatchesNothing(t *testing.T) {
	store := StoreSQL{City: "", Category: "", Free: "TRUE", Start: "starts_at", End: "COALESCE(ends_at, starts_at)"}
	a := NewSQLArgs()
	if got := store.CategoryCond(Filter{Category: "concert"}, a); got != "FALSE" {
		t.Errorf("стор без категории в разделе «Концерты» отвечает %q", got)
	}
	if got := store.CityCond(Filter{City: DefaultCity()}, a); got != "TRUE" {
		t.Errorf("стор без города не попал на доску города по умолчанию: %q", got)
	}
	spb := City{Slug: "spb", Name: "Санкт-Петербург"}
	if got := store.CityCond(Filter{City: spb}, a); got != "FALSE" {
		t.Errorf("стор без города приехал на доску другого города: %q", got)
	}
}

// Карточка без города — это неизвестность, а не другой город: выбросить её с
// доски значило бы спрятать живое событие из-за незаполненного поля формы.
func TestUnknownCityFallsBackToTheDefaultBoard(t *testing.T) {
	a := NewSQLArgs()
	cond := mainStore.CityCond(Filter{City: DefaultCity()}, a)
	if !strings.Contains(cond, "COALESCE(NULLIF(") {
		t.Fatalf("условие города не подставляет город по умолчанию: %s", cond)
	}
	args := a.All()
	if len(args) != 2 || args[0] != "москва" || args[1] != "москва" {
		t.Errorf("аргументы города %v, ожидали дважды название города по умолчанию", args)
	}
}

// Форма ответа фасетов — контракт с фронтом: по этим ключам рисуются плитки
// разделов и переключатель города. Тест держит ИМЕНА полей: переименование
// поля на бэкенде компилируется, деплоится и молча оставляет страницу без
// плиток — фронт просто не найдёт ключ.
func TestFacetsResponseShapeMatchesTheContract(t *testing.T) {
	agg := NewFacets()
	agg.Total = 417
	agg.Free = 76
	agg.Categories["exhibition"] = 74
	agg.Categories["theatre_cinema"] = 119
	agg.Categories["campus"] = 0 // пустой раздел — плитка-тупик, не показываем
	agg.When[WhenToday] = 42
	agg.Cities[DefaultCitySlug] = 417

	raw, err := json.Marshal(facetsResponse(Filter{City: DefaultCity()}, agg, nil))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"city", "cities", "total", "categories", "when", "free"} {
		if _, ok := got[key]; !ok {
			t.Errorf("в ответе нет поля %q", key)
		}
	}
	if _, ok := got["degraded"]; ok {
		t.Error("degraded попал в ответ при живых сторах — фронт прочитает это как отказ")
	}

	var city map[string]string
	if err := json.Unmarshal(got["city"], &city); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"slug", "name", "in", "of"} {
		if city[key] == "" {
			t.Errorf("у города нет падежа %q — заголовок страницы писать нечем", key)
		}
	}

	var cities []map[string]any
	if err := json.Unmarshal(got["cities"], &cities); err != nil {
		t.Fatal(err)
	}
	if len(cities) != len(Cities()) || cities[0]["count"] != float64(417) {
		t.Errorf("список городов не тот: %v", cities)
	}

	var cats []CategoryCount
	if err := json.Unmarshal(got["categories"], &cats); err != nil {
		t.Fatal(err)
	}
	// По убыванию счётчика и без пустых.
	if len(cats) != 2 || cats[0].Code != "theatre_cinema" || cats[1].Code != "exhibition" {
		t.Errorf("разделы не отсортированы по убыванию или показан пустой: %v", cats)
	}

	var when map[string]int
	if err := json.Unmarshal(got["when"], &when); err != nil {
		t.Fatal(err)
	}
	if len(when) != len(WhenCodes) {
		t.Errorf("вёдер «когда» %d, ожидали %d — переключатель не нарисуется целиком", len(when), len(WhenCodes))
	}
}

// Ключ обязан быть КАНОНИЧЕСКИМ: один и тот же запрос, записанный по-разному,
// делит одну запись кэша. Ключ собирается из полей разобранного фильтра в
// фиксированном порядке, а не склейкой сырой query-строки: иначе
// перестановка параметров в адресе даёт второй ключ на тот же ответ — кэш
// при этом «работает», просто мимо, и заметить это можно только по числу
// промахов.
func TestCacheKeyIsCanonical(t *testing.T) {
	now := mskDate(2026, 9, 7, 12)
	spellings := []string{
		"category=concert",
		"category=concert&when=",
		"when=&free=&category=concert",
		"city=msk&category=concert",
		"free=0&category=concert&city=msk",
	}
	var first, firstRaw string
	for _, raw := range spellings {
		f := mustParse(t, raw)
		f.At = now
		key := f.CacheKey(30, 0)
		if first == "" {
			first, firstRaw = key, raw
			continue
		}
		if key != first {
			t.Errorf("«%s» и «%s» дали разные ключи:\n  %s\n  %s", firstRaw, raw, first, key)
		}
	}
}
