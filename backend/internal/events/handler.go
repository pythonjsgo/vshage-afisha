package events

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// ExtraSource — дополнительный источник событий для ленты афиши.
// Реализуется пакетом webreg: события веб-регистрации живут в своих
// таблицах, но фаундер хочет видеть их в общей афише (директива 17.08).
// Интерфейс объявлен здесь, чтобы webreg зависел от events, а не наоборот.
type ExtraSource interface {
	UpcomingForAfisha(ctx context.Context, since time.Time) ([]PublicEvent, error)
}

type Handler struct {
	repo  *Repository
	cache *Cache
	extra ExtraSource
}

func NewHandler(r *Repository, c *Cache) *Handler {
	return &Handler{repo: r, cache: c}
}

// WithExtraSource подключает дополнительный источник событий (webreg).
// Опционален: без него лента ведёт себя ровно как раньше.
func (h *Handler) WithExtraSource(s ExtraSource) *Handler {
	h.extra = s
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

	result, listErr := h.repo.List(ctx, ListQuery{Limit: limit, Offset: offset})
	if listErr != nil {
		log.Printf("events.List: %v", listErr)
	}

	// Два источника, и ни один не отменяет другой. Событие живой
	// веб-регистрации обязано быть видно, даже если запрос к общим
	// таблицам отвалился, — и наоборот. Пустой ответ отдаём только когда
	// молчат оба.
	var extra []PublicEvent
	if h.extra != nil {
		var extraErr error
		extra, extraErr = h.extra.UpcomingForAfisha(ctx, time.Now().Add(-24*time.Hour))
		if extraErr != nil {
			log.Printf("events.List: extra source: %v", extraErr)
		}
	}

	if listErr != nil && len(extra) == 0 {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	if listErr != nil {
		result = ListResult{Featured: []PublicEvent{}, All: []PublicEvent{}}
	}
	if len(extra) > 0 {
		result.All = append(extra, result.All...)
		sort.SliceStable(result.All, func(i, j int) bool {
			return result.All[i].StartTime.Before(result.All[j].StartTime)
		})
		result.Total += len(extra)
	}
	h.cache.SetList(ctx, key, result, 60*time.Second)
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
