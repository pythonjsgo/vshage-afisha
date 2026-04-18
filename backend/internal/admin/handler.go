package admin

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	pool *pgxpool.Pool
}

func NewHandler(p *pgxpool.Pool) *Handler {
	return &Handler{pool: p}
}

type featureReq struct {
	EventID  string `json:"event_id"`
	Position int    `json:"position"`
}

func (h *Handler) Feature(w http.ResponseWriter, r *http.Request) {
	var req featureReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.EventID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	pos := req.Position
	if pos == 0 {
		pos = 100
	}
	user := UserFromCtx(r.Context())

	_, err := h.pool.Exec(r.Context(), `
		INSERT INTO afisha_featured (event_id, position, pinned_at, pinned_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (event_id) DO UPDATE
		SET position = EXCLUDED.position, pinned_at = EXCLUDED.pinned_at, pinned_by = EXCLUDED.pinned_by
	`, req.EventID, pos, time.Now(), user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Unfeature(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventID string `json:"event_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.EventID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	_, err := h.pool.Exec(r.Context(),
		`DELETE FROM afisha_featured WHERE event_id = $1`, req.EventID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
