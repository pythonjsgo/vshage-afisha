package join

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// maxBody — потолок тела запроса. Анкета укладывается в единицы килобайт;
// всё, что больше, — не человек.
const maxBody = 16 << 10

// adminListLimit — потолок выдачи модератору. Отдаётся вместе с данными, и
// клиент обязан листать: серверный дефолт лимита — это усечение, а усечение
// без числа рядом читается как полнота (кабинет уже показывал так половину
// записавшихся).
const adminListLimit = 200

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler { return &Handler{repo: repo} }

// apiError повторяет форму ошибки веб-регистрации (internal/webreg): один
// разбор на фронте на обе формы.
type apiError struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, e apiError) {
	writeJSON(w, e.Status, e)
}

// Routes — публичная ручка. limit применяется ТОЛЬКО здесь: это единственная
// запись без авторизации на всей поверхности пакета.
func (h *Handler) Routes(r chi.Router, limit func(http.Handler) http.Handler) {
	r.With(limit).Post("/", h.Submit)
}

// AdminRoutes — разбор анкет. Гейт — общий админский JWT афиши, он же у
// featured (см. cmd/server/main.go).
func (h *Handler) AdminRoutes(r chi.Router) {
	r.Get("/", h.AdminList)
	r.Patch("/{id}", h.AdminPatch)
}

func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	var in Submission
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&in); err != nil {
		writeErr(w, apiError{Status: http.StatusBadRequest, Code: "bad_json",
			Message: "Не удалось прочитать форму"})
		return
	}

	if strings.TrimSpace(in.HP) != "" {
		// Ответ неотличим от успеха — это осознанно: скажи боту «отказ», и
		// автор поправит скрипт, а страница получит вторую волну уже без
		// ловушки. Но выброс обязан быть ВОССТАНОВИМЫМ: ложное срабатывание
		// (менеджер паролей, автозаполнение) иначе уничтожает оплаченного
		// человека бесследно — он видит «получили», а у нас нет ни строки.
		// Поэтому в лог уходит всё, чем с ним можно связаться.
		log.Printf("join: honeypot сработал — имя=%q тг=%q вуз=%q кампания=%q ua=%q",
			clip(in.Name, 80), clip(in.Telegram, 40), clip(in.University, 40),
			clip(in.UTMCampaign, 80), clip(r.UserAgent(), 120))
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
		return
	}

	clean, fieldErrs := Validate(in)
	if fieldErrs != nil {
		writeErr(w, apiError{Status: http.StatusBadRequest, Code: "validation_failed",
			Message: "Проверь заполненные поля", Fields: fieldErrs})
		return
	}
	clean.IP = clientIP(r)
	clean.UserAgent = clip(r.UserAgent(), maxMeta)

	id, err := h.repo.Create(r.Context(), clean)
	switch {
	case errors.Is(err, ErrDuplicate):
		// 409 с отдельным кодом, а не общий 400: фронт показывает спокойное
		// «анкета уже есть», и это не ошибка ввода.
		writeErr(w, apiError{Status: http.StatusConflict, Code: "already_submitted",
			Message: "Анкета уже есть — мы её получили, напишем в телеграм"})
		return
	case err != nil:
		// Логируем ПРИЧИНУ рядом с кодом ответа: 500 без строки в логе мы уже
		// разбирали сутки (409 при регистрации по почте).
		// Юзернейм в лог НЕ пишем: это персональные данные, а лог живёт
		// вечно и читается кем угодно. Для разбирательства хватает адреса
		// (хешем) и того, что ошибка вообще названа.
		log.Printf("join.Create: %v (ip_hash=%s…)", err, clean.hashHint())
		writeErr(w, apiError{Status: http.StatusInternalServerError, Code: "server_error",
			Message: "Не сохранилось. Попробуй ещё раз через минуту"})
		return
	}

	log.Printf("join: анкета #%d принята (вуз=%s курс=%s кампания=%q)",
		id, clean.University, clean.Course, clean.UTMCampaign)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "id": id})
}

func (h *Handler) AdminList(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status != "" && !validStatus(status) {
		writeErr(w, apiError{Status: http.StatusBadRequest, Code: "bad_status",
			Message: "status: new | approved | rejected"})
		return
	}
	limit := adminListLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= adminListLimit {
			limit = n
		}
	}

	rows, err := h.repo.List(r.Context(), status, limit)
	if err != nil {
		log.Printf("join.List: %v", err)
		writeErr(w, apiError{Status: http.StatusInternalServerError, Code: "server_error",
			Message: "Не прочиталось"})
		return
	}
	// limit отдаётся рядом с данными: увидев len(items) == limit, читатель
	// знает, что список усечён, и не примет его за полный.
	writeJSON(w, http.StatusOK, map[string]any{"items": rows, "limit": limit})
}

func (h *Handler) AdminPatch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, apiError{Status: http.StatusBadRequest, Code: "bad_id", Message: "id"})
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&body); err != nil {
		writeErr(w, apiError{Status: http.StatusBadRequest, Code: "bad_json", Message: "тело"})
		return
	}
	if !validStatus(body.Status) {
		writeErr(w, apiError{Status: http.StatusBadRequest, Code: "bad_status",
			Message: "status: new | approved | rejected"})
		return
	}

	ok, err := h.repo.SetStatus(r.Context(), id, body.Status)
	if err != nil {
		log.Printf("join.SetStatus(%d): %v", id, err)
		writeErr(w, apiError{Status: http.StatusInternalServerError, Code: "server_error",
			Message: "Не сохранилось"})
		return
	}
	if !ok {
		writeErr(w, apiError{Status: http.StatusNotFound, Code: "not_found",
			Message: "Анкеты с таким id нет"})
		return
	}
	log.Printf("join: анкета #%d → %s", id, body.Status)
	writeJSON(w, http.StatusOK, map[string]any{"status": body.Status, "id": id})
}

func validStatus(s string) bool {
	switch s {
	case "new", "approved", "rejected":
		return true
	}
	return false
}
