package events

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pythonjsgo/vshage-afisha/internal/mail"
)

// Уведомления о регистрациях через outbox (миграции 010 и 013).
//
// enqueue* пишет готовую доставку в очередь В ТОЙ ЖЕ транзакции, что и
// регистрацию: сохранились вместе или никак. StartNotifySender — фоновый
// отправщик: канал лежит или бэкенд перезапустился — строка переживёт и
// доедет со следующего тика. Ошибка отправки НИКОГДА не влияет на саму
// регистрацию, она только копит attempts/last_error в outbox.
//
// Каналов три и они равноправны:
//   tg    — организатору в чат (payload = готовый текст)
//   email — гостю письмо (payload = JSON письма, recipient = адрес)
//   push  — организатору и админам в приложение (recipient = profile_id)

const notifyTick = 10 * time.Second

const (
	channelTG    = "tg"
	channelEmail = "email"
	channelPush  = "push"
)

var mskZone = time.FixedZone("MSK", 3*60*60)

func enqueueNotify(ctx context.Context, tx pgx.Tx, text string) error {
	return enqueueChannel(ctx, tx, channelTG, "", text)
}

func enqueueChannel(ctx context.Context, tx pgx.Tx, channel, recipient, payload string) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO registration_notify_outbox (channel, recipient, payload) VALUES ($1, NULLIF($2,''), $3)`,
		channel, recipient, payload)
	return err
}

// mailJob — письмо в очереди. Сериализуем целиком, а не «id записи, соберём
// при отправке»: в момент записи все поля под рукой, а отправщику не нужны
// ни JOIN'ы, ни живое событие. Удалили событие — письмо всё равно доедет тем,
// кто уже записался, с тем текстом, который они ожидают.
type mailJob struct {
	To string `json:"to"`
	// EventStart — когда начинается событие. Письмо лежит в очереди, пока
	// канал недоступен; без этой отметки день, когда почту наконец настроят,
	// начался бы с рассылки «ты записан(а)» на прошедшие события.
	EventStart time.Time `json:"event_start,omitempty"`
	ToName     string    `json:"to_name,omitempty"`
	Subject    string    `json:"subject"`
	HTML       string    `json:"html"`
	Text       string    `json:"text"`
	ICSName    string    `json:"ics_name,omitempty"`
	ICS        []byte    `json:"ics,omitempty"`
}

// pushJob — уведомление в приложение. Строка в notifications пишется
// отправщиком, а не в транзакции записи: иначе гость видел бы уведомление
// организатора мгновенно, а APNs-баннер — с задержкой, и рассинхрон между
// списком и баннером пришлось бы объяснять.
type pushJob struct {
	ProfileID string `json:"profile_id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	EventID   string `json:"event_id"`
}

// PushSender — то, что умеет доставить уведомление в приложение. Интерфейс
// узкий, реализация в internal/appnotify; nil ⇒ канал выключен, и это
// сообщается в лог на старте, а не молча.
type PushSender interface {
	Deliver(ctx context.Context, profileID, title, body, eventID string) error
}

type notifier struct {
	pool     *pgxpool.Pool
	botToken string
	chatID   string
	mailer   mail.Sender
	pusher   PushSender
}

// StartNotifySender запускает вечный цикл доставки. Пустые креды канала —
// сервис работает как раньше, но кричит в лог на старте, а не молчит: немой
// канал уведомлений, выглядящий исправным, у нас уже был (алертманажер без
// токена).
func StartNotifySender(ctx context.Context, pool *pgxpool.Pool, botToken, chatID string, mailer mail.Sender, pusher PushSender) {
	n := &notifier{pool: pool, botToken: botToken, chatID: chatID, mailer: mailer, pusher: pusher}
	if botToken == "" || chatID == "" {
		log.Print("notify: TG_BOT_TOKEN/TG_CHAT_ID не заданы — телеграм-уведомления ОТКЛЮЧЕНЫ")
	}
	if mailer == nil {
		log.Print("notify: почта не настроена — письма о записи ОТКЛЮЧЕНЫ")
	}
	if pusher == nil {
		log.Print("notify: пуши не настроены — уведомления в приложение ОТКЛЮЧЕНЫ")
	}
	log.Print("notify: отправщик запущен")
	go func() {
		t := time.NewTicker(notifyTick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				n.drain(ctx)
			}
		}
	}()
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

func (n *notifier) drain(ctx context.Context) {
	// SKIP LOCKED — на случай второй реплики; одиночному инстансу не мешает.
	rows, err := n.pool.Query(ctx, `
		SELECT id, COALESCE(channel,'tg'), COALESCE(recipient,''), payload
		FROM registration_notify_outbox
		WHERE sent_at IS NULL
		  AND (next_attempt_at IS NULL OR next_attempt_at <= NOW())
		ORDER BY id
		LIMIT 20
		FOR UPDATE SKIP LOCKED`)
	if err != nil {
		log.Printf("notify: чтение outbox: %v", err)
		return
	}
	type item struct {
		id        int64
		channel   string
		recipient string
		payload   string
	}
	var batch []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.channel, &it.recipient, &it.payload); err != nil {
			rows.Close()
			log.Printf("notify: scan: %v", err)
			return
		}
		batch = append(batch, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		// pgx прячет отказ прав и обрыв соединения именно здесь: без этой
		// проверки короткий батч выглядит как «очередь пуста».
		log.Printf("notify: чтение outbox: %v", err)
		return
	}

	// Отказ одного канала не должен задерживать другие: телега лежит — письма
	// всё равно уходят. Поэтому «подождать до следующего тика» действует на
	// свой канал, а не на всю пачку.
	stalled := map[string]bool{}
	for _, it := range batch {
		if stalled[it.channel] {
			continue
		}
		err := n.deliver(ctx, it.channel, it.recipient, it.payload)
		if err != nil {
			// Адрес получателя в тексте ошибки релея — персональные данные,
			// а last_error хранится вечно и читается глазами. Режем.
			msg := scrubAddress(err.Error(), it.recipient)
			log.Printf("notify: отправка id=%d канал=%s: %s", it.id, it.channel, msg)
			// Пауза растёт с попытками и упирается в пять минут: канал,
			// который лежит час, не должен ни собирать восемь тысяч
			// обращений, ни потерять строку.
			_, _ = n.pool.Exec(ctx, `
				UPDATE registration_notify_outbox
				SET attempts = attempts + 1, last_error = $2,
				    next_attempt_at = NOW() + LEAST(attempts + 1, 30) * interval '10 seconds'
				WHERE id = $1`, it.id, msg)
			stalled[it.channel] = true
			continue
		}
		if _, err := n.pool.Exec(ctx, `
			UPDATE registration_notify_outbox
			SET sent_at = NOW(), attempts = attempts + 1, last_error = NULL,
			    next_attempt_at = NULL
			WHERE id = $1 AND sent_at IS NULL`, it.id); err != nil {
			log.Printf("notify: пометка id=%d: %v", it.id, err)
			return
		}
	}
}

func (n *notifier) deliver(ctx context.Context, channel, recipient, payload string) error {
	switch channel {
	case channelEmail:
		if n.mailer == nil {
			return fmt.Errorf("почта не настроена")
		}
		var j mailJob
		if err := json.Unmarshal([]byte(payload), &j); err != nil {
			return fmt.Errorf("разбор письма: %w", err)
		}
		to := j.To
		if to == "" {
			to = recipient
		}
		if !j.EventStart.IsZero() && time.Now().After(j.EventStart) {
			// Событие прошло — письмо больше не про что. Возвращаем nil,
			// строка помечается отправленной и не копится вечно.
			log.Printf("notify: письмо на %s пропущено — событие уже прошло", to)
			return nil
		}
		l := mail.Letter{To: to, ToName: j.ToName, Subject: j.Subject, HTML: j.HTML, Text: j.Text}
		if len(j.ICS) > 0 {
			name := j.ICSName
			if name == "" {
				name = "event.ics"
			}
			l.Attachments = []mail.Attachment{{
				Filename: name,
				MIMEType: `text/calendar; charset=utf-8; method=PUBLISH`,
				Content:  j.ICS,
			}}
		}
		return n.mailer.Send(ctx, l)

	case channelPush:
		if n.pusher == nil {
			return fmt.Errorf("пуши не настроены")
		}
		var j pushJob
		if err := json.Unmarshal([]byte(payload), &j); err != nil {
			return fmt.Errorf("разбор пуша: %w", err)
		}
		id := j.ProfileID
		if id == "" {
			id = recipient
		}
		return n.pusher.Deliver(ctx, id, j.Title, j.Body, j.EventID)

	default: // channelTG
		if n.botToken == "" || n.chatID == "" {
			return fmt.Errorf("телеграм не настроен")
		}
		return sendTelegram(ctx, n.botToken, n.chatID, payload)
	}
}

// scrubAddress убирает адрес получателя из текста, который мы собираемся
// записать в базу и в лог.
func scrubAddress(msg, recipient string) string {
	if recipient == "" {
		return msg
	}
	return strings.ReplaceAll(msg, recipient, "<получатель>")
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
