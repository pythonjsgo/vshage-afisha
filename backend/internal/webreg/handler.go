package webreg

import (
	"crypto/subtle"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// maxBodyBytes caps a signup payload. Generous for a form, small enough that
// a flood of oversized bodies cannot exhaust memory.
const maxBodyBytes = 32 << 10 // 32 KiB

// eventCacheTTL keeps an announcement burst off the database: thousands of
// page opens collapse into ~4 event queries per minute. The registration
// counter is at most this stale, which is fine for a "N человек идут" line.
const eventCacheTTL = 15 * time.Second

type Handler struct {
	repo       *Repository
	adminToken string
	submitLog  *SubmitLog

	mu    sync.RWMutex
	cache map[string]cachedEvent
}

type cachedEvent struct {
	ev  *Event
	exp time.Time
}

func NewHandler(repo *Repository, adminToken string, submitLog *SubmitLog) *Handler {
	return &Handler{
		repo:       repo,
		adminToken: adminToken,
		submitLog:  submitLog,
		cache:      map[string]cachedEvent{},
	}
}

// Routes mounts the public surface. Mounted under /api by the server, so the
// live paths are /api/e/{slug}, /api/e/{slug}/register, …
//
// writeLimit is applied to the two anonymous write endpoints only; reads stay
// unlimited on purpose (see the note at the call site in cmd/server/main.go).
func (h *Handler) Routes(r chi.Router, writeLimit func(http.Handler) http.Handler) {
	r.Get("/{slug}", h.GetEvent)
	r.With(writeLimit).Post("/{slug}/register", h.Register)
	r.With(writeLimit).Post("/{slug}/waitlist", h.Waitlist)
	r.Get("/{slug}/manage", h.Manage)
	r.Get("/{slug}/manage.csv", h.ManageCSV)
}

// AdminRoutes mounts the config surface, gated by a static bearer token.
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Use(h.requireAdmin)
	r.Put("/events", h.UpsertEvent)
}

func (h *Handler) GetEvent(w http.ResponseWriter, r *http.Request) {
	ev, err := h.event(r, chi.URLParam(r, "slug"))
	if err != nil {
		writeLookupError(w, err, "events.GetEvent")
		return
	}
	// Short shared cache: Caddy/browsers may serve a slightly stale page
	// during a burst, and revalidate in the background afterwards.
	w.Header().Set("Cache-Control", "public, max-age=15, stale-while-revalidate=120")
	writeJSON(w, http.StatusOK, ev)
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	ev, err := h.event(r, slug)
	if err != nil {
		writeLookupError(w, err, "webreg.Register lookup")
		return
	}
	if !ev.RegistrationOpen {
		writeAPIError(w, &APIError{Status: http.StatusConflict, Code: "registration_closed",
			Message: "Регистрация на это событие закрыта"})
		return
	}

	var in RegisterInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeAPIError(w, err)
		return
	}

	clean, apiErr := validateRegistration(ev, &in)
	if apiErr != nil {
		writeAPIError(w, apiErr)
		return
	}

	// Durable copy BEFORE the write. If the database is unreachable the
	// visitor still exists in the container log (json-file on the host), so
	// a submission is recoverable rather than gone.
	h.submitLog.Record(map[string]any{
		"kind":        "registration",
		"event":       slug,
		"name":        clean.Name,
		"tg":          clean.Answers["__tg_display"],
		"affiliation": clean.Affiliation,
		"answers":     withoutInternal(clean.Answers),
		"source":      clean.Source,
		"at":          time.Now().UTC().Format(time.RFC3339),
	})

	res, err := h.repo.Register(r.Context(), slug, clean, clientIP(r), r.UserAgent())
	if err != nil {
		log.Printf("webreg.Register(%s): %v", slug, err)
		writeAPIError(w, &APIError{Status: http.StatusServiceUnavailable, Code: "register_failed",
			Message: "Не смогли записать — попробуй ещё раз через пару секунд"})
		return
	}

	h.invalidate(slug)
	status := http.StatusCreated
	if res.AlreadyRegistered {
		status = http.StatusOK
	}
	writeJSON(w, status, res)
}

func (h *Handler) Waitlist(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	var in WaitlistInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeAPIError(w, err)
		return
	}
	platform := strings.ToLower(strings.TrimSpace(in.Platform))
	if platform != "android" && platform != "ios" {
		writeAPIError(w, &APIError{Status: http.StatusBadRequest, Code: "platform_invalid",
			Message: "Неизвестная платформа"})
		return
	}
	key, display, ok := NormalizeTG(in.TGUsername)
	if !ok {
		writeAPIError(w, &APIError{Status: http.StatusBadRequest, Code: "validation_failed",
			Message: "Нужен юзернейм из Телеграма — например @ivanov",
			Fields:  map[string]string{"tg_username": "Например @ivanov"}})
		return
	}

	h.submitLog.Record(map[string]any{
		"kind":     "waitlist",
		"event":    slug,
		"platform": platform,
		"tg":       display,
		"at":       time.Now().UTC().Format(time.RFC3339),
	})

	if err := h.repo.AddWaitlist(r.Context(), slug, platform, key, display, collapseSpaces(in.Name)); err != nil {
		log.Printf("webreg.Waitlist(%s): %v", slug, err)
		writeAPIError(w, &APIError{Status: http.StatusServiceUnavailable, Code: "waitlist_failed",
			Message: "Не смогли записать — попробуй ещё раз"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "platform": platform})
}

func (h *Handler) Manage(w http.ResponseWriter, r *http.Request) {
	list, err := h.manageList(r)
	if err != nil {
		writeLookupError(w, err, "webreg.Manage")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) ManageCSV(w http.ResponseWriter, r *http.Request) {
	list, err := h.manageList(r)
	if err != nil {
		writeLookupError(w, err, "webreg.ManageCSV")
		return
	}

	loc := location(list.Timezone)
	sep := ';'
	if r.URL.Query().Get("sep") == "," {
		sep = ','
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s-registrations.csv"`, list.Slug))

	// UTF-8 BOM: without it Excel on Windows/RU renders Cyrillic as mojibake.
	if _, err := io.WriteString(w, "\ufeff"); err != nil {
		return
	}
	cw := csv.NewWriter(w)
	cw.Comma = sep

	header := []string{"#", "Имя", "Телеграм", "Вуз / статус"}
	for _, f := range list.Fields {
		header = append(header, f.Label)
	}
	header = append(header, "Время регистрации")
	_ = cw.Write(header)

	for i, reg := range list.Registrations {
		row := []string{
			fmt.Sprintf("%d", i+1),
			reg.Name,
			reg.TGDisplay,
			reg.Affiliation,
		}
		for _, f := range list.Fields {
			row = append(row, reg.Answers[f.Key])
		}
		row = append(row, reg.CreatedAt.In(loc).Format("02.01.2006 15:04"))
		_ = cw.Write(row)
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		log.Printf("webreg.ManageCSV(%s): flush: %v", list.Slug, err)
	}
}

func (h *Handler) UpsertEvent(w http.ResponseWriter, r *http.Request) {
	var in EventUpsert
	if err := decodeJSON(w, r, &in); err != nil {
		writeAPIError(w, err)
		return
	}
	if err := h.repo.UpsertEvent(r.Context(), &in); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			writeAPIError(w, apiErr)
			return
		}
		log.Printf("webreg.UpsertEvent(%s): %v", in.Slug, err)
		writeAPIError(w, &APIError{Status: http.StatusInternalServerError, Code: "upsert_failed",
			Message: "upsert failed"})
		return
	}
	h.invalidate(in.Slug)
	log.Printf("webreg: event %q upserted", in.Slug)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "slug": in.Slug})
}

// ── helpers ──────────────────────────────────────────────────────────────

func (h *Handler) manageList(r *http.Request) (*ManageList, error) {
	slug := chi.URLParam(r, "slug")
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" {
		return nil, &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "Не найдено"}
	}
	return h.repo.ManageList(r.Context(), slug, key)
}

// event reads through a small in-process cache.
func (h *Handler) event(r *http.Request, slug string) (*Event, error) {
	h.mu.RLock()
	hit, ok := h.cache[slug]
	h.mu.RUnlock()
	if ok && time.Now().Before(hit.exp) {
		return hit.ev, nil
	}

	ev, err := h.repo.GetEvent(r.Context(), slug)
	if err != nil {
		return nil, err
	}

	h.mu.Lock()
	// Bound the map so an unbounded stream of bogus slugs cannot grow it.
	// Misses are cheap; a real deployment has a handful of live events.
	if len(h.cache) > 256 {
		h.cache = map[string]cachedEvent{}
	}
	h.cache[slug] = cachedEvent{ev: ev, exp: time.Now().Add(eventCacheTTL)}
	h.mu.Unlock()
	return ev, nil
}

func (h *Handler) invalidate(slug string) {
	h.mu.Lock()
	delete(h.cache, slug)
	h.mu.Unlock()
}

func (h *Handler) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-Admin-Token")
		if h.adminToken == "" || subtle.ConstantTimeCompare([]byte(got), []byte(h.adminToken)) != 1 {
			writeAPIError(w, &APIError{Status: http.StatusUnauthorized, Code: "unauthorized",
				Message: "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) *APIError {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := dec.Decode(dst); err != nil {
		return &APIError{Status: http.StatusBadRequest, Code: "invalid_json",
			Message: "Не смогли прочитать форму — обнови страницу и попробуй снова"}
	}
	return nil
}

func withoutInternal(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if strings.HasPrefix(k, "__") {
			continue
		}
		out[k] = v
	}
	return out
}

// clientIP prefers the proxy-set header; Caddy sets X-Forwarded-For and chi's
// RealIP middleware already promotes it to RemoteAddr, so this is a fallback.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		host = host[:i]
	}
	return host
}

func location(tz string) *time.Location {
	if tz == "" {
		tz = "Europe/Moscow"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("webreg: encode response: %v", err)
	}
}

func writeAPIError(w http.ResponseWriter, e *APIError) {
	if e == nil {
		return
	}
	status := e.Status
	if status == 0 {
		status = http.StatusBadRequest
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, e)
}

// writeLookupError maps a repository error onto the wire. A wrong manage key
// and an unknown slug both produce 404 on purpose — a probe cannot tell an
// existing private list from a nonexistent one.
func writeLookupError(w http.ResponseWriter, err error, what string) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		writeAPIError(w, apiErr)
		return
	}
	if IsNotFound(err) {
		writeAPIError(w, &APIError{Status: http.StatusNotFound, Code: "not_found",
			Message: "Событие не найдено"})
		return
	}
	log.Printf("%s: %v", what, err)
	writeAPIError(w, &APIError{Status: http.StatusServiceUnavailable, Code: "unavailable",
		Message: "Сервис временно недоступен — обнови страницу"})
}
