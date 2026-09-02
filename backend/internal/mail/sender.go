// Package mail — исходящая почта афиши: письмо после записи на событие и
// напоминание перед ним.
//
// Своя реализация, а не вызов кабинета, по трём причинам: данные события и
// записи живут здесь, очередь доставки (registration_notify_outbox) тоже
// здесь, а межсервисный вызов добавил бы отказ и аутентификацию ради
// пересылки текста, который мы уже собрали.
//
// Клиент — stdlib net/smtp, без вендорских SDK, как в core-api
// (internal/email/sender.go). Sender — интерфейс, чтобы тест и стенд
// подставляли заглушку, а не поднимали SMTP.
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"mime"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// Attachment — файл во вложении. Сегодня это карточка календаря (.ics).
type Attachment struct {
	Filename string
	MIMEType string // например "text/calendar; charset=utf-8; method=PUBLISH"
	Content  []byte
}

// Letter — готовое письмо. HTML и Text отправляются вместе (multipart/alternative):
// почтовик показывает то, что умеет, и письмо остаётся читаемым в терминале.
type Letter struct {
	To          string
	ToName      string
	Subject     string
	HTML        string
	Text        string
	Attachments []Attachment
}

// Sender отправляет одно письмо. Узкий интерфейс намеренно: подменить
// транспорт можно, не трогая места вызова.
//
// ctx обязателен: отправщик очереди — одна горутина, и зависшее соединение
// с релеем заморозило бы вместе с письмами телеграм и пуши. Молча и навсегда.
type Sender interface {
	Send(ctx context.Context, l Letter) error
}

// SMTPSender — рабочая реализация.
type SMTPSender struct {
	Host     string // "mail.vshage.app:465" или "mailpit:1025"
	User     string
	Pass     string
	FromAddr string
	FromName string
}

// NewSenderFromEnv собирает отправителя из окружения. Возвращает nil, если
// SMTP_HOST не задан — сервис в этом случае работает как раньше, но кричит
// в лог на старте: немой канал, выглядящий исправным, у нас уже был.
//
// Пароль читается либо из SMTP_PASSWORD_FILE (так он смонтирован на проде),
// либо из SMTP_PASSWORD. Файл имеет приоритет.
func NewSenderFromEnv() Sender {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		log.Print("mail: SMTP_HOST не задан — письма о записи ОТКЛЮЧЕНЫ")
		return nil
	}
	port := os.Getenv("SMTP_PORT")
	if port == "" {
		port = "587"
	}
	pass := os.Getenv("SMTP_PASSWORD")
	if f := os.Getenv("SMTP_PASSWORD_FILE"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			log.Printf("mail: SMTP_PASSWORD_FILE %s не читается: %v — письма ОТКЛЮЧЕНЫ", f, err)
			return nil
		}
		pass = strings.TrimSpace(string(b))
	}
	from := os.Getenv("SMTP_FROM_ADDR")
	if from == "" {
		from = "noreply@vshage.app"
	}
	fromName := os.Getenv("SMTP_FROM_NAME")
	if fromName == "" {
		fromName = "Вшаге"
	}
	s := &SMTPSender{
		Host:     net.JoinHostPort(host, port),
		User:     os.Getenv("SMTP_USER"),
		Pass:     pass,
		FromAddr: from,
		FromName: fromName,
	}
	log.Printf("mail: отправитель готов, релей %s, from %s", s.Host, s.FromAddr)
	return s
}

// sendTimeout — потолок на всю сессию с релеем. Больше него отправка не
// висит ни при каких обстоятельствах.
const sendTimeout = 30 * time.Second

func (s *SMTPSender) Send(ctx context.Context, l Letter) error {
	if l.To == "" {
		return errors.New("mail: пустой адрес получателя")
	}
	msg, err := s.build(l)
	if err != nil {
		return err
	}

	sctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.deliver(l.To, msg) }()
	select {
	case <-sctx.Done():
		// Соединение доживёт в своей горутине и закроется по дедлайну сокета;
		// очередь при этом уже свободна.
		return fmt.Errorf("mail: релей не ответил за %s: %w", sendTimeout, sctx.Err())
	case err := <-done:
		return err
	}
}

// deliver разговаривает с релеем руками, а не через smtp.SendMail, по двум
// причинам, и обе стоили бы боя.
//
// Первая: на проде релей слушает 465 — это НЕЯВНЫЙ TLS, где шифрование
// начинается до приветствия. smtp.SendMail умеет только STARTTLS поверх
// открытого соединения (587) и на 465 висит, ожидая приветствие, которого в
// открытом виде не будет никогда.
//
// Вторая: у smtp.SendMail нет ни таймаута набора, ни дедлайна на сокете.
// Мёртвый релей у нас уже был, и три недели этого никто не заметил.
func (s *SMTPSender) deliver(to string, msg []byte) error {
	host, port, err := net.SplitHostPort(s.Host)
	if err != nil {
		return fmt.Errorf("mail: адрес релея %q: %w", s.Host, err)
	}

	var conn net.Conn
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if port == "465" {
		conn, err = tls.DialWithDialer(dialer, "tcp", s.Host, &tls.Config{ServerName: host})
	} else {
		conn, err = dialer.Dial("tcp", s.Host)
	}
	if err != nil {
		return fmt.Errorf("mail: соединение с %s: %w", s.Host, err)
	}
	_ = conn.SetDeadline(time.Now().Add(sendTimeout))
	defer conn.Close()

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("mail: приветствие %s: %w", s.Host, err)
	}
	defer c.Close()

	if port != "465" {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return fmt.Errorf("mail: STARTTLS: %w", err)
			}
		}
	}
	if s.User != "" {
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(smtp.PlainAuth("", s.User, s.Pass, host)); err != nil {
				return fmt.Errorf("mail: аутентификация: %w", err)
			}
		}
	}
	if err := c.Mail(s.FromAddr); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("mail: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("mail: тело: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: закрытие тела: %w", err)
	}
	return c.Quit()
}

// build собирает MIME вручную. Структура:
//
//	multipart/mixed
//	├── multipart/alternative
//	│   ├── text/plain
//	│   └── text/html
//	└── text/calendar (вложение)
//
// Без вложений внешний слой не создаётся — лишний уровень некоторые
// почтовики показывают как «письмо с приложением» на пустом месте.
func (s *SMTPSender) build(l Letter) ([]byte, error) {
	var b bytes.Buffer
	altBoundary := boundary()

	fmt.Fprintf(&b, "From: %s <%s>\r\n", mime.QEncoding.Encode("utf-8", s.FromName), s.FromAddr)
	if l.ToName != "" {
		fmt.Fprintf(&b, "To: %s <%s>\r\n", mime.QEncoding.Encode("utf-8", l.ToName), l.To)
	} else {
		fmt.Fprintf(&b, "To: %s\r\n", l.To)
	}
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", l.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%s@vshage.app>\r\n", messageID())
	b.WriteString("MIME-Version: 1.0\r\n")
	// Автоответчик «меня нет в офисе» на транзакционное письмо — мусор
	// в обе стороны; заголовки ниже его глушат.
	b.WriteString("Auto-Submitted: auto-generated\r\n")
	b.WriteString("X-Auto-Response-Suppress: All\r\n")

	var mixBoundary string
	if len(l.Attachments) > 0 {
		mixBoundary = boundary()
		fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", mixBoundary)
		fmt.Fprintf(&b, "--%s\r\n", mixBoundary)
	}

	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", altBoundary)

	fmt.Fprintf(&b, "--%s\r\n", altBoundary)
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	writeBase64(&b, []byte(l.Text))

	fmt.Fprintf(&b, "--%s\r\n", altBoundary)
	b.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	writeBase64(&b, []byte(l.HTML))

	fmt.Fprintf(&b, "--%s--\r\n", altBoundary)

	for _, a := range l.Attachments {
		fmt.Fprintf(&b, "\r\n--%s\r\n", mixBoundary)
		fmt.Fprintf(&b, "Content-Type: %s\r\n", a.MIMEType)
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&b, "Content-Disposition: attachment; filename=%q\r\n\r\n", a.Filename)
		writeBase64(&b, a.Content)
	}
	if mixBoundary != "" {
		fmt.Fprintf(&b, "\r\n--%s--\r\n", mixBoundary)
	}
	return b.Bytes(), nil
}

// writeBase64 пишет тело кусками по 76 символов — так требует RFC 2045, и
// релеи, которые режут длинные строки, письмо не портят.
func writeBase64(b *bytes.Buffer, data []byte) {
	enc := base64.StdEncoding.EncodeToString(data)
	for len(enc) > 76 {
		b.WriteString(enc[:76])
		b.WriteString("\r\n")
		enc = enc[76:]
	}
	if enc != "" {
		b.WriteString(enc)
		b.WriteString("\r\n")
	}
}

func boundary() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	return "vshage-" + hex.EncodeToString(raw[:])
}

func messageID() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	return hex.EncodeToString(raw[:])
}
