package events

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Фильтр публичной ленты: город, категория, «когда», «бесплатно».
//
// Всё, что здесь есть, существует ради одного правила: список и счётчики
// считаются ОДНИМИ И ТЕМИ ЖЕ выражениями. Плитка раздела обещает число,
// человек по ней кликает и обязан увидеть ровно столько же карточек; два
// похожих условия в двух местах — это отказ, который никто не заметит, пока
// кто-нибудь не пересчитает руками.

// Коды параметра when. Строки, а не iota: они едут в query и в ключ кэша.
const (
	WhenToday    = "today"
	WhenTomorrow = "tomorrow"
	WhenWeekend  = "weekend"
)

// WhenCodes — все ведра «когда» в порядке показа. Фасеты считают их все
// сразу, поэтому список нужен и здесь, и в ответе.
var WhenCodes = []string{WhenToday, WhenTomorrow, WhenWeekend}

// FeedCategories — словарь категорий ленты 0.7, дословная копия
// network internal/feed/taxonomy.go (allCategories) в каноническом порядке.
// Копия, а не импорт: афиша и core-api — разные сервисы в разных
// репозиториях, общей библиотеки между ними нет, а код едет по HTTP.
//
// Живёт в пакете events, а не в tgevents, где лежал раньше: словарь читают
// ТРИ стора ленты (общий, tg, веб-регистрация) плюс разбор query, и все они
// сходятся здесь. Обратное направление невозможно технически — tgevents уже
// импортирует events, и вторая копия словаря разъехалась бы с первой ровно в
// тот день, когда лента пополнит его.
var FeedCategories = []string{
	"concert", "party", "lecture", "workshop", "exhibition", "market",
	"sport", "theatre_cinema", "networking", "excursion", "campus",
	"family", "dating", "nightlife", "spiritual", "health", "other",
}

var feedCategorySet = func() map[string]bool {
	m := make(map[string]bool, len(FeedCategories))
	for _, c := range FeedCategories {
		m[c] = true
	}
	return m
}()

// IsFeedCategory — код из словаря ленты? Граница доверия и на импорте
// карточек (tgevents.Card.Validate), и на разборе query.
func IsFeedCategory(code string) bool { return feedCategorySet[code] }

// legacyCategories — значения, которые кабинет организатора писал ДО
// появления словаря; ничего в базе не бэкфилилось, и каждый читатель
// нормализует сам (дословно так же: vshage-organizer/web/src/lib/event-category.ts
// и core-api NormalizeCategory). Без этой таблицы страница «Нетворкинг»
// потеряла бы все события, сохранённые как `meetup`, — и потеряла бы тихо:
// в ленте они есть, в разделе их нет.
//
// Слайсом пар, а не map: из этого списка собирается SQL-выражение, а порядок
// обхода map в Go случайный — текст запроса менялся бы от прогона к прогону,
// и сверка «список и фасеты фильтруют одинаково» перестала бы что-либо
// доказывать.
var legacyCategories = [][2]string{
	{"meetup", "networking"},
	{"conference", "lecture"},
	{"culture", "exhibition"},
	{"education", "lecture"},
	{"social", "party"},
	{"art", "exhibition"},
	{"music", "concert"},
	{"books", "lecture"},
	{"business", "networking"},
	{"tech", "lecture"},
	{"cinema", "theatre_cinema"},
}

// Filter — разобранный и проверенный фильтр ленты.
//
// Город хранится структурой, а не слагом: ответ фасетов отдаёт его падежи, и
// второй поиск по словарю в хендлере — это второй шанс разойтись.
type Filter struct {
	City     City
	Category string // код словаря ленты; пусто = все
	When     string // today | tomorrow | weekend; пусто = все
	Free     bool
	// At — момент, относительно которого разрешается `when`. Ставится ОДИН
	// раз на запрос (Handler.List / Handler.Facets). Сторов у ленты три, и
	// если каждый возьмёт своё time.Now(), в полночь они разъедутся на день:
	// редко, зато молча и вразнобой. Нулевое значение означает «взять
	// текущее время» — так фильтр остаётся пригодным для прямого вызова.
	At time.Time
}

// Now — момент разрешения `when`. Один на запрос и один на все сторы.
func (f Filter) Now() time.Time {
	if f.At.IsZero() {
		return time.Now()
	}
	return f.At
}

// ParseFilter разбирает query публичной ленты.
//
// Неизвестное значение — ОТКАЗ, а не молчаливый сброс фильтра. Сброс выглядит
// как успех: страница `/msk/koncerty` с опечаткой в коде отдала бы полную
// доску, подпись сказала бы «417 событий», и единственным признаком ошибки
// осталась бы чужая карточка в разделе, которую никто не свяжет с опечаткой.
func ParseFilter(q url.Values) (Filter, error) {
	f := Filter{}

	// Пустое значение параметра означает «не задан» — одинаково у всех
	// четырёх: url.Values.Get не различает «ключа нет» и «ключ пустой», и
	// разное поведение у соседних параметров пришлось бы держать в голове
	// каждому, кто собирает адрес руками.
	citySlug := strings.TrimSpace(q.Get("city"))
	if citySlug == "" {
		citySlug = DefaultCitySlug
	}
	city, ok := CityBySlug(citySlug)
	if !ok {
		return Filter{}, fmt.Errorf("город %q не из словаря", citySlug)
	}
	f.City = city

	if cat := strings.TrimSpace(q.Get("category")); cat != "" {
		if !IsFeedCategory(cat) {
			return Filter{}, fmt.Errorf("категория %q не из словаря ленты", cat)
		}
		f.Category = cat
	}

	if when := strings.TrimSpace(q.Get("when")); when != "" {
		if when != WhenToday && when != WhenTomorrow && when != WhenWeekend {
			return Filter{}, fmt.Errorf("when %q — не today, tomorrow или weekend", when)
		}
		f.When = when
	}

	// `free` принимает ровно «1» (фильтровать) и «0» (не фильтровать).
	// Всё остальное — отказ: `free=true` от клиента, который решил, что тут
	// булево, иначе тихо отдал бы платные события в разделе «бесплатно».
	switch free := strings.TrimSpace(q.Get("free")); free {
	case "", "0":
	case "1":
		f.Free = true
	default:
		return Filter{}, fmt.Errorf("free %q — ожидается 1", free)
	}

	return f, nil
}

// IsEmpty — «раздел не выбран». Город сюда НЕ входит: доска города — это и
// есть доска, а не раздел, и закрепление на ней законно. Именно по этому
// признаку лента решает, показывать ли featured (см. Handler.List).
func (f Filter) IsEmpty() bool {
	return f.Category == "" && f.When == "" && !f.Free
}

// CacheKey — ключ кэша страницы. ВСЕ параметры фильтра входят в него.
//
// До 06.09 ключ был `afisha:events:list:<limit>:<offset>`, и добавление
// фильтров без правки ключа отдало бы странице раздела закэшированную полную
// ленту: 200, карточки есть, просто чужие. Немой отказ — и на минуту он
// одинаков для всех, кто открыл раздел.
//
// Для `when` в ключ едет ещё и разрешённая дата: «сегодня», посчитанное до
// полуночи, после полуночи означает другой день, а TTL записи переживает
// полночь.
func (f Filter) CacheKey(limit, offset int) string {
	free := "0"
	if f.Free {
		free = "1"
	}
	key := "afisha:events:list:" + strconv.Itoa(limit) + ":" + strconv.Itoa(offset) +
		":" + f.City.Slug + ":" + f.Category + ":" + f.When + ":" + free
	if r, ok := whenRange(f.When, f.Now()); ok {
		key += ":" + r.FirstDate() + "-" + r.LastDate()
	}
	return key
}

// dayRange — отрезок дней МСК: [From, To), где To — начало дня, следующего за
// последним. Полуинтервал, потому что события сравниваются и по датам, и по
// таймстампам, а «конец последнего дня» в таймстампах пишется только так.
type dayRange struct {
	From time.Time
	To   time.Time
}

func (r dayRange) FirstDate() string { return r.From.Format("2006-01-02") }
func (r dayRange) LastDate() string  { return r.To.AddDate(0, 0, -1).Format("2006-01-02") }

// whenRange переводит код в отрезок дней МСК.
//
// «Сегодня» считается по МСК и уезжает в запрос ПАРАМЕТРОМ: CURRENT_DATE в
// контейнере — UTC, и с полуночи до трёх ночи это вчерашний день.
// mskZone объявлена один раз на пакет (notify.go) — второй FixedZone был бы
// вторым определением того же, а такие расходятся первыми.
func whenRange(code string, now time.Time) (dayRange, bool) {
	if code == "" {
		return dayRange{}, false
	}
	msk := now.In(mskZone)
	today := time.Date(msk.Year(), msk.Month(), msk.Day(), 0, 0, 0, 0, mskZone)
	switch code {
	case WhenToday:
		return dayRange{From: today, To: today.AddDate(0, 0, 1)}, true
	case WhenTomorrow:
		t := today.AddDate(0, 0, 1)
		return dayRange{From: t, To: t.AddDate(0, 0, 1)}, true
	case WhenWeekend:
		// Ближайшие сб+вс; если сегодня суббота или воскресенье — ТЕКУЩИЕ.
		// В воскресенье отрезок начинается со вчерашней субботы намеренно:
		// доска и так держит вчерашнее ещё сутки (окно since = now-24h), и
		// «выходные» без субботы означали бы, что в воскресенье раздел
		// показывает меньше, чем общая лента, — расхождение, которое человек
		// прочитает как пропажу событий.
		var sat time.Time
		switch today.Weekday() {
		case time.Saturday:
			sat = today
		case time.Sunday:
			sat = today.AddDate(0, 0, -1)
		default:
			sat = today.AddDate(0, 0, int(time.Saturday-today.Weekday()))
		}
		return dayRange{From: sat, To: sat.AddDate(0, 0, 2)}, true
	}
	return dayRange{}, false
}

// SQLArgs нумерует аргументы запроса по мере их появления в тексте.
//
// Нужен потому, что условия фильтра собираются из кусков и у каждого стора
// свой набор: считать `$1..$N` руками при трёх запросах на стор — это тот
// самый способ однажды передать limit туда, где ждали дату.
type SQLArgs struct{ args []any }

// NewSQLArgs заводит нумератор, уже занятый аргументами базового предиката
// стора (обычно граница «не старше суток»).
func NewSQLArgs(seed ...any) *SQLArgs {
	a := &SQLArgs{args: make([]any, 0, len(seed)+8)}
	a.args = append(a.args, seed...)
	return a
}

// Add кладёт значение и возвращает его плейсхолдер.
func (a *SQLArgs) Add(v any) string {
	a.args = append(a.args, v)
	return "$" + strconv.Itoa(len(a.args))
}

// All — аргументы в порядке нумерации.
func (a *SQLArgs) All() []any { return a.args }

// StoreSQL описывает, какими КОЛОНКАМИ конкретный стор ленты отвечает на
// четыре вопроса фильтра. Заводится один раз рядом с запросами стора.
//
// Пустая строка означает «такого поля у стора нет вовсе», и это не то же
// самое, что «поле пустое»: стор без категории не попадает НИ В ОДИН раздел
// (и не считается ни в одной плитке), а не попадает во все.
type StoreSQL struct {
	City     string // колонка с названием города: `d.city`, `city`
	Category string // колонка с кодом словаря ленты: `e.category`, `category`
	Free     string // булево выражение «бесплатно»
	Start    string // начало события: `e.start_time`, `date`
	End      string // конец события, COALESCE уже внутри
	// Dates=true у стора, где интервал хранится колонками DATE, а не
	// таймстампами: сравнение тогда идёт с ::date, а не с границей суток.
	Dates bool
}

// CityExpr — нормализованное название города строкой.
//
// Город, которого в карточке нет, считается городом по умолчанию. «Нет
// города» — это неизвестность, а не другой город: выбросить такие карточки с
// доски значило бы спрятать живое событие из-за незаполненного поля формы.
// Настоящий второй город приезжает со СВОИМ названием и на московскую доску
// не попадёт — ради этого фильтр и применяется всегда.
func (s StoreSQL) CityExpr(a *SQLArgs) string {
	if s.City == "" {
		return ""
	}
	return "COALESCE(NULLIF(lower(btrim(" + s.City + ")), ''), " +
		a.Add(strings.ToLower(DefaultCity().Name)) + ")"
}

// CategoryExpr — код словаря ленты либо NULL.
//
// Унаследованные значения кабинета переводятся в коды здесь (см.
// legacyCategories). Всё, что не код и не унаследованное значение, даёт NULL:
// пустое честнее неверного — карточка без категории остаётся в общей ленте и
// не приписывается разделу, которому не принадлежит.
func (s StoreSQL) CategoryExpr() string {
	if s.Category == "" {
		return ""
	}
	norm := "NULLIF(lower(btrim(COALESCE(" + s.Category + ", ''))), '')"
	var b strings.Builder
	b.WriteString("CASE ")
	b.WriteString(norm)
	for _, pair := range legacyCategories {
		b.WriteString(" WHEN '" + pair[0] + "' THEN '" + pair[1] + "'")
	}
	b.WriteString(" ELSE ")
	b.WriteString(norm)
	b.WriteString(" END")
	return b.String()
}

// FreeExpr — «бесплатно» безусловно, без оглядки на текущий фильтр. Нужен
// фасетам: ведро «бесплатно» считается и тогда, когда человек это ведро ещё
// не выбрал.
func (s StoreSQL) FreeExpr() string {
	if s.Free == "" {
		return "FALSE"
	}
	return "(" + s.Free + ")"
}

// CityCond — условие города. У стора без колонки города карточки считаются
// принадлежащими городу по умолчанию: они наши, и заводятся руками (см.
// webreg). Для чужого города такой стор отдаёт FALSE, а не всё подряд.
func (s StoreSQL) CityCond(f Filter, a *SQLArgs) string {
	if f.City.Slug == "" {
		// Нулевое значение Filter — «фильтра нет». Через HTTP оно не приходит
		// (ParseFilter всегда ставит город), но репозиторий зовут и напрямую,
		// и молча сравнивать название с пустой строкой значило бы отдать
		// пустую доску тому, кто просто не заполнил структуру.
		return "TRUE"
	}
	if s.City == "" {
		if f.City.Slug == DefaultCitySlug {
			return "TRUE"
		}
		return "FALSE"
	}
	return "(" + s.CityExpr(a) + " = " + a.Add(strings.ToLower(f.City.Name)) + ")"
}

// CategoryCond — условие раздела.
func (s StoreSQL) CategoryCond(f Filter, a *SQLArgs) string {
	if f.Category == "" {
		return "TRUE"
	}
	if s.Category == "" {
		return "FALSE"
	}
	return "(" + s.CategoryExpr() + " = " + a.Add(f.Category) + ")"
}

// FreeCond — условие «бесплатно» с оглядкой на текущий фильтр.
func (s StoreSQL) FreeCond(f Filter, a *SQLArgs) string {
	if !f.Free {
		return "TRUE"
	}
	return s.FreeExpr()
}

// WhenCond — условие «когда».
//
// Событие попадает в день D, если ИНТЕРВАЛ события накрывает D:
// date <= D <= COALESCE(date_end, date). Это НЕ `eff_date = D`.
//
// eff_date сдвигает идущую многодневную программу на сегодня ради СОРТИРОВКИ
// (см. tgevents.selectCard): выставка, открывшаяся в июне, показывается в
// ленте как сегодняшняя, иначе она лежала бы в прошлом. Фильтр, построенный
// на том же выражении, ответил бы «завтра её нет» — хотя завтра она идёт.
// Сортировка и фильтрация — два разных вопроса, и выражения у них разные.
//
// ОГРАНИЧЕНИЕ, которое это условие снять не может (замер 06.09). Пересечение
// интервалов работает целиком только у tgevents: его базовый предикат доски —
// `COALESCE(date_end, date) >= сегодня`, то есть идущая многодневная
// программа на доске есть. У общего стора база гейтит по НАЧАЛУ
// (`e.start_time >= $1`, repository.go), у webreg — так же (`starts_at >=`),
// и событие, начавшееся раньше суточного окна, отсутствует на доске вовсе;
// до фильтра «сегодня» дело не доходит. То есть у этих двух сторов раздел
// «Сегодня» не покажет идущую многодневную программу — не потому что условие
// неверно, а потому что строки нет.
//
// Не чинится здесь намеренно: правка базы меняет СОСТАВ доски (идущее
// событие встало бы в начало `ORDER BY start_time` и висело там до конца),
// а это отдельное решение, а не побочный эффект фильтров. Практического
// эффекта сегодня нет — в общем сторе одно будущее событие. Написано здесь,
// потому что комментарий, обещающий поведение, которого два стора не дают,
// хуже отсутствующего.
func (s StoreSQL) WhenCond(code string, now time.Time, a *SQLArgs) string {
	r, ok := whenRange(code, now)
	if !ok {
		return "TRUE"
	}
	if s.Dates {
		return "(" + s.Start + " <= " + a.Add(r.LastDate()) + "::date AND " +
			s.End + " >= " + a.Add(r.FirstDate()) + "::date)"
	}
	return "(" + s.Start + " < " + a.Add(r.To) + " AND " +
		s.End + " >= " + a.Add(r.From) + ")"
}

// Where — все четыре условия конъюнкцией. Пустой фильтр даёт TRUE, а не
// пустую строку: вызывающий всегда пишет `AND (` + Where + `)` и не может
// забыть ветку «фильтра нет» — забытая ветка это либо синтаксис, либо, что
// хуже, потерянное условие.
func (s StoreSQL) Where(f Filter, a *SQLArgs) string {
	return s.CityCond(f, a) + " AND " + s.CategoryCond(f, a) +
		" AND " + s.WhenCond(f.When, f.Now(), a) + " AND " + s.FreeCond(f, a)
}

// Facets — счётчики одного стора под текущим фильтром. Складываются по
// сторам (Merge) и отдаются одним ответом.
//
// Каждое измерение считается с ПРОЧИМИ условиями фильтра, но без своего
// собственного: иначе переключатель «завтра» на странице раздела показывал бы
// нули у всех остальных дней и человек решил бы, что событий нет вовсе.
// Total — под ПОЛНЫМ фильтром, то есть ровно то число, которое отдаст список.
type Facets struct {
	Total      int
	Free       int
	Categories map[string]int
	When       map[string]int
	Cities     map[string]int
}

// NewFacets — пустые счётчики с готовыми картами.
func NewFacets() Facets {
	return Facets{
		Categories: map[string]int{},
		When:       map[string]int{},
		Cities:     map[string]int{},
	}
}

// Merge складывает счётчики другого стора.
func (f *Facets) Merge(o Facets) {
	f.Total += o.Total
	f.Free += o.Free
	for k, v := range o.Categories {
		f.Categories[k] += v
	}
	for k, v := range o.When {
		f.When[k] += v
	}
	for k, v := range o.Cities {
		f.Cities[k] += v
	}
}

// FacetQuery — всё, что нужно, чтобы посчитать фасеты одного стора.
//
// Считает их ОДНА функция на все три стора (CountFacets). Три копии
// счётчиков — три способа разойтись со списком, и разойдутся они тихо:
// плитка обещает 119, открывается 12.
type FacetQuery struct {
	Pool  *pgxpool.Pool
	Store StoreSQL
	// From — «FROM …» стора вместе с джойнами, нужными условиям фильтра.
	From string
	// Base — предикат витрины стора БЕЗ фильтра: не скрыто, не прошло,
	// видимо. Ровно тот же текст, что и у списка, — иначе «показано N из M».
	Base string
	// Seed — аргументы, которые Base занимает первыми номерами.
	Seed []any
	// Name — имя стора для строки лога про город вне словаря.
	Name string
}

// CountFacets считает счётчики стора.
func CountFacets(ctx context.Context, q FacetQuery, f Filter) (Facets, error) {
	now := f.Now()
	out := NewFacets()

	// 1. Вёдра «когда», «бесплатно» и общее число — одним запросом: пять
	// COUNT(*) FILTER по одной и той же выборке дешевле пяти проходов.
	a := NewSQLArgs(q.Seed...)
	city := q.Store.CityCond(f, a)
	cat := q.Store.CategoryCond(f, a)
	when := q.Store.WhenCond(f.When, now, a)
	free := q.Store.FreeCond(f, a)
	freeExpr := q.Store.FreeExpr()
	buckets := make([]string, 0, len(WhenCodes))
	for _, code := range WhenCodes {
		buckets = append(buckets,
			"COUNT(*) FILTER (WHERE "+cat+" AND "+free+" AND "+q.Store.WhenCond(code, now, a)+")")
	}
	sql := "SELECT " + strings.Join(buckets, ", ") +
		", COUNT(*) FILTER (WHERE " + cat + " AND " + when + " AND " + freeExpr + ")" +
		", COUNT(*) FILTER (WHERE " + cat + " AND " + when + " AND " + free + ")" +
		" " + q.From + " WHERE " + q.Base + " AND " + city
	counts := make([]int, len(WhenCodes)+2)
	dst := make([]any, len(counts))
	for i := range counts {
		dst[i] = &counts[i]
	}
	if err := q.Pool.QueryRow(ctx, sql, a.All()...).Scan(dst...); err != nil {
		return Facets{}, err
	}
	for i, code := range WhenCodes {
		out.When[code] = counts[i]
	}
	out.Free = counts[len(WhenCodes)]
	out.Total = counts[len(WhenCodes)+1]

	// 2. Разрез по категориям — со всеми условиями, КРОМЕ самой категории.
	if expr := q.Store.CategoryExpr(); expr != "" {
		ac := NewSQLArgs(q.Seed...)
		sqlCat := "SELECT " + expr + ", COUNT(*) " + q.From +
			" WHERE " + q.Base +
			" AND " + q.Store.CityCond(f, ac) +
			" AND " + q.Store.WhenCond(f.When, now, ac) +
			" AND " + q.Store.FreeCond(f, ac) +
			" AND " + expr + " IS NOT NULL GROUP BY 1"
		rows, err := q.Pool.Query(ctx, sqlCat, ac.All()...)
		if err != nil {
			return Facets{}, err
		}
		unknown := map[string]int{}
		for rows.Next() {
			var code string
			var n int
			if err := rows.Scan(&code, &n); err != nil {
				rows.Close()
				return Facets{}, err
			}
			if !IsFeedCategory(code) {
				unknown[code] += n
				continue
			}
			out.Categories[code] += n
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return Facets{}, err
		}
		for code, n := range unknown {
			// Значение вне словаря в разделы не попадает и в плитках не
			// показывается — но и молчать о нём нельзя: так выглядит
			// разъехавшийся конвейер, а по числу карточек это заметно.
			log.Printf("events.Facets: стор %s: категория %q вне словаря, карточек %d", q.Name, code, n)
		}
	}

	// 3. Разрез по городам — со всеми условиями, КРОМЕ самого города: иначе
	// переключатель города показывал бы ноль у всех городов, кроме открытого.
	acity := NewSQLArgs(q.Seed...)
	catC := q.Store.CategoryCond(f, acity)
	whenC := q.Store.WhenCond(f.When, now, acity)
	freeC := q.Store.FreeCond(f, acity)
	if q.Store.City == "" {
		// У стора нет колонки города: его события наши и заводятся руками,
		// то есть принадлежат городу по умолчанию.
		var n int
		sqlNoCity := "SELECT COUNT(*) " + q.From + " WHERE " + q.Base +
			" AND " + catC + " AND " + whenC + " AND " + freeC
		if err := q.Pool.QueryRow(ctx, sqlNoCity, acity.All()...).Scan(&n); err != nil {
			return Facets{}, err
		}
		if n > 0 {
			out.Cities[DefaultCitySlug] += n
		}
		return out, nil
	}
	sqlCity := "SELECT " + q.Store.CityExpr(acity) + ", COUNT(*) " + q.From +
		" WHERE " + q.Base + " AND " + catC + " AND " + whenC + " AND " + freeC + " GROUP BY 1"
	rows, err := q.Pool.Query(ctx, sqlCity, acity.All()...)
	if err != nil {
		return Facets{}, err
	}
	byName := map[string]string{}
	for _, c := range Cities() {
		byName[strings.ToLower(c.Name)] = c.Slug
	}
	unknownCity := map[string]int{}
	for rows.Next() {
		var name string
		var n int
		if err := rows.Scan(&name, &n); err != nil {
			rows.Close()
			return Facets{}, err
		}
		slug, ok := byName[name]
		if !ok {
			unknownCity[name] += n
			continue
		}
		out.Cities[slug] += n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Facets{}, err
	}
	for name, n := range unknownCity {
		// Ровно тот случай, ради которого фильтр по городу применяется при
		// одном городе в словаре: данные второго города уже приехали, а
		// доски у него ещё нет. Он не смешался с московской — и об этом
		// сказано вслух, иначе узнали бы мы об этом от человека.
		log.Printf("events.Facets: стор %s: город %q вне словаря, карточек %d", q.Name, name, n)
	}
	return out, nil
}
