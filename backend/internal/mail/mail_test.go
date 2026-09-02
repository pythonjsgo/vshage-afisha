package mail

import (
	"strings"
	"testing"
	"time"
)

func sampleEvent() Event {
	start := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC) // 18:00 МСК
	return Event{
		ID:          "f1db277e-0beb-4d56-9f64-73702287d00f",
		Title:       "Панельная дискуссия с Сергеем Протопоповым",
		Description: "Разговор про смыслы, карьеру и то, зачем всё это.",
		Start:       start,
		VenueName:   "РАНХиГС",
		Address:     "Москва, пр-т Вернадского, 82",
		City:        "Москва",
		PhotoURL:    "https://afisha.vshage.app/uploads/cover.jpg",
		PageURL:     "https://afisha.vshage.app/f1db277e-0beb-4d56-9f64-73702287d00f",
		Organizer:   "Julia",
		PriceNote:   "Бесплатно",
	}
}

// Правило, ради которого существует этот тест: карточку календаря читает
// чужая программа по строгому формату. Строка длиннее 75 октетов или
// разрезанная посреди кириллической буквы превращает событие в мусор, и
// увидит это человек, а не мы, — уже у себя в календаре.
func TestICSLinesFitTheFormatAndKeepCyrillicIntact(t *testing.T) {
	ics := string(ICS(sampleEvent(), "reg-1", 6*time.Hour))

	if !strings.HasSuffix(ics, "END:VCALENDAR\r\n") {
		t.Fatalf("календарь не закрыт: %q", tail(ics))
	}
	for i, line := range strings.Split(strings.TrimSuffix(ics, "\r\n"), "\r\n") {
		if len(line) > 75 {
			t.Errorf("строка %d длиной %d октетов: %q", i, len(line), line)
		}
	}
	// Склеиваем обратно по правилу разворачивания (RFC 5545 §3.1) и
	// проверяем, что текст пережил резку.
	unfolded := strings.ReplaceAll(ics, "\r\n ", "")
	for _, want := range []string{
		"SUMMARY:Панельная дискуссия с Сергеем Протопоповым",
		"DTSTART:20260910T150000Z",
		"LOCATION:РАНХиГС\\, Москва\\, пр-т Вернадского\\, 82",
		"TRIGGER:-PT360M",
	} {
		if !strings.Contains(unfolded, want) {
			t.Errorf("в календаре нет %q", want)
		}
	}
	if strings.Contains(unfolded, "�") {
		t.Error("в календаре битая кодировка")
	}
}

func TestICSWithoutAlarmHasNoAlarmBlock(t *testing.T) {
	if strings.Contains(string(ICS(sampleEvent(), "r", 0)), "BEGIN:VALARM") {
		t.Error("будильник поставлен там, где его не просили")
	}
}

// Адрес собирается из трёх полей, которые организатор заполняет как хочет.
// «Москва, Москва, пр-т Вернадского, 82» в письме выглядит как баг, потому
// что это он и есть.
func TestLocationDoesNotRepeatTheCity(t *testing.T) {
	e := sampleEvent()
	if got := e.Location(); got != "РАНХиГС, Москва, пр-т Вернадского, 82" {
		t.Errorf("адрес: %q", got)
	}
	online := Event{OnlineURL: "https://meet.vshage.app/x"}
	if got := online.Location(); got != "Онлайн" {
		t.Errorf("онлайн-событие без адреса: %q", got)
	}
	if online.MapURL() != "" {
		t.Error("маршрут до онлайна строить некуда")
	}
}

func TestConfirmationCarriesEverythingNeededToShowUp(t *testing.T) {
	subject, html, text := Render(Signup{
		Kind:      KindConfirm,
		Event:     sampleEvent(),
		GuestName: "Иванов Иван Иванович",
		Email:     "ivan@example.com",
		PassNote:  "Вход по спискам, нужен паспорт",
		Answers:   []KV{{Label: "ФИО", Value: "Иванов Иван Иванович"}, {Label: "Дата рождения", Value: "01.01.1990"}},
		RegID:     "abcdef1234",
	})
	if !strings.Contains(subject, "Панельная дискуссия") || !strings.Contains(subject, "10 сентября") {
		t.Errorf("тема письма не говорит, на что и когда: %q", subject)
	}
	for _, want := range []string{
		"четверг, 10 сентября", "18:00 МСК", "РАНХиГС", "Вход по спискам",
		"01.01.1990", "yandex.ru/maps", "afisha.vshage.app",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("в HTML нет %q", want)
		}
		if !strings.Contains(text, want) {
			t.Errorf("в тексте нет %q", want)
		}
	}
	// Обращение по имени, а не по фамилии: «Иванов, место за тобой» читается
	// как повестка. И строчная после запятой — «Иван, Место за тобой» выдаёт
	// машинную склейку, это уже вылезло на DEV.
	if !strings.Contains(html, "Иван, место за тобой") {
		t.Errorf("обращение собрано неправильно: %q", opening(html))
	}
}

func TestReminderSaysHowLongIsLeft(t *testing.T) {
	e := sampleEvent()
	e.Start = time.Now().Add(5*time.Hour + 47*time.Minute)
	subject, html, _ := Render(Signup{Kind: KindReminder, Event: e, Email: "a@b.co"})
	if !strings.Contains(html, "Через 6 часов") {
		t.Errorf("остаток посчитан не по-человечески: %q", firstHeadline(html))
	}
	if !strings.HasPrefix(subject, "Сегодня в ") {
		t.Errorf("тема напоминания: %q", subject)
	}
}

// Русский счёт — не форматная мелочь: «через 1 часов» в письме читается как
// машинный сбой и подрывает доверие ко всему остальному тексту.
func TestRussianPluralsAreCorrect(t *testing.T) {
	cases := map[time.Duration]string{
		61 * time.Minute:  "1 час",
		59 * time.Minute:  "59 минут",
		2*time.Hour + 1:   "2 часа",
		5 * time.Hour:     "5 часов",
		21 * time.Hour:    "21 час",
		30 * time.Minute:  "30 минут",
		time.Minute * 21:  "21 минуту",
		time.Minute * 22:  "22 минуты",
		time.Second * 100: "1 минуту",
	}
	for d, want := range cases {
		if got := humanLeft(d); got != want {
			t.Errorf("humanLeft(%s) = %q, ожидали %q", d, got, want)
		}
	}
}

// Письмо собирается из данных организатора и гостя. Кавычка в названии не
// должна ломать разметку — html/template экранирует, и это надо держать.
func TestUserTextIsEscapedInHTML(t *testing.T) {
	e := sampleEvent()
	e.Title = `Лекция "<script>alert(1)</script>"`
	_, html, _ := Render(Signup{Kind: KindConfirm, Event: e, Email: "a@b.co"})
	if strings.Contains(html, "<script>") {
		t.Fatal("название события уехало в разметку неэкранированным")
	}
}

// Текстовую часть читает человек, а не браузер. Прогон через html/template
// выдавал ему «Ёлки &amp;amp; Палки — &amp;#34;вечер&amp;#34;».
func TestPlainTextPartIsNotHTMLEscaped(t *testing.T) {
	e := sampleEvent()
	e.Title = `Ёлки & Палки — "вечер"`
	e.VenueName = `Бар "Стрелка" & Ко`
	_, _, text := Render(Signup{Kind: KindConfirm, Event: e, Email: "a@b.co"})
	for _, bad := range []string{"&amp;", "&#34;", "&quot;", "&#39;"} {
		if strings.Contains(text, bad) {
			t.Errorf("в текстовой части экранированный HTML (%s):\n%s", bad, text)
		}
	}
	if !strings.Contains(text, `Ёлки & Палки — "вечер"`) {
		t.Error("название в текстовой части искажено")
	}
}

// Ручная модерация кладёт человека в лист ожидания. Сказать ему «место за
// тобой» — соврать: места может и не оказаться.
func TestWaitlistLetterDoesNotPromiseASeat(t *testing.T) {
	subject, html, text := Render(Signup{Kind: KindWaitlist, Event: sampleEvent(), Email: "a@b.co"})
	if !strings.Contains(subject, "Заявка принята") {
		t.Errorf("тема: %q", subject)
	}
	for _, body := range []string{html, text} {
		if strings.Contains(body, "Место за тобой") || strings.Contains(body, "место за тобой") {
			t.Error("письму из листа ожидания обещано место")
		}
		if !strings.Contains(body, "подтверждают") && !strings.Contains(body, "подтвердят") {
			t.Error("не сказано, что запись ещё подтверждают")
		}
	}
}

func TestMIMEHasBothBodiesAndTheAttachment(t *testing.T) {
	s := &SMTPSender{FromAddr: "noreply@vshage.app", FromName: "Вшаге"}
	msg, err := s.build(Letter{
		To: "ivan@example.com", ToName: "Иван", Subject: "Тема письма",
		HTML: "<b>hi</b>", Text: "hi",
		Attachments: []Attachment{{Filename: "e.ics", MIMEType: "text/calendar", Content: []byte("BEGIN:VCALENDAR")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := string(msg)
	for _, want := range []string{
		"multipart/mixed", "multipart/alternative",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Type: text/html; charset=utf-8",
		`filename="e.ics"`, "Auto-Submitted: auto-generated",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("в письме нет %q", want)
		}
	}
	// Кириллица в теме обязана быть закодирована — иначе часть релеев режет
	// заголовок, и человек видит «=?...?» или пустую тему.
	if strings.Contains(out, "Subject: Тема письма") {
		t.Error("тема ушла сырым UTF-8")
	}
	if strings.Count(out, "\r\n\r\n--") < 1 {
		t.Error("границы частей расставлены неправильно")
	}
}

func opening(html string) string {
	i := strings.Index(html, "Иван")
	if i < 0 {
		return "(нет имени в письме)"
	}
	j := strings.Index(html[i:], "<")
	return html[i : i+j]
}

func tail(s string) string {
	if len(s) > 60 {
		return s[len(s)-60:]
	}
	return s
}

func firstHeadline(html string) string {
	i := strings.Index(html, "class=\"h1\"")
	if i < 0 {
		return ""
	}
	j := strings.Index(html[i:], ">")
	k := strings.Index(html[i+j:], "<")
	return html[i+j+1 : i+j+k]
}
