package events

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
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
type ExtraSource interface {
	UpcomingForAfisha(ctx context.Context, since time.Time, limit, offset int) ([]PublicEvent, error)
	CountUpcomingForAfisha(ctx context.Context, since time.Time) (int, error)
}

// Потолки страницы. Раньше их роль играли разрозненные `> 100 → 30` внутри
// источников: не потолок, а тихая подмена запрошенного окна.
const (
	defaultPageSize = 30
	maxPageSize     = 100
	maxWindow       = 300
)

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

	key := "afisha:events:list:" + strconv.Itoa(limit) + ":" + strconv.Itoa(offset)
	if cached, ok := h.cache.GetList(ctx, key); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	// Вход валидируется ЗДЕСЬ и явно. Раньше потолки стояли внутри каждого
	// источника и при превышении не обрезали окно, а откатывались к 30 —
	// то есть глубокая страница молча теряла события основного стора и
	// выглядела полной. Отказ лучше тихой полуправды: `?offset=1000` — это
	// ошибка вызывающего, а не повод показать неверную ленту.
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
	result, listErr := h.repo.List(ctx, ListQuery{Limit: window, Offset: 0})
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
		page, err := src.UpcomingForAfisha(ctx, since, window, 0)
		if err != nil {
			log.Printf("events.List: источник %s: %v", name, err)
			degraded = append(degraded, name)
			continue
		}
		n, err := src.CountUpcomingForAfisha(ctx, since)
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
	result.All = MergePage(pages, limit, offset)
	result.Total += extraTotal
	result.Degraded = degraded

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

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id required")
		return
	}

	if cached, ok := h.cache.GetEvent(ctx, id); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	ev, err := h.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		log.Printf("events.GetByID(%s): %v", id, err)
		writeError(w, http.StatusInternalServerError, "get failed")
		return
	}
	h.cache.SetEvent(ctx, *ev, 5*time.Minute)
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

	h.cache.Invalidate(ctx, "afisha:events:"+id)
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
