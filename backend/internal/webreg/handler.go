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
	r.Get("/{slug}/t/{code}", h.GetTicket)
	// Check-in carries the organizer's manage key, so it is gated by that
	// rather than by the anonymous write limiter — a door scanning a queue
	// of arrivals from one phone must not be throttled as if it were a flood.
	r.Post("/{slug}/t/{code}/checkin", h.CheckIn)
}

// AdminRoutes mounts the config surface, gated by a static bearer token.
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Use(h.requireAdmin)
	r.Get("/events", h.ListEvents)
	r.Put("/events", h.UpsertEvent)
	r.Get("/events/{slug}", h.GetEventConfig)
	r.Get("/events/{slug}/registrations", h.AdminRegistrations)
	r.Post("/events/{slug}/manage-key", h.RotateManageKey)
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
		"full_name":   clean.FullName,
		"email":       clean.Email,
		"phone":       clean.Phone,
		"tg":          clean.Answers["__tg_display"],
		"affiliation": clean.Affiliation,
		"answers":     withoutInternal(clean.Answers),
		"source":      clean.Source,
		"at":          time.Now().UTC().Format(time.RFC3339),
	})

	res, err := h.repo.Register(r.Context(), ev, clean, clientIP(r), r.UserAgent())
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

	// Only the columns this event actually collects. A sheet padded with
	// empty «Телефон» / «ФИО» columns is what an organizer has to clean up by
	// hand before it can be handed to a venue as a pass list.
	form := list.Form
	header := []string{"#"}
	if form.Name.Enabled {
		header = append(header, "Имя")
	}
	if form.FullName.Enabled {
		header = append(header, "ФИО (для пропуска)")
	}
	if form.Email.Enabled {
		header = append(header, "Почта")
	}
	if form.Phone.Enabled {
		header = append(header, "Телефон")
	}
	if form.TG.Enabled {
		header = append(header, "Телеграм")
	}
	if form.Affiliation.Enabled {
		header = append(header, "Вуз / статус")
	}
	for _, f := range list.Fields {
		header = append(header, f.Label)
	}
	if list.TicketMode != TicketOff && list.TicketMode != "" {
		header = append(header, "Билет", "Пришёл")
	}
	header = append(header, "Время регистрации")
	_ = cw.Write(header)

	for i, reg := range list.Registrations {
		row := []string{fmt.Sprintf("%d", i+1)}
		if form.Name.Enabled {
			row = append(row, reg.Name)
		}
		if form.FullName.Enabled {
			row = append(row, reg.FullName)
		}
		if form.Email.Enabled {
			row = append(row, reg.Email)
		}
		if form.Phone.Enabled {
			row = append(row, reg.Phone)
		}
		if form.TG.Enabled {
			row = append(row, reg.TGDisplay)
		}
		if form.Affiliation.Enabled {
			row = append(row, reg.Affiliation)
		}
		for _, f := range list.Fields {
			row = append(row, reg.Answers[f.Key])
		}
		if list.TicketMode != TicketOff && list.TicketMode != "" {
			arrived := ""
			if reg.CheckedInAt != nil {
				arrived = reg.CheckedInAt.In(loc).Format("15:04")
			}
			row = append(row, reg.TicketCode, arrived)
		}
		row = append(row, reg.CreatedAt.In(loc).Format("02.01.2006 15:04"))
		_ = cw.Write(row)
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		log.Printf("webreg.ManageCSV(%s): flush: %v", list.Slug, err)
	}
}

func (h *Handler) GetTicket(w http.ResponseWriter, r *http.Request) {
	t, err := h.repo.GetTicket(r.Context(), chi.URLParam(r, "slug"), chi.URLParam(r, "code"))
	if err != nil {
		writeLookupError(w, err, "webreg.GetTicket")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) CheckIn(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" {
		// Same 404 as an unknown code: without the organizer key there is
		// nothing to distinguish "you may not do this" from "no such ticket",
		// and saying which would confirm the ticket exists.
		writeAPIError(w, &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "Не найдено"})
		return
	}
	t, err := h.repo.CheckIn(r.Context(), chi.URLParam(r, "slug"), key, chi.URLParam(r, "code"))
	if err != nil {
		writeLookupError(w, err, "webreg.CheckIn")
		return
	}
	h.invalidate(t.EventSlug)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, t)
}

// ownerOf — кабинет, от имени которого пришёл админский запрос.
//
// Полный доступ запрашивается ЯВНО, заголовком `X-Owner-Scope: all`. Забытый
// заголовок не должен означать «можно всё»: поверхность хоть и закрыта
// админским токеном, но она торчит наружу (`/api/*` на afisha.vshage.app), и
// умолчание «нет заголовка → весь список» — это тихий отказ, который выглядит
// как успех. Тот же урок, что и с `{"sent":true}` при мёртвом SMTP.
func ownerOf(r *http.Request) (owner *string, ok bool) {
	if strings.TrimSpace(r.Header.Get("X-Owner-Scope")) == "all" {
		return nil, true
	}
	v := strings.TrimSpace(r.Header.Get("X-Owner-Slug"))
	if v == "" {
		return nil, false
	}
	return &v, true
}

// requireScope отвечает 403 и объясняет, чего не хватило: это не ошибка гостя,
// а неверно собранный внутренний запрос, и читать его будет инженер.
func requireScope(w http.ResponseWriter, r *http.Request) (*string, bool) {
	owner, ok := ownerOf(r)
	if !ok {
		writeAPIError(w, &APIError{Status: http.StatusForbidden, Code: "owner_scope_required",
			Message: "нужен X-Owner-Slug (кабинет) либо X-Owner-Scope: all"})
		return nil, false
	}
	return owner, true
}

func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	owner, ok := requireScope(w, r)
	if !ok {
		return
	}
	list, err := h.repo.ListEvents(r.Context(), owner)
	if err != nil {
		log.Printf("webreg.ListEvents: %v", err)
		writeAPIError(w, &APIError{Status: http.StatusServiceUnavailable, Code: "unavailable",
			Message: "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) GetEventConfig(w http.ResponseWriter, r *http.Request) {
	owner, ok := requireScope(w, r)
	if !ok {
		return
	}
	cfg, err := h.repo.GetEventConfig(r.Context(), chi.URLParam(r, "slug"), owner)
	if err != nil {
		writeLookupError(w, err, "webreg.GetEventConfig")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// AdminRegistrations is the same list the organizer sees, reachable with the
// admin token instead of the per-event secret — so the panel can show any
// event's attendees without the operator hunting down its manage link.
func (h *Handler) AdminRegistrations(w http.ResponseWriter, r *http.Request) {
	owner, ok := requireScope(w, r)
	if !ok {
		return
	}
	list, err := h.repo.AdminList(r.Context(), chi.URLParam(r, "slug"), owner)
	if err != nil {
		writeLookupError(w, err, "webreg.AdminRegistrations")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, list)
}

// RotateManageKey issues a fresh organizer link and invalidates the old one.
// The plaintext key is returned exactly once, here — it is never stored.
func (h *Handler) RotateManageKey(w http.ResponseWriter, r *http.Request) {
	owner, ok := requireScope(w, r)
	if !ok {
		return
	}
	slug := chi.URLParam(r, "slug")
	key, err := newManageKey()
	if err != nil {
		log.Printf("webreg.RotateManageKey(%s): %v", slug, err)
		writeAPIError(w, &APIError{Status: http.StatusInternalServerError, Code: "rotate_failed",
			Message: "rotate failed"})
		return
	}
	if err := h.repo.SetManageKey(r.Context(), slug, key, owner); err != nil {
		writeLookupError(w, err, "webreg.RotateManageKey")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"slug": slug, "manage_key": key})
}

func (h *Handler) UpsertEvent(w http.ResponseWriter, r *http.Request) {
	owner, ok := requireScope(w, r)
	if !ok {
		return
	}
	var in EventUpsert
	if err := decodeJSON(w, r, &in); err != nil {
		writeAPIError(w, err)
		return
	}
	if err := h.repo.UpsertEvent(r.Context(), &in, owner); err != nil {
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
