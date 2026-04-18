package events

import (
	"encoding/json"
	"errors"
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
		writeError(w, http.StatusInternalServerError, "get failed")
		return
	}
	h.cache.SetEvent(ctx, *ev, 5*time.Minute)
	writeJSON(w, http.StatusOK, ev)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
