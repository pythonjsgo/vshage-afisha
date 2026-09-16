package events

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pythonjsgo/vshage-afisha/internal/regform"
)

type groupRoundTrip func(*http.Request) (*http.Response, error)

func (f groupRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGroupMessageOnlySharesApprovedFields(t *testing.T) {
	clean := regform.Clean{FullName: "Тестовый участник", Email: "test@example.invalid", Phone: "+79990000000", TGDisplay: "@test_only",
		Answers: map[string]string{"birth_date": "01.01.2000", "internal_id": "secret-id", "about": "Мой проект"}}
	got := groupRegistrationMessage("NetForKing", clean, time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC))
	for _, want := range []string{"NetForKing", clean.FullName, clean.Email, clean.Phone, clean.TGDisplay, "15.09.2026 12:00:00 МСК", "заявка через ВШаге", "Мой проект"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	for _, forbidden := range []string{"01.01.2000", "birth_date", "secret-id", "internal_id", "ID записи"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("shared forbidden field %q", forbidden)
		}
	}
}

func TestGroupDeliverySQL(t *testing.T) {
	pool := boardPool(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	const owner = "00000000-0000-0000-0000-000000000001"
	const event = "00000000-0000-0000-0000-000000000002"
	exec(`DROP TABLE IF EXISTS organizer_settings, registration_notify_outbox;
	 CREATE TABLE organizer_settings(profile_id uuid PRIMARY KEY, telegram_chat_id text NOT NULL DEFAULT '');
	 CREATE TABLE registration_notify_outbox(id bigserial PRIMARY KEY, channel text, recipient text, payload text, sent_at timestamptz, attempts int NOT NULL DEFAULT 0, last_error text, next_attempt_at timestamptz);
	 INSERT INTO events(id,title,start_time,status,organizer_id) VALUES ('` + event + `','NetForKing',now(),'published','` + owner + `');
	 INSERT INTO organizer_settings VALUES ('` + owner + `','-12345')`)
	t.Cleanup(func() { exec(`DROP TABLE organizer_settings, registration_notify_outbox`) })
	ev := mailEvent{ID: event, Title: "NetForKing"}
	clean := regform.Clean{Name: "Тест", Contact: "@test_only"}
	enqueue := func(commit bool) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		enqueueGroupTelegram(ctx, tx, ev, clean)
		if err := EnqueueTelegram(ctx, tx, "Owner unchanged"); err != nil {
			t.Fatal(err)
		}
		if commit {
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	count := func(want int) {
		t.Helper()
		var got int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM registration_notify_outbox`).Scan(&got); err != nil || got != want {
			t.Fatalf("queue count=%d want=%d err=%v", got, want, err)
		}
	}
	enqueue(false)
	count(0) // Signup rollback rolls back both destinations.
	enqueue(true)
	count(2)
	oldClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = oldClient })
	var recipients []string
	failGroup := true
	http.DefaultClient = &http.Client{Transport: groupRoundTrip(func(r *http.Request) (*http.Response, error) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		recipients = append(recipients, body["chat_id"])
		status, response := 200, `{"ok":true}`
		if body["chat_id"] == "-12345" && failGroup {
			status = 400
			response = `{"ok":false,"description":"chat not found"}`
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(response))}, nil
	})}
	n := &notifier{pool: pool, botToken: "test-token", chatID: "owner"}
	n.drain(ctx)
	if strings.Join(recipients, ",") != "-12345,owner" {
		t.Fatalf("group failure blocked owner: %v", recipients)
	}
	var attempts int
	var lastError string
	if err := pool.QueryRow(ctx, `SELECT attempts,last_error FROM registration_notify_outbox WHERE channel='tg_group' AND sent_at IS NULL`).Scan(&attempts, &lastError); err != nil || attempts != 1 || lastError == "" {
		t.Fatalf("failure not recorded: %d %q %v", attempts, lastError, err)
	}
	failGroup = false
	exec(`UPDATE registration_notify_outbox SET next_attempt_at=NULL`)
	n.drain(ctx)
	var pending int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM registration_notify_outbox WHERE sent_at IS NULL`).Scan(&pending)
	if pending != 0 {
		t.Fatalf("retry did not deliver: %d", pending)
	}
	// A positive chat ID routes the same durable job to a personal account.
	exec(`UPDATE organizer_settings SET telegram_chat_id='882079062'`)
	enqueue(true)
	recipients = nil
	n.drain(ctx)
	if strings.Join(recipients, ",") != "882079062,owner" {
		t.Fatalf("private destination not delivered: %v", recipients)
	}
	// Revocation suppresses queued group delivery without affecting the owner.
	enqueue(true)
	exec(`UPDATE organizer_settings SET telegram_chat_id=''`)
	recipients = nil
	n.drain(ctx)
	if strings.Join(recipients, ",") != "owner" {
		t.Fatalf("revoked destination received data: %v", recipients)
	}
	exec(`TRUNCATE registration_notify_outbox`)
	enqueue(true)
	count(1)
	// Missing migration/schema is isolated by a savepoint, owner enqueue survives.
	exec(`DROP TABLE organizer_settings`)
	enqueue(true)
	count(2)
	exec(`CREATE TABLE organizer_settings(profile_id uuid PRIMARY KEY)`)
	enqueue(true)
	count(3)
	// A chat configured for a different organizer does not receive this event.
	exec(`ALTER TABLE organizer_settings ADD COLUMN telegram_chat_id text; INSERT INTO organizer_settings VALUES ('00000000-0000-0000-0000-000000000099','-999')`)
	enqueue(true)
	count(4)
}
