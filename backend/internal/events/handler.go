package events

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type Handler struct {
	repo  *Repository
	cache *Cache
}

func NewHandler(r *Repository, c *Cache) *Handler {
	return &Handler{repo: r, cache: c}
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

	result, err := h.repo.List(ctx, ListQuery{Limit: limit, Offset: offset})
	if err != nil {
		log.Printf("events.List: %v", err)
		writeError(w, http.StatusInternalServerError, "list failed")
		return
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
