package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Уведомления о регистрациях в Telegram через outbox (миграция 010).
//
// enqueueNotify пишет готовый текст в очередь В ТОЙ ЖЕ транзакции, что и
// регистрацию: сохранились вместе или никак. StartNotifySender — фоновый
// отправщик: телега лежит или бэкенд перезапустился — строка переживёт и
// доедет со следующего тика. Ошибка отправки НИКОГДА не влияет на саму
// регистрацию, она только копит attempts/last_error в outbox.

const notifyTick = 10 * time.Second

var mskZone = time.FixedZone("MSK", 3*60*60)

func enqueueNotify(ctx context.Context, tx pgx.Tx, text string) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO registration_notify_outbox (payload) VALUES ($1)`, text)
	return err
}

// registrationMessage собирает текст со ВСЕМИ полями записи — «чтоб ничего
// не проебать» (формулировка заказчика): событие, имя и контакт из формы,
// статус, счётчик мест, id для разбирательств.
func registrationMessage(eventTitle, name, contact, status, regID string,
	taken int, capacity *int, startTime time.Time, repeat bool, extra string) string {

	seats := fmt.Sprintf("%d", taken)
	if capacity != nil && *capacity > 0 {
		seats = fmt.Sprintf("%d из %d", taken, *capacity)
	}
	kind := "Новая запись"
	if repeat {
		kind = "Повторная запись (после отмены)"
	}
	return fmt.Sprintf(
		"✍️ %s — %s\n"+
			"Имя: %s\n"+
			"Контакт: %s\n"+
			"%s"+
			"Статус: %s\n"+
			"Занято: %s\n"+
			"Событие: %s МСК\n"+
			"Записан(а): %s МСК\n"+
			"ID записи: %s",
		kind, eventTitle,
		name,
		contact,
		extra,
		status,
		seats,
		startTime.In(mskZone).Format("02.01 15:04"),
		time.Now().In(mskZone).Format("02.01 15:04:05"),
		regID,
	)
}

// StartNotifySender запускает вечный цикл доставки. Пустые креды — сервис
// работает как раньше, но кричит в лог на старте, а не молчит: немой канал
// уведомлений, выглядящий исправным, у нас уже был (алертманажер без токена).
func StartNotifySender(ctx context.Context, pool *pgxpool.Pool, botToken, chatID string) {
	if botToken == "" || chatID == "" {
		log.Print("tgnotify: TG_BOT_TOKEN/TG_CHAT_ID не заданы — уведомления о регистрациях ОТКЛЮЧЕНЫ")
		return
	}
	log.Print("tgnotify: отправщик запущен")
	go func() {
		t := time.NewTicker(notifyTick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				drainOutbox(ctx, pool, botToken, chatID)
			}
		}
	}()
}

func drainOutbox(ctx context.Context, pool *pgxpool.Pool, botToken, chatID string) {
	// SKIP LOCKED — на случай второй реплики; одиночному инстансу не мешает.
	rows, err := pool.Query(ctx, `
		SELECT id, payload FROM registration_notify_outbox
		WHERE sent_at IS NULL
		ORDER BY id
		LIMIT 20
		FOR UPDATE SKIP LOCKED`)
	if err != nil {
		log.Printf("tgnotify: чтение outbox: %v", err)
		return
	}
	type item struct {
		id      int64
		payload string
	}
	var batch []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.payload); err != nil {
			rows.Close()
			log.Printf("tgnotify: scan: %v", err)
			return
		}
		batch = append(batch, it)
	}
	rows.Close()

	for _, it := range batch {
		if err := sendTelegram(ctx, botToken, chatID, it.payload); err != nil {
			log.Printf("tgnotify: отправка id=%d: %v", it.id, err)
			_, _ = pool.Exec(ctx, `
				UPDATE registration_notify_outbox
				SET attempts = attempts + 1, last_error = $2
				WHERE id = $1`, it.id, err.Error())
			// Телега недоступна — остаток пачки подождёт следующего тика,
			// долбить её в цикле бессмысленно.
			return
		}
		if _, err := pool.Exec(ctx, `
			UPDATE registration_notify_outbox
			SET sent_at = NOW(), attempts = attempts + 1, last_error = NULL
			WHERE id = $1`, it.id); err != nil {
			log.Printf("tgnotify: пометка id=%d: %v", it.id, err)
			return
		}
	}
}

func sendTelegram(ctx context.Context, botToken, chatID, text string) error {
	body, _ := json.Marshal(map[string]string{"chat_id": chatID, "text": text})
	sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(sctx, http.MethodPost,
		"https://api.telegram.org/bot"+botToken+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("telegram %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
