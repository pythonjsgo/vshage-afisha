package tgevents

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// Партия — payload полной карточки на каждое событие; после осеннего
// пересъёма (когда предстоящим станет весь свежий корпус) сотни карточек
// весят единицы МБ. 16 МБ — запас на порядок, но не открытые ворота.
const maxBodyBytes = 16 << 20

// Витрина московская, а контейнер живёт в UTC: «сегодня» без сдвига держало
// бы вчерашние события до 03:00 МСК. FixedZone вместо LoadLocation — образ
// без tzdata не должен молча съехать обратно в UTC.
var msk = time.FixedZone("MSK", 3*60*60)

type Handler struct {
	repo *Repository
	// adminToken переиспользует WEBREG_ADMIN_TOKEN — один админ афиши, один
	// секрет. Пустой токен = поверхность выключена (fail closed, как webreg).
	adminToken string
}

func NewHandler(repo *Repository, adminToken string) *Handler {
	return &Handler{repo: repo, adminToken: adminToken}
}

// Get — GET /api/tg-events/{id}: одна карточка в контракте ленты
// (events.PublicEvent), чтобы страница события открывалась тем же кодом, что
// и у остальных. Отдаём ИМЕННО PublicEvent, а не Card: у Card наружу торчит
// payload с чужим текстом поста, и один невнимательный маршалинг сделал бы
// его публичным.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	ev, err := h.repo.GetByID(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		log.Printf("tgevents.Get: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
	writeJSON(w, http.StatusOK, ev)
}

// Cover — GET /api/tg-events/{id}/cover: байты обложки со своего origin.
func (h *Handler) Cover(w http.ResponseWriter, r *http.Request) {
	data, mime, err := h.repo.Cover(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, ErrNoCover) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		log.Printf("tgevents.Cover: %v", err)
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	// Обложка меняется только импортом и только вместе с карточкой — кэшу
	// можно жить сутки, битая картинка лечится следующим импортом сама.
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
	if _, err := w.Write(data); err != nil {
		log.Printf("tgevents.Cover write: %v", err)
	}
}

// AdminBulkUpsert — PUT /api/tg-events/admin/bulk: идемпотентная заливка
// партии карточек импортёром (vshage-geo/collect/export_afisha.py, запуск
// с DEV-хоста — токен не покидает хост).
func (h *Handler) AdminBulkUpsert(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	var in struct {
		Events []Card `json:"events"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := dec.Decode(&in); err != nil {
		// Переполнение лимита — не «битый JSON»: скажи импортёру правду,
		// иначе оператор ищет несуществующий дефект сериализации.
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeJSON(w, http.StatusRequestEntityTooLarge,
				map[string]string{"error": fmt.Sprintf("too_large: партия больше %d МБ — бей на части", maxBodyBytes>>20)})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if len(in.Events) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty_batch"})
		return
	}
	// Брак партии — 400 с именем карточки; всё, что валидацию прошло и всё
	// равно упало, — 500: инфраструктура, ретрай уместен.
	for i, c := range in.Events {
		if err := c.Validate(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("карточка #%d (%s): %v", i, c.ID, err)})
			return
		}
	}
	n, err := h.repo.UpsertBulk(r.Context(), in.Events)
	if err != nil {
		// Текст ошибки уходит импортёру: поверхность под токеном, читать
		// ответ будет инженер, а не гость.
		log.Printf("tgevents.AdminBulkUpsert: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	log.Printf("tgevents: импорт применён, карточек: %d", n)
	writeJSON(w, http.StatusOK, map[string]int{"upserted": n})
}

// requireAdmin — единственный вход в админ-поверхность tg-событий. Общий на
// все админ-ручки сознательно: разойдись проверки, одна из ручек однажды
// осталась бы без неё, и заметить это было бы нечем — 200 выглядит одинаково.
// Пустой токен = поверхность выключена (fail closed, как у webreg).
func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	got := r.Header.Get("X-Admin-Token")
	if h.adminToken == "" || subtle.ConstantTimeCompare([]byte(got), []byte(h.adminToken)) != 1 {
		// Строка в логе — иначе оператор, дебажащий 401 импортёра, не увидит
		// связи с незаданным WEBREG_ADMIN_TOKEN.
		log.Printf("tgevents: админ-запрос отвергнут (токен %s)",
			map[bool]string{true: "не настроен", false: "не совпал"}[h.adminToken == ""])
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}
	return true
}

// maxPatchBytes — тело курации это три булевых поля. Всё, что больше, прислано
// по ошибке, и читать это в память незачем.
const maxPatchBytes = 4 << 10

// AdminPatch — PATCH /api/tg-events/admin/{id}: курация витрины ленты
// приложения. Тело — любое подмножество {feed, anchor, hidden}; неназванное
// поле не трогается. Именно частичная правка, а не подмена строки: снятие с
// витрины и допуск на полку — разные решения, и одно не должно возвращать
// другому умолчание.
func (h *Handler) AdminPatch(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	var in struct {
		Feed   *bool `json:"feed"`
		Anchor *bool `json:"anchor"`
		Hidden *bool `json:"hidden"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPatchBytes))
	// Незнакомое поле — 400, а не молчаливое игнорирование: опечатка в
	// "anchor" иначе вернула бы 200 с неизменившейся строкой, то есть успех
	// на невыполненной команде. Курация правит руками — она эту опечатку не
	// увидит нигде, кроме ответа.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeJSON(w, http.StatusRequestEntityTooLarge,
				map[string]string{"error": fmt.Sprintf("too_large: тело больше %d КБ", maxPatchBytes>>10)})
			return
		}
		writeJSON(w, http.StatusBadRequest,
			map[string]string{"error": fmt.Sprintf("invalid_json: %v", err)})
		return
	}
	if in.Feed == nil && in.Anchor == nil && in.Hidden == nil {
		writeJSON(w, http.StatusBadRequest,
			map[string]string{"error": "empty_patch: нужно хотя бы одно из feed/anchor/hidden"})
		return
	}
	st, err := h.repo.AdminSetFlags(r.Context(), chi.URLParam(r, "id"),
		AdminFlags{Feed: in.Feed, Anchor: in.Anchor, Hidden: in.Hidden})
	if errors.Is(err, ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		log.Printf("tgevents.AdminPatch %s: %v", chi.URLParam(r, "id"), err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	// Курация — ручное решение о том, что увидят люди: пишем в лог, кто во
	// что превратился. Иначе «почему это событие в ленте» не восстановить.
	log.Printf("tgevents: курация %s → feed=%v anchor=%v hidden=%v",
		st.ID, st.Feed, st.Anchor, st.Hidden)
	writeJSON(w, http.StatusOK, st)
}

// AdminList — GET /api/tg-events/admin/list: плоский список для курации.
// Ни annonce, ни payload наружу не идут: во втором дословный чужой пост, а
// первый в списке не нужен — решение принимается по заголовку, дате и городу.
func (h *Handler) AdminList(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	items, err := h.repo.AdminList(r.Context())
	if err != nil {
		log.Printf("tgevents.AdminList: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	// Список меняется руками и читается человеком в момент правки —
	// кэшировать его нельзя ни на секунду, иначе он покажет доправку.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, items)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("tgevents.writeJSON: %v", err)
	}
}
