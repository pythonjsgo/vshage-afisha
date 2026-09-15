package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pythonjsgo/vshage-afisha/internal/regform"
)

// to_jsonb allows the sender to roll out before the organizer migration.
// Missing settings or an event deleted since enqueue means no destination.
const groupChatQuery = `SELECT COALESCE((
	SELECT to_jsonb(s)->>'telegram_chat_id'
	FROM events e JOIN organizer_settings s ON s.profile_id = e.organizer_id
	WHERE e.id = $1), '')`

type groupTelegramJob struct {
	EventID string `json:"event_id"`
	Text    string `json:"text"`
}

func groupRegistrationMessage(title string, clean regform.Clean, createdAt time.Time) string {
	text := fmt.Sprintf("✍️ Новая заявка\nСобытие: %s\nИмя: %s\nКонтакт: %s\nДата заявки: %s МСК\nзаявка через ВШаге",
		title, clean.DisplayName(), clean.ContactLine(), createdAt.In(mskZone).Format("02.01.2006 15:04:05"))
	// Only the optional intro is shared. Never copy arbitrary form answers:
	// other organizers may collect dates of birth or internal metadata.
	if about := clean.Answers["about"]; about != "" {
		intro := []rune(about)
		if len(intro) > 300 {
			intro = intro[:300]
		}
		text += "\nО себе, пара строк: " + string(intro)
	}
	return text
}

// A savepoint keeps schema/grant/configuration failures from aborting signup.
// Successful enqueue still commits atomically with the registration.
func enqueueGroupTelegram(ctx context.Context, tx pgx.Tx, ev mailEvent, clean regform.Clean) {
	sub, err := tx.Begin(ctx)
	if err != nil {
		log.Printf("notify: group savepoint: %v", err)
		return
	}
	defer sub.Rollback(ctx)
	var chatID string
	if err := sub.QueryRow(ctx, groupChatQuery, ev.ID).Scan(&chatID); err != nil {
		log.Printf("notify: group settings: %v", err)
		return
	}
	if chatID == "" {
		return
	}
	var createdAt time.Time
	if err := sub.QueryRow(ctx, `SELECT transaction_timestamp()`).Scan(&createdAt); err != nil {
		log.Printf("notify: group timestamp: %v", err)
		return
	}
	payload, err := json.Marshal(groupTelegramJob{EventID: ev.ID, Text: groupRegistrationMessage(ev.Title, clean, createdAt)})
	if err == nil {
		err = enqueueChannel(ctx, sub, channelTGGroup, chatID, string(payload))
	}
	if err == nil {
		err = sub.Commit(ctx)
	}
	if err != nil {
		log.Printf("notify: enqueue group: %v", err)
	}
}
