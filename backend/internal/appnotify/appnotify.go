// Package appnotify доставляет уведомление о новой записи в приложение Вшаге:
// строкой в общей таблице notifications (её читает лента уведомлений core-api)
// и баннером через APNs.
//
// Почему афиша шлёт пуш сама, а не зовёт core-api: своего внутреннего
// эндпоинта у core-api нет, а заводить его — это выкатка core-api на прод
// через ручной гейт ради уведомления. База у сервисов общая, ключ APNs
// монтируется тем же файлом. Клиент здесь намеренно маленький: одна
// alert-нотификация, без VoIP, без фоновых пушей.
package appnotify

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	apnsSandbox    = "https://api.sandbox.push.apple.com"
	apnsProduction = "https://api.push.apple.com"
	// Apple держит токен час; берём запас, чтобы не отправить протухший.
	jwtLifetime = 45 * time.Minute
)

// Sender пишет уведомление в базу и, если получится, шлёт баннер.
type Sender struct {
	pool *pgxpool.Pool
	apns *apnsClient
}

// New собирает отправителя. APNs может быть не настроен — тогда уведомление
// всё равно появляется в списке в приложении, просто без баннера. Это
// сообщается в лог: «пуш не ушёл» и «пуш не настроен» должны различаться.
func New(pool *pgxpool.Pool) *Sender {
	return &Sender{pool: pool, apns: newAPNs()}
}

// Deliver реализует events.PushSender.
func (s *Sender) Deliver(ctx context.Context, profileID, title, body, eventID string) error {
	if profileID == "" {
		return errors.New("appnotify: пустой profile_id")
	}
	data, _ := json.Marshal(map[string]string{"type": "event", "event_id": eventID})

	// Строка в ленте уведомлений — главное: она переживает выключенный APNs,
	// отозванный токен и человека, запретившего уведомления системе.
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO notifications (profile_id, type, title, body, data)
		VALUES ($1, 'event', $2, $3, $4)`, profileID, title, body, data); err != nil {
		return fmt.Errorf("appnotify: запись уведомления: %w", err)
	}

	if s.apns == nil {
		return nil
	}
	var token string
	if err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(push_token,'') FROM profiles WHERE id = $1`, profileID).Scan(&token); err != nil {
		log.Printf("appnotify: токен %s: %v", profileID, err)
		return nil
	}
	if token == "" {
		return nil // человек не заходил с телефона — не отказ
	}
	if err := s.apns.send(ctx, token, title, body, eventID); err != nil {
		// Уведомление в базе уже есть, повторять доставку строки нельзя —
		// иначе ретрай очереди наплодит дублей в ленте. Баннер теряем,
		// но громко.
		log.Printf("appnotify: APNs для %s: %v", profileID, err)
	}
	return nil
}

type apnsClient struct {
	teamID   string
	keyID    string
	bundleID string
	key      *ecdsa.PrivateKey
	prod     bool
	http     *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

func newAPNs() *apnsClient {
	team, keyID := os.Getenv("APNS_TEAM_ID"), os.Getenv("APNS_KEY_ID")
	path, bundle := os.Getenv("APNS_KEY_PATH"), os.Getenv("APNS_BUNDLE_ID")
	if team == "" || keyID == "" || path == "" || bundle == "" {
		log.Print("appnotify: APNs не настроен — баннеры о записях выключены, список уведомлений работает")
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Printf("appnotify: ключ APNs %s не читается: %v", path, err)
		return nil
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		log.Print("appnotify: ключ APNs не в формате PEM")
		return nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		log.Printf("appnotify: разбор ключа APNs: %v", err)
		return nil
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		log.Print("appnotify: ключ APNs не ECDSA")
		return nil
	}
	prod := os.Getenv("APNS_PRODUCTION") == "true"
	env := "SANDBOX"
	if prod {
		env = "PRODUCTION"
	}
	log.Printf("appnotify: APNs готов (%s, bundle %s)", env, bundle)
	return &apnsClient{
		teamID: team, keyID: keyID, bundleID: bundle, key: key, prod: prod,
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *apnsClient) baseURL() string {
	if c.prod {
		return apnsProduction
	}
	return apnsSandbox
}

func (c *apnsClient) jwt() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.expires) {
		return c.token, nil
	}
	t := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": c.teamID,
		"iat": time.Now().Unix(),
	})
	t.Header["kid"] = c.keyID
	signed, err := t.SignedString(c.key)
	if err != nil {
		return "", err
	}
	c.token, c.expires = signed, time.Now().Add(jwtLifetime)
	return signed, nil
}

func (c *apnsClient) send(ctx context.Context, deviceToken, title, body, eventID string) error {
	payload, _ := json.Marshal(map[string]any{
		"aps": map[string]any{
			"alert": map[string]string{"title": title, "body": body},
			"sound": "default",
			// Одна нить на событие: пять записей подряд собираются в стопку,
			// а не в пять отдельных баннеров.
			"thread-id": "event-" + eventID,
		},
		"type":     "event",
		"event_id": eventID,
	})

	auth, err := c.jwt()
	if err != nil {
		return fmt.Errorf("подпись JWT: %w", err)
	}
	sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(sctx, http.MethodPost,
		c.baseURL()+"/3/device/"+deviceToken, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("authorization", "bearer "+auth)
	req.Header.Set("apns-topic", c.bundleID)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("apns-priority", "10")
	req.Header.Set("content-type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	var b bytes.Buffer
	_, _ = b.ReadFrom(resp.Body)
	return fmt.Errorf("apns %d: %s", resp.StatusCode, b.String())
}
