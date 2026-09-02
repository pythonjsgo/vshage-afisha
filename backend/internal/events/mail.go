package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pythonjsgo/vshage-afisha/internal/mail"
	"github.com/pythonjsgo/vshage-afisha/internal/regform"
)

// Письма гостю: подтверждение сразу после записи и напоминание за шесть часов
// до начала (директива фаундера 02.09). Оба уходят через тот же outbox, что и
// телеграм: транзакция записи либо кладёт письмо в очередь, либо не создаёт
// записи.

// ReminderLead — за сколько до начала уходит напоминание.
const ReminderLead = 6 * time.Hour

// reminderTick — как часто цикл смотрит, кому пора. Минута против шести часов
// даёт точность, которую человек не замечает, и один дешёвый запрос в минуту.
const reminderTick = time.Minute

// publicBaseURL — куда ведут ссылки в письмах. Переопределяется из окружения
// на стенде; дефолт боевой, потому что письмо с ссылкой на localhost хуже,
// чем письмо без ссылки.
var publicBaseURL = "https://afisha.vshage.app"

// SetPublicBaseURL задаёт базовый адрес публичных страниц.
func SetPublicBaseURL(u string) {
	if u = strings.TrimRight(strings.TrimSpace(u), "/"); u != "" {
		publicBaseURL = u
	}
}

// mailEvent — поля события, нужные письму и карточке календаря. Отдельная
// структура, потому что RegisterPublic грузит событие «под запись», а письму
// нужен адрес, обложка и организатор.
type mailEvent struct {
	ID          string
	Title       string
	Description string
	Start       time.Time
	End         *time.Time
	VenueName   string
	Address     string
	City        string
	Location    string
	OnlineURL   string
	PhotoURL    string
	Organizer   string
	OrganizerID string
	PriceType   string
	PriceMin    *int
	PriceMax    *int
	Currency    string
}

const mailEventCols = `
	e.id, e.title, COALESCE(e.description,''), e.start_time, e.end_time,
	COALESCE(d.venue_name,''), COALESCE(d.address,''), COALESCE(d.city,''),
	COALESCE(e.location,''), COALESCE(d.online_url,''), COALESCE(e.photo_url,''),
	COALESCE(p.name,''), COALESCE(e.organizer_id::text,''),
	COALESCE(d.price_type,'free'), d.price_min, d.price_max, COALESCE(d.currency,'RUB')
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMailEvent(row rowScanner) (mailEvent, error) {
	var m mailEvent
	err := row.Scan(&m.ID, &m.Title, &m.Description, &m.Start, &m.End,
		&m.VenueName, &m.Address, &m.City, &m.Location, &m.OnlineURL, &m.PhotoURL,
		&m.Organizer, &m.OrganizerID, &m.PriceType, &m.PriceMin, &m.PriceMax, &m.Currency)
	return m, err
}

func (m mailEvent) toMail() mail.Event {
	// location — свободная строка, которую организатор пишет, когда не
	// заполнил venue/address. Берём её как адрес, чтобы карта нашла хоть что-то.
	addr := m.Address
	if addr == "" && m.VenueName == "" {
		addr = m.Location
	}
	return mail.Event{
		ID:          m.ID,
		Title:       m.Title,
		Description: m.Description,
		Start:       m.Start,
		End:         m.End,
		VenueName:   m.VenueName,
		Address:     addr,
		City:        m.City,
		OnlineURL:   m.OnlineURL,
		PhotoURL:    absoluteURL(m.PhotoURL),
		PageURL:     publicBaseURL + "/" + m.ID,
		Organizer:   m.Organizer,
		PriceNote:   m.priceNote(),
	}
}

func (m mailEvent) priceNote() string {
	switch m.PriceType {
	case "free", "":
		return "Бесплатно"
	case "donation":
		return "Донат"
	}
	cur := "₽"
	if m.Currency != "" && m.Currency != "RUB" {
		cur = m.Currency
	}
	switch {
	case m.PriceMin != nil && m.PriceMax != nil && *m.PriceMin != *m.PriceMax:
		return fmt.Sprintf("%d–%d %s", *m.PriceMin, *m.PriceMax, cur)
	case m.PriceMin != nil:
		return fmt.Sprintf("от %d %s", *m.PriceMin, cur)
	case m.PriceMax != nil:
		return fmt.Sprintf("до %d %s", *m.PriceMax, cur)
	}
	return ""
}

// absoluteURL достраивает относительный путь обложки до полного адреса:
// картинку в письме грузит почтовик, а не наш фронт, и «/uploads/…» он
// разрешить не может.
func absoluteURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" || strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	if !strings.HasPrefix(u, "/") {
		u = "/" + u
	}
	return publicBaseURL + u
}

// answersFor собирает блок «что ты указал(а)» в порядке, в котором человек
// видел форму: сначала документные поля, потом свои вопросы организатора,
// потом контакты.
func answersFor(fields []regform.Field, clean regform.Clean) []mail.KV {
	out := make([]mail.KV, 0, len(fields)+4)
	add := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			out = append(out, mail.KV{Label: label, Value: value})
		}
	}
	add("ФИО", clean.FullName)
	for _, f := range fields {
		add(regform.LabelFor(fields, f.Key), clean.Answers[f.Key])
	}
	add("Почта", clean.Email)
	add("Телефон", clean.Phone)
	add("Телеграм", clean.TGDisplay)
	return out
}

// enqueueGuestMail кладёт письмо-подтверждение в очередь. Без адреса письма
// нет — это не ошибка записи: почта обязательна не у каждого события.
func enqueueGuestMail(ctx context.Context, tx pgx.Tx, ev mailEvent, form regform.FormConfig,
	fields []regform.Field, clean regform.Clean, regID, status string) error {

	if clean.Email == "" {
		return nil
	}
	// Событие с ручной модерацией кладёт человека в лист ожидания. Слать ему
	// «место за тобой» — прямая ложь: места может и не оказаться.
	kind := mail.KindConfirm
	if status != "registered" {
		kind = mail.KindWaitlist
	}
	s := mail.Signup{
		Kind:      kind,
		Event:     ev.toMail(),
		GuestName: clean.DisplayName(),
		Email:     clean.Email,
		PassNote:  form.PassNote,
		Answers:   answersFor(fields, clean),
		RegID:     regID,
	}
	if err := enqueueLetter(ctx, tx, s, regID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`UPDATE event_registrations SET confirm_mail_at = NOW() WHERE id = $1`, regID)
	return err
}

func enqueueLetter(ctx context.Context, tx pgx.Tx, s mail.Signup, regID string) error {
	subject, html, text := mail.Render(s)
	alarm := time.Duration(0)
	if s.Kind == mail.KindConfirm {
		alarm = ReminderLead
	}
	job := mailJob{
		To:         s.Email,
		EventStart: s.Event.Start,
		ToName:     s.GuestName,
		Subject:    subject,
		HTML:       html,
		Text:       text,
		ICSName:    "vshage-event.ics",
		ICS:        mail.ICS(s.Event, s.Event.ID+"-"+regID, alarm),
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return enqueueChannel(ctx, tx, channelEmail, s.Email, string(payload))
}

// enqueueOrganizerPush ставит в очередь уведомление в приложение владельцу
// события и админам платформы. Админ в списке потому, что события заводятся
// под аккаунтами организаторов (ШАГ, mavantura), а следит за потоком записей
// владелец платформы — без этого уведомление уходило бы в аккаунт, в который
// никто не заходит.
func enqueueOrganizerPush(ctx context.Context, tx pgx.Tx, ev mailEvent, guest string, taken int, capacity *int) error {
	seats := fmt.Sprintf("%d", taken)
	if capacity != nil && *capacity > 0 {
		seats = fmt.Sprintf("%d/%d", taken, *capacity)
	}
	body := fmt.Sprintf("%s · %s · мест занято %s", guest, ev.Title, seats)

	// Ошибка ПОИСКА получателей не должна ронять запись человека — но просто
	// вернуть nil мало: неудачный запрос переводит транзакцию в aborted, и
	// тогда падает уже COMMIT, то есть регистрация всё равно теряется, а в
	// логе стоит безобидная строчка. Поэтому поиск идёт во ВЛОЖЕННОЙ
	// транзакции: pgx делает из неё SAVEPOINT, и откат возвращает внешнюю
	// транзакцию в рабочее состояние.
	targets, err := pushTargets(ctx, tx, ev)
	if err != nil {
		log.Printf("notify: получатели пуша для %s: %v", ev.ID, err)
		return nil
	}

	for _, id := range targets {
		payload, err := json.Marshal(pushJob{
			ProfileID: id,
			Title:     "Новая запись",
			Body:      body,
			EventID:   ev.ID,
		})
		if err != nil {
			return err
		}
		if err := enqueueChannel(ctx, tx, channelPush, id, string(payload)); err != nil {
			return err
		}
	}
	return nil
}

// afterSignup — всё, что происходит после успешной записи, кроме телеграма:
// письмо гостю и уведомление организатору. Вызывается ВНУТРИ транзакции
// записи, поэтому либо запись и обе доставки в очереди, либо ничего.
func afterSignup(ctx context.Context, tx pgx.Tx, eventID string, form regform.FormConfig,
	fields []regform.Field, clean regform.Clean, regID, status string, taken int, capacity *int) error {

	ev, err := scanMailEvent(tx.QueryRow(ctx, `
		SELECT `+mailEventCols+`
		FROM events e
		LEFT JOIN organizer_event_details d ON d.event_id = e.id
		LEFT JOIN profiles p ON p.id = e.organizer_id
		WHERE e.id = $1`, eventID))
	if err != nil {
		return err
	}
	if err := enqueueGuestMail(ctx, tx, ev, form, fields, clean, regID, status); err != nil {
		return err
	}
	return enqueueOrganizerPush(ctx, tx, ev, clean.DisplayName(), taken, capacity)
}

func pushTargets(ctx context.Context, tx pgx.Tx, ev mailEvent) ([]string, error) {
	sub, err := tx.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer sub.Rollback(ctx)

	// id сравнивается как uuid, а не приведением к тексту: id::text убивает
	// primary key и превращает поиск в чтение всей таблицы пользователей —
	// на каждую запись, под блокировкой строки события.
	rows, err := sub.Query(ctx, `
		SELECT id::text FROM profiles
		WHERE (id = NULLIF($1,'')::uuid OR is_admin) AND status = 'active'`, ev.OrganizerID)
	if err != nil {
		return nil, err
	}
	var targets []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		targets = append(targets, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		// pgx прячет отказ прав именно здесь: без этой проверки «нет доступа
		// к profiles» выглядит как «получателей нет».
		return nil, err
	}
	return targets, sub.Commit(ctx)
}

// StartReminderLoop раз в минуту ищет записи, которым пора напомнить, и
// кладёт им письма в ту же очередь.
//
// Условие created_at < start - lead намеренное: тому, кто записался за три
// часа до начала, «напоминание» прилетело бы сразу следом за подтверждением
// и читалось бы как сбой.
func StartReminderLoop(ctx context.Context, pool *pgxpool.Pool) {
	log.Printf("notify: напоминания за %s до события включены", ReminderLead)
	go func() {
		t := time.NewTicker(reminderTick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if n, err := sweepReminders(ctx, pool); err != nil {
					log.Printf("notify: напоминания: %v", err)
				} else if n > 0 {
					log.Printf("notify: поставлено напоминаний: %d", n)
				}
			}
		}
	}()
}

func sweepReminders(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	rows, err := pool.Query(ctx, `
		SELECT r.id::text, COALESCE(r.signup_email,''), COALESCE(r.signup_full_name,''),
		       COALESCE(r.signup_name,''), COALESCE(r.signup_phone,''), COALESCE(r.signup_tg,''),
		       COALESCE(r.signup_answers,'{}'::jsonb),
		       COALESCE(d.reg_form,'{}'::jsonb), COALESCE(d.reg_fields,'[]'::jsonb),
		       `+mailEventCols+`
		FROM event_registrations r
		JOIN events e ON e.id = r.event_id
		LEFT JOIN organizer_event_details d ON d.event_id = e.id
		LEFT JOIN profiles p ON p.id = e.organizer_id
		WHERE r.reminder_mail_at IS NULL
		  AND r.status = 'registered'
		  AND r.signup_email IS NOT NULL AND r.signup_email <> ''
		  AND e.status = 'published'
		  AND e.start_time > NOW()
		  AND e.start_time <= NOW() + $1::interval
		  AND r.created_at < e.start_time - $1::interval
		LIMIT 200`, fmt.Sprintf("%d seconds", int(ReminderLead.Seconds())))
	if err != nil {
		return 0, err
	}

	type pending struct {
		regID    string
		email    string
		fullName string
		name     string
		phone    string
		tg       string
		answers  []byte
		formRaw  []byte
		fieldRaw []byte
		ev       mailEvent
	}
	var todo []pending
	for rows.Next() {
		var p pending
		dest := []any{&p.regID, &p.email, &p.fullName, &p.name, &p.phone, &p.tg,
			&p.answers, &p.formRaw, &p.fieldRaw,
			&p.ev.ID, &p.ev.Title, &p.ev.Description, &p.ev.Start, &p.ev.End,
			&p.ev.VenueName, &p.ev.Address, &p.ev.City, &p.ev.Location, &p.ev.OnlineURL, &p.ev.PhotoURL,
			&p.ev.Organizer, &p.ev.OrganizerID, &p.ev.PriceType, &p.ev.PriceMin, &p.ev.PriceMax, &p.ev.Currency}
		if err := rows.Scan(dest...); err != nil {
			rows.Close()
			return 0, err
		}
		todo = append(todo, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	sent := 0
	for _, p := range todo {
		form := regform.Decode(p.formRaw)
		fields := regform.DecodeFields(p.fieldRaw)
		var answers map[string]string
		_ = json.Unmarshal(p.answers, &answers)

		clean := regform.Clean{
			Name: p.name, FullName: p.fullName, Email: p.email,
			Phone: p.phone, TGDisplay: p.tg, Answers: answers,
		}
		s := mail.Signup{
			Kind:      mail.KindReminder,
			Event:     p.ev.toMail(),
			GuestName: clean.DisplayName(),
			Email:     p.email,
			PassNote:  form.PassNote,
			Answers:   answersFor(fields, clean),
			RegID:     p.regID,
		}

		// Отметка и письмо — одной транзакцией. Иначе перезапуск между ними
		// либо роняет письмо, либо шлёт его повторно каждую минуту.
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return sent, err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE event_registrations SET reminder_mail_at = NOW()
			WHERE id = $1 AND reminder_mail_at IS NULL`, p.regID)
		if err != nil {
			_ = tx.Rollback(ctx)
			return sent, err
		}
		if tag.RowsAffected() == 0 { // успел другой инстанс
			_ = tx.Rollback(ctx)
			continue
		}
		if err := enqueueLetter(ctx, tx, s, p.regID); err != nil {
			_ = tx.Rollback(ctx)
			return sent, err
		}
		if err := tx.Commit(ctx); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}
