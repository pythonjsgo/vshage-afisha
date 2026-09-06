package events

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// ExtraSource — дополнительный источник событий для ленты афиши.
// Реализуют пакеты webreg (события веб-регистрации, директива 17.08) и
// tgevents (студсобытия из телеграм-каналов, директива 30.08). Интерфейс
// объявлен здесь, чтобы источники зависели от events, а не наоборот.
//
// Пагинация — часть контракта, а не удобство. Источник обязан уметь отдать
// СВОЁ окно [offset, offset+limit) по возрастанию времени начала и своё общее
// число: до 30.08 шов брал у источника всё и приклеивал к уже обрезанной
// странице, поэтому вторая страница повторяла первую, а total не равнялся
// числу событий, которые лента способна отдать. Пока источник был один и
// отдавал единицы событий, оба дефекта были незаметны.
//
// Фильтр — тоже часть контракта, и он в СИГНАТУРЕ, а не в необязательном
// интерфейсе с проверкой типа: источник, молча не применивший фильтр, отдал
// бы в раздел «Выставки» свои концерты, а счётчик при этом сошёлся бы сам с
// собой. Расширение интерфейса ломает компиляцию у обоих реализующих
// репозиториев — это и есть прибор, который нельзя забыть запустить.
type ExtraSource interface {
	UpcomingForAfisha(ctx context.Context, since time.Time, f Filter, limit, offset int) ([]PublicEvent, error)
	CountUpcomingForAfisha(ctx context.Context, since time.Time, f Filter) (int, error)
	FacetsForAfisha(ctx context.Context, since time.Time, f Filter) (Facets, error)
}

// Потолки страницы. Раньше их роль играли разрозненные `> 100 → 30` внутри
// источников: не потолок, а тихая подмена запрошенного окна.
const (
	defaultPageSize = 30
	// maxPageSize — сколько отдаём за ОДИН запрос, maxWindow — как глубоко
	// можно листать. До 06.09 они были равны (300/300), и это было верно,
	// пока страница просила одну страницу целиком: тогда «размер страницы» и
	// «глубина» были одним числом. С «показать ещё» это две разные величины,
	// и держать их равными значит либо запретить глубину, либо разрешить
	// выкачивать доску одним запросом.
	//
	// Что произойдёт, когда доска перерастёт maxWindow: события за окном
	// станут недостижимы из интерфейса — «показать ещё» упрётся в 400. Это
	// уже случалось на 150 событиях при потолке 90, и единственным признаком
	// была подпись «СОБЫТИЯ · 90 ИЗ 150», которую прочитал человек. Поэтому
	// ниже, в List, стоит строка в лог: прибор обязан кричать сам, а не
	// ждать, пока кто-нибудь сверит два числа глазами.
	maxPageSize = 200
	maxWindow   = 1000
)

// MaxWindow — тот же maxWindow наружу, для источников ленты. Свой потолок в
// источнике, ниже общего, молча обрезал бы глубокую страницу: источник
// вернул бы «сколько смог», слияние приняло бы это за «сколько есть», и
// страница выглядела бы полной без части событий. Так уже было, когда внутри
// источников стояло `> 100 → 30`.
const MaxWindow = maxWindow

// overWindowLoggedAt — когда последний раз жаловались на переросшую доску
// (unix-секунды). Жалоба нужна каждый день, но не каждому запросу: при 1 rps
// это 86 тысяч одинаковых строк в сутки, и в них утонет всё остальное.
// Гонка здесь безобидна — худшее, что бывает, это две строки вместо одной.
var overWindowLoggedAt atomic.Int64

// sourceName — имя источника для поля degraded и для лога. Без него отказ
// одного из двух сторов неотличим в ответе от «в нём просто ничего нет».
func sourceName(s ExtraSource) string {
	if n, ok := s.(interface{ AfishaSourceName() string }); ok {
		return n.AfishaSourceName()
	}
	return "extra"
}

type Handler struct {
	repo  *Repository
	cache *Cache
	extra []ExtraSource
}

func NewHandler(r *Repository, c *Cache) *Handler {
	return &Handler{repo: r, cache: c}
}

// WithExtraSource подключает дополнительный источник событий.
// Источников может быть несколько, и они ДОБАВЛЯЮТСЯ: раньше здесь было одно
// поле, и второй вызов молча вытеснял первый — то есть подключить студсобытия
// значило бы выключить веб-регистрацию, ничего об этом не сказав.
func (h *Handler) WithExtraSource(s ExtraSource) *Handler {
	if s != nil {
		h.extra = append(h.extra, s)
	}
	return h
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	// ВЕСЬ вход проверяется ДО того, как появится ключ кэша, и порядок здесь
	// имеет значение.
	//
	// `/api/*` торчит наружу на afisha.vshage.app, и ключ, посчитанный по
	// непроверенному значению, — это ключевое пространство редиса, которым
	// распоряжается кто угодно снаружи. Записей оно не порождает (пишем мы
	// только успешный ответ), но и обращаться в кэш за заведомо отказным
	// запросом незачем. После проверок ключ собирается из СЛОВАРНЫХ значений
	// фильтра и уже нормализованных limit/offset: `?limit=` и `?limit=30` —
	// одна и та же страница, и делить одну запись они обязаны.
	filter, err := ParseFilter(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Часы ставятся ОДИН раз на запрос: три стора ленты, взявшие каждый своё
	// time.Now(), в полночь разрешат «сегодня» в разные дни.
	filter.At = time.Now()

	// Потолки. Раньше они стояли внутри каждого источника и при превышении не
	// обрезали окно, а откатывались к 30 — то есть глубокая страница молча
	// теряла события основного стора и выглядела полной. Отказ лучше тихой
	// полуправды: `?offset=1000` — это ошибка вызывающего, а не повод
	// показать неверную ленту.
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		writeError(w, http.StatusBadRequest,
			"limit больше "+strconv.Itoa(maxPageSize))
		return
	}
	if offset < 0 {
		offset = 0
	}
	// Слияние требует от КАЖДОГО источника его первые offset+limit событий,
	// иначе окно вырезать не из чего (см. MergePage).
	window := offset + limit
	if window > maxWindow {
		writeError(w, http.StatusBadRequest,
			"offset+limit больше "+strconv.Itoa(maxWindow)+" — лента столько не отдаёт")
		return
	}

	// Ключ несёт ВСЕ параметры фильтра. Пока он был
	// `afisha:events:list:<limit>:<offset>`, страница раздела получила бы
	// закэшированную полную ленту — 200, карточки есть, просто чужие, и на
	// минуту одинаково у всех, кто открыл раздел.
	//
	// Списочные ключи никто не инвалидирует — они живут ровно свои 60 секунд
	// (единственный Invalidate ниже чистит карточку события). Значит
	// удлинять TTL при выросшем числе ключей нельзя: раздел, отставший на
	// минуту, человек не заметит, а отставший на пять — заметит.
	key := filter.CacheKey(limit, offset)
	if cached, ok := h.cache.GetList(ctx, key); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}
	result, listErr := h.repo.List(ctx, ListQuery{Limit: window, Offset: 0, Filter: filter})
	if listErr != nil {
		log.Printf("events.List: %v", listErr)
	}

	// Источники не отменяют друг друга. Событие живой веб-регистрации
	// обязано быть видно, даже если запрос к общим таблицам отвалился, — и
	// наоборот. Пустой ответ отдаём только когда молчат все.
	pages := [][]PublicEvent{}
	extraTotal, extraCount := 0, 0
	degraded := []string{}
	if listErr != nil {
		degraded = append(degraded, "main")
	}
	since := time.Now().Add(-24 * time.Hour)
	for _, src := range h.extra {
		name := sourceName(src)
		page, err := src.UpcomingForAfisha(ctx, since, filter, window, 0)
		if err != nil {
			log.Printf("events.List: источник %s: %v", name, err)
			degraded = append(degraded, name)
			continue
		}
		n, err := src.CountUpcomingForAfisha(ctx, since, filter)
		if err != nil {
			// Считать «сколько всего» и «отдать страницу» — разные запросы, и
			// отказ первого не повод прятать второй: берём хотя бы то, что
			// видим сами. Но молча занижать total нельзя — это видимое
			// человеку число («ВСЕ СОБЫТИЯ · N»), и заниженное выглядит
			// достоверным.
			log.Printf("events.List: счётчик источника %s: %v", name, err)
			degraded = append(degraded, name+":count")
			n = len(page)
		}
		pages = append(pages, page)
		extraTotal += n
		extraCount += len(page)
	}

	if listErr != nil && extraCount == 0 {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	if listErr != nil {
		result = ListResult{Featured: []PublicEvent{}, All: []PublicEvent{}}
	}
	pages = append([][]PublicEvent{result.All}, pages...)
	// Ключ слияния обязан совпасть с ORDER BY каждого источника: полоса «идёт
	// сейчас» отсортирована по КОНЦУ (сверху то, что закрывается раньше), всё
	// остальное — по началу. Разойдись они, окно [offset, offset+limit)
	// перестало бы быть честным: событие, стоящее у источника за окном, могло
	// бы оказаться в нём по другому ключу — то есть страница потеряла бы
	// карточки и выглядела бы полной.
	result.All = MergePage(pages, limit, offset, filter.Kind == KindRunning)
	result.Total += extraTotal
	result.Degraded = degraded
	if !filter.IsEmpty() {
		// Закрепление — свойство доски города, а не раздела. Общий стор его
		// уже не отдал (см. Repository.List), но источники о featured вообще
		// ничего не знают, и правило должно держаться в одном месте.
		result.Featured = []PublicEvent{}
	}
	if result.Total > maxWindow {
		if sec := time.Now().Unix(); sec-overWindowLoggedAt.Load() > 60 {
			overWindowLoggedAt.Store(sec)
			log.Printf("events.List: доска переросла окно листания — total=%d при maxWindow=%d: события за окном недостижимы из интерфейса, окно пора поднимать",
				result.Total, maxWindow)
		}
	}

	// Деградированный ответ НЕ кэшируем. Иначе разовая икота одного стора
	// замерзает в редисе на минуту и раздаётся всем — включая те секунды,
	// когда база уже здорова, а причина уже уехала из логов. И именно такой
	// ответ невозможно отличить от честной ленты: 200, события есть, просто
	// не все.
	if len(degraded) == 0 {
		h.cache.SetList(ctx, key, result, 60*time.Second)
	}
	writeJSON(w, http.StatusOK, result)
}

// Facets — счётчики доски под текущим фильтром: сколько в каждом разделе,
// сколько сегодня/завтра/на выходных, сколько бесплатных, сколько по городам.
//
// Число рядом с плиткой — обещание: столько карточек откроется по клику.
// Держится оно тем, что список и счётчики строятся ОДНИМИ выражениями (см.
// StoreSQL и CountFacets), а каждое измерение считается с прочими условиями
// фильтра, но без своего собственного — иначе на странице «завтра» все
// остальные дни показали бы ноль и человек решил бы, что событий нет.
//
// Ответ намеренно НЕ кэшируется: считающие запросы дешёвые (сотни строк), а
// кэш фасетов потребовал бы своего ключа и своей инвалидации рядом с
// ключом списка — двух кэшей одной ленты, которые расходятся первыми.
func (h *Handler) Facets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	filter, err := ParseFilter(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	filter.At = time.Now()
	since := filter.At.Add(-24 * time.Hour)

	agg := NewFacets()
	degraded := []string{}
	answered := 0
	if main, err := h.repo.Facets(ctx, since, filter); err != nil {
		log.Printf("events.Facets: %v", err)
		degraded = append(degraded, "main")
	} else {
		agg.Merge(main)
		answered++
	}
	for _, src := range h.extra {
		name := sourceName(src)
		part, err := src.FacetsForAfisha(ctx, since, filter)
		if err != nil {
			log.Printf("events.Facets: источник %s: %v", name, err)
			degraded = append(degraded, name)
			continue
		}
		agg.Merge(part)
		answered++
	}
	// Молчат все — отказ. Молчит один — числа занижены, и об этом сказано в
	// degraded: заниженное число выглядит достоверным, отличить его от
	// честного нечем.
	if answered == 0 {
		writeError(w, http.StatusInternalServerError, "facets failed")
		return
	}
	writeJSON(w, http.StatusOK, facetsResponse(filter, agg, degraded))
}

// categoryOrder — канонический порядок словаря ленты. Нужен тай-брейком:
// при равных счётчиках плитки иначе переставлялись бы между запросами, и
// человек читал бы это как «страница дёргается».
var categoryOrder = func() map[string]int {
	m := make(map[string]int, len(FeedCategories))
	for i, c := range FeedCategories {
		m[c] = i
	}
	return m
}()

func facetsResponse(f Filter, agg Facets, degraded []string) FacetsResponse {
	cities := make([]CityCount, 0, len(Cities()))
	for _, c := range Cities() {
		cities = append(cities, CityCount{City: c, Count: agg.Cities[c.Slug]})
	}
	// Только непустые разделы: плитка с нулём — это тупик, по которому
	// человек кликает и получает пустой экран.
	cats := make([]CategoryCount, 0, len(agg.Categories))
	for code, n := range agg.Categories {
		if n <= 0 {
			continue
		}
		cats = append(cats, CategoryCount{Code: code, Count: n})
	}
	sort.SliceStable(cats, func(i, j int) bool {
		if cats[i].Count != cats[j].Count {
			return cats[i].Count > cats[j].Count
		}
		return categoryOrder[cats[i].Code] < categoryOrder[cats[j].Code]
	})
	when := make(map[string]int, len(WhenCodes))
	for _, code := range WhenCodes {
		when[code] = agg.When[code]
	}
	// Обе полосы всегда, даже нулевые — в отличие от плиток разделов. Пилюля
	// раздела с нулём это тупик, который лучше не показывать; пилюля полосы —
	// переключатель из двух положений, и пропавшая половина читается как
	// «полос нет», а не как «в этой полосе пусто».
	kind := make(map[string]int, len(KindCodes))
	for _, code := range KindCodes {
		kind[code] = agg.Kind[code]
	}
	return FacetsResponse{
		City:       f.City,
		Cities:     cities,
		Total:      agg.Total,
		Categories: cats,
		When:       when,
		Kind:       kind,
		Free:       agg.Free,
		Degraded:   degraded,
	}
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}

	// Один момент на запрос: он идёт и в ключ кэша, и в классификатор полосы.
	// Два вызова часов дали бы карточке ключ сегодняшнего дня и полосу
	// вчерашнего, если между ними прошла полночь.
	now := time.Now()
	if cached, ok := h.cache.GetEvent(ctx, id, now); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	ev, err := h.repo.GetByID(ctx, id, now)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		log.Printf("events.GetByID(%s): %v", id, err)
		writeError(w, http.StatusInternalServerError, "get failed")
		return
	}
	h.cache.SetEvent(ctx, *ev, 5*time.Minute, now)
	writeJSON(w, http.StatusOK, ev)
}

func (h *Handler) RegisterPublic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}

	var input PublicRegistrationInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeRegistrationError(w, &RegistrationError{
			Status:  http.StatusBadRequest,
			Code:    "invalid_json",
			Message: "Некорректные данные формы",
		})
		return
	}

	result, err := h.repo.RegisterPublic(ctx, id, input)
	if err != nil {
		var regErr *RegistrationError
		if errors.As(err, &regErr) {
			writeRegistrationError(w, regErr)
			return
		}
		log.Printf("events.RegisterPublic(%s): %v", id, err)
		writeError(w, http.StatusInternalServerError, "registration failed")
		return
	}

	h.cache.InvalidateEvent(ctx, id, time.Now())
	status := http.StatusCreated
	if result.AlreadyRegistered {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeRegistrationError(w http.ResponseWriter, err *RegistrationError) {
	writeJSON(w, err.Status, err)
}
