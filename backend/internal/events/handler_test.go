package events

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Неизвестный раздел получает ОТКАЗ с текстом, а не полную ленту. Проверка
// идёт ДО кэша — потому и обходится без базы и редиса: если однажды разбор
// уедет за обращение к кэшу, тест упадёт паникой, и это правильный сигнал.
func TestListRefusesUnknownFilterBeforeTouchingAnything(t *testing.T) {
	h := NewHandler(nil, nil)
	for _, query := range []string{"?category=koncert", "?when=tonight", "?city=spb", "?free=true"} {
		req := httptest.NewRequest(http.MethodGet, "/api/events"+query, nil)
		w := httptest.NewRecorder()
		h.List(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: код %d, ожидали 400", query, w.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: ответ не разбирается: %s", query, w.Body.String())
		}
		if strings.TrimSpace(body["error"]) == "" {
			t.Errorf("%s: отказ без текста — вызывающему нечего прочитать", query)
		}
	}
}

func TestFacetsRefusesUnknownFilter(t *testing.T) {
	h := NewHandler(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/events/facets?category=koncert", nil)
	w := httptest.NewRecorder()
	h.Facets(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("код %d, ожидали 400", w.Code)
	}
}

// /api/events/facets и /api/events/{id} стоят в одном узле маршрутизатора.
// Мы полагаемся на то, что chi матчит статический сегмент первым: если
// возьмёт верх параметр, фасеты ответят «event not found», плитки на странице
// будут пустыми, а по кодам ответа всё будет выглядеть исправным.
func TestStaticFacetsRouteWinsOverTheIDParam(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/api/events/facets", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("facets"))
	})
	r.Get("/api/events/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("by-id"))
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/events/facets", nil))
	if got := w.Body.String(); got != "facets" {
		t.Errorf("/api/events/facets ушёл в %q — фасеты недостижимы", got)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/events/ev_deadbeef", nil))
	if got := w.Body.String(); got != "by-id" {
		t.Errorf("карточка события ушла в %q", got)
	}
}

// handlerListCode — тело List без строк-комментариев. Комментарии выброшены
// потому, что тест ищет ПОРЯДОК операций, а имена проверок упоминаются и в
// объяснениях рядом с ними.
func handlerListCode(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, line := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	body := b.String()
	start := strings.Index(body, "func (h *Handler) List(")
	if start < 0 {
		t.Fatal("не нашёл List в handler.go")
	}
	body = body[start:]
	if end := strings.Index(body, "\nfunc "); end > 0 {
		body = body[:end]
	}
	return body
}

// Весь вход проверяется ДО того, как появится ключ кэша.
//
// `/api/*` торчит наружу, и ключ, посчитанный по непроверенному значению, —
// это ключевое пространство редиса, которым распоряжается кто угодно снаружи.
// Плюс нормализация: `?limit=` и `?limit=30` — одна страница, и ключ у них
// обязан быть один, а это верно ровно тогда, когда ключ считается ПОСЛЕ
// подстановки значения по умолчанию.
func TestValidationRunsBeforeTheCacheKey(t *testing.T) {
	list := handlerListCode(t)
	pos := func(needle string) int {
		i := strings.Index(list, needle)
		if i < 0 {
			t.Fatalf("в List нет %q — тест устарел вместе с кодом", needle)
		}
		return i
	}
	parse := pos("ParseFilter(")
	maxPage := pos("maxPageSize")
	window := pos("maxWindow")
	key := pos("CacheKey(")
	get := pos("GetList(")

	if parse > key {
		t.Error("ключ кэша считается до разбора фильтра — мусорный раздел доедет до редиса")
	}
	if maxPage > key || window > key {
		t.Error("потолки проверяются после ключа кэша — ключ строится по непроверенному limit/offset")
	}
	if key > get {
		t.Fatal("чтение кэша идёт раньше, чем считается ключ")
	}
}
