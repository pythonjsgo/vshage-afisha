package join

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Прогон ручки POST /api/join против НАСТОЯЩЕГО постгреса.
//
// Здесь проверяется ровно то, что нельзя проверить чтением: транзакция
// «анкета + уведомление», дедуп по юзернейму за сутки и то, что honeypot
// НИЧЕГО не пишет, отвечая при этом успехом.
//
// Таблица создаётся из ФАЙЛА МИГРАЦИИ 019, а не копией DDL в тесте: копия
// разъезжается с боевой схемой молча, и тест начинает охранять несуществующее.
// Очередь уведомлений, наоборот, заводится минимальной — она принадлежит
// миграциям 010/013/014, тянуть их сюда значит тянуть половину схемы афиши;
// код пишет в неё три колонки, их и проверяем.
//
//	AFISHA_TEST_DATABASE_URL=postgres://…/afisha_z1 go test ./internal/join/ -run SQL -v
const joinSchema = "afisha_join_test"

func joinPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	// Имя переменной в репозитории не одно: часть файлов читает
	// AFISHA_TEST_DATABASE_URL, часть — TEST_DATABASE_URL. Принимаем обе,
	// иначе тест «зелёный» ровно потому, что не выполнялся.
	url := os.Getenv("AFISHA_TEST_DATABASE_URL")
	if url == "" {
		url = os.Getenv("TEST_DATABASE_URL")
	}
	if url == "" {
		t.Skip("AFISHA_TEST_DATABASE_URL / TEST_DATABASE_URL не заданы — прогон против постгреса пропущен")
	}

	ctx := context.Background()
	boot, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("подключение: %v", err)
	}
	if _, err := boot.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+joinSchema); err != nil {
		boot.Close()
		t.Fatalf("создание схемы: %v", err)
	}
	boot.Close()

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("разбор строки подключения: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = joinSchema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("подключение к схеме: %v", err)
	}
	t.Cleanup(pool.Close)

	ddl, err := os.ReadFile("../../pkg/db/migrations/019_join_requests.sql")
	if err != nil {
		t.Fatalf("миграция 019 не прочиталась: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		DROP TABLE IF EXISTS join_requests, registration_notify_outbox;
		CREATE TABLE registration_notify_outbox (
			id         BIGSERIAL PRIMARY KEY,
			channel    TEXT,
			recipient  TEXT,
			payload    TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			sent_at    TIMESTAMPTZ
		);`+string(ddl)); err != nil {
		t.Fatalf("схема теста: %v", err)
	}
	return pool
}

func joinRouter(pool *pgxpool.Pool) http.Handler {
	h := NewHandler(NewRepository(pool, "test-salt"))
	r := chi.NewRouter()
	r.Route("/api/join", func(r chi.Router) {
		// Лимитер здесь намеренно НЕ подключён: он проверяется отдельно
		// (pkg/middleware), а внутри теста дал бы 429 на пятой анкете и
		// покрасил бы тест, который про другое.
		h.Routes(r, func(next http.Handler) http.Handler { return next })
	})
	return r
}

func postJoin(t *testing.T, h http.Handler, body map[string]any, ip string) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/join", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "test-agent/1.0")
	if ip != "" {
		req.Header.Set("X-Forwarded-For", ip)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func okBody() map[string]any {
	return map[string]any{
		"name":         "Иван Петров",
		"university":   "hse",
		"course":       "3",
		"about":        "Пишу бота для расписания",
		"telegram":     "@ivanov_hse",
		"consent":      true,
		"utm_source":   "yandex",
		"utm_medium":   "cpc",
		"utm_campaign": "join_msk_students",
		"yclid":        "1234567890",
	}
}

func TestJoinSubmitSQL(t *testing.T) {
	// Решение о пропуске принимается ЗДЕСЬ, в родителе, а не внутри t.Run:
	// t.Skip в подтесте печатает у родителя --- PASS, и набор из четырёх
	// пропущенных проверок выглядит как четыре пройденные.
	pool := joinPool(t)
	h := joinRouter(pool)
	ctx := context.Background()

	countRows := func() int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM join_requests`).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}

	t.Run("валидная анкета пишется и уведомляет", func(t *testing.T) {
		w := postJoin(t, h, okBody(), "203.0.113.10")
		if w.Code != http.StatusOK {
			t.Fatalf("код %d, тело %s", w.Code, w.Body.String())
		}

		var got struct {
			Name, University, Course, About, Telegram, Status string
			UTMCampaign, YClid, UserAgent, IPHash             string
			ConsentAt                                         *string
		}
		err := pool.QueryRow(ctx, `
			SELECT name, university, course, about, telegram, status,
			       COALESCE(utm_campaign,''), COALESCE(yclid,''),
			       COALESCE(user_agent,''), COALESCE(ip_hash,''),
			       consent_at::text
			FROM join_requests ORDER BY id DESC LIMIT 1`).
			Scan(&got.Name, &got.University, &got.Course, &got.About, &got.Telegram,
				&got.Status, &got.UTMCampaign, &got.YClid, &got.UserAgent,
				&got.IPHash, &got.ConsentAt)
		if err != nil {
			t.Fatalf("чтение анкеты: %v", err)
		}
		if got.Telegram != "ivanov_hse" {
			t.Errorf("telegram = %q, ждали без '@' и в нижнем регистре", got.Telegram)
		}
		if got.Status != "new" {
			t.Errorf("status = %q, ждали new", got.Status)
		}
		if got.UTMCampaign != "join_msk_students" || got.YClid != "1234567890" {
			t.Errorf("метки кампании не доехали: campaign=%q yclid=%q", got.UTMCampaign, got.YClid)
		}
		if got.ConsentAt == nil || *got.ConsentAt == "" {
			t.Error("consent_at пуст — согласие обязано быть с датой")
		}
		// Адрес хранится хешем, а не как есть: он нужен для «один ли это
		// отправитель», а не для опознания.
		if got.IPHash == "" || strings.Contains(got.IPHash, "203.0.113") {
			t.Errorf("ip_hash = %q — ждали хеш, а не адрес", got.IPHash)
		}

		var payload string
		if err := pool.QueryRow(ctx, `
			SELECT payload FROM registration_notify_outbox ORDER BY id DESC LIMIT 1`).
			Scan(&payload); err != nil {
			t.Fatalf("уведомление не поставлено в очередь: %v", err)
		}
		for _, want := range []string{"Новая анкета", "Иван Петров", "ВШЭ", "@ivanov_hse"} {
			if !strings.Contains(payload, want) {
				t.Errorf("в уведомлении нет %q:\n%s", want, payload)
			}
		}
	})

	t.Run("повтор того же телеграма за сутки", func(t *testing.T) {
		before := countRows()
		body := okBody()
		body["telegram"] = "IVANOV_HSE" // другой регистр — тот же человек
		w := postJoin(t, h, body, "203.0.113.11")
		if w.Code != http.StatusConflict {
			t.Fatalf("код %d, ждали 409; тело %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "already_submitted") {
			t.Errorf("ждали код already_submitted, тело %s", w.Body.String())
		}
		if n := countRows(); n != before {
			t.Errorf("строк стало %d вместо %d — повтор записался", n, before)
		}
	})

	t.Run("невалидный телеграм", func(t *testing.T) {
		before := countRows()
		body := okBody()
		body["telegram"] = "иванов"
		w := postJoin(t, h, body, "203.0.113.12")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("код %d, ждали 400; тело %s", w.Code, w.Body.String())
		}
		var e struct {
			Code   string            `json:"code"`
			Fields map[string]string `json:"fields"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &e)
		if e.Fields["telegram"] == "" {
			t.Errorf("ждали ошибку по полю telegram, тело %s", w.Body.String())
		}
		if n := countRows(); n != before {
			t.Errorf("невалидная анкета записалась")
		}
	})

	t.Run("без согласия", func(t *testing.T) {
		before := countRows()
		body := okBody()
		body["telegram"] = "@petrov_msu"
		body["consent"] = false
		w := postJoin(t, h, body, "203.0.113.13")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("код %d, ждали 400; тело %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "consent") {
			t.Errorf("ждали ошибку по consent, тело %s", w.Body.String())
		}
		if n := countRows(); n != before {
			t.Errorf("анкета без согласия записалась")
		}
	})

	t.Run("honeypot отвечает успехом и ничего не пишет", func(t *testing.T) {
		before := countRows()
		body := okBody()
		body["telegram"] = "@spam_bot_777"
		body["hp_note"] = "http://buy-cheap.example"
		w := postJoin(t, h, body, "203.0.113.14")
		// Ответ обязан быть неотличим от успешного: скажи боту «отказ» — и
		// автор поправит скрипт, а страница получит вторую волну без ловушки.
		if w.Code != http.StatusOK {
			t.Fatalf("код %d, ждали 200; тело %s", w.Code, w.Body.String())
		}
		if n := countRows(); n != before {
			t.Errorf("honeypot записал строку: было %d, стало %d", before, n)
		}
	})

	t.Run("другой человек проходит", func(t *testing.T) {
		before := countRows()
		body := okBody()
		body["telegram"] = "@sidorov_mipt"
		body["university"] = "mipt"
		if w := postJoin(t, h, body, "203.0.113.15"); w.Code != http.StatusOK {
			t.Fatalf("код %d, тело %s", w.Code, w.Body.String())
		}
		if n := countRows(); n != before+1 {
			t.Fatalf("строк %d, ждали %d", n, before+1)
		}
	})
}

// TestJoinModerationSQL — модерация: список и смена статуса. Проверяется в том
// числе то, что несуществующий id даёт 404, а не тихий «успех»: опечатка в
// номере иначе читается как разобранная анкета.
func TestJoinModerationSQL(t *testing.T) {
	pool := joinPool(t)
	repo := NewRepository(pool, "test-salt")
	ctx := context.Background()

	clean, errs := Validate(Submission{
		Name: "Мария Смирнова", University: "msu", Course: "graduate",
		About: "Организую лекции", Telegram: "@smirnova_msu", Consent: true,
	})
	if errs != nil {
		t.Fatalf("фикстура невалидна: %v", errs)
	}
	id, err := repo.Create(ctx, clean)
	if err != nil {
		t.Fatalf("создание: %v", err)
	}

	rows, err := repo.List(ctx, "new", 50)
	if err != nil {
		t.Fatalf("список: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("список пуст — прибор сломан, а не данных нет")
	}

	ok, err := repo.SetStatus(ctx, id, "approved")
	if err != nil || !ok {
		t.Fatalf("смена статуса: ok=%v err=%v", ok, err)
	}
	var status string
	var decidedAt *string
	if err := pool.QueryRow(ctx,
		`SELECT status, decided_at::text FROM join_requests WHERE id=$1`, id).
		Scan(&status, &decidedAt); err != nil {
		t.Fatalf("чтение статуса: %v", err)
	}
	if status != "approved" {
		t.Fatalf("status = %q", status)
	}
	// Без даты решения срок хранения, обещанный политикой («90 дней с даты
	// решения»), невозможно ни выполнить, ни проверить.
	if decidedAt == nil || *decidedAt == "" {
		t.Fatal("decided_at пуст после смены статуса")
	}

	if ok, err := repo.SetStatus(ctx, id+100000, "rejected"); err != nil || ok {
		t.Fatalf("несуществующий id: ok=%v err=%v — ждали ok=false без ошибки", ok, err)
	}
}
