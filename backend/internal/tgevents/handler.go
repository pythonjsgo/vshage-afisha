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

// List — GET /api/tg-events: предстоящие события витрины.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	cards, err := h.repo.ListUpcoming(r.Context(), time.Now().In(msk), 100)
	if err != nil {
		log.Printf("tgevents.List: %v", err)
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "internal"})
		return
	}
	// Тот же короткий shared-кэш, что у страницы событий: витрина обновляется
	// импортом раз в дни, но врать дольше пары минут незачем.
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=300")
	writeJSON(w, http.StatusOK, map[string]any{
		"events": cards,
		"count":  len(cards),
	})
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
	got := r.Header.Get("X-Admin-Token")
	if h.adminToken == "" || subtle.ConstantTimeCompare([]byte(got), []byte(h.adminToken)) != 1 {
		// Строка в логе — иначе оператор, дебажащий 401 импортёра, не увидит
		// связи с незаданным WEBREG_ADMIN_TOKEN.
		log.Printf("tgevents: админ-запрос отвергнут (токен %s)",
			map[bool]string{true: "не настроен", false: "не совпал"}[h.adminToken == ""])
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("tgevents.writeJSON: %v", err)
	}
}
