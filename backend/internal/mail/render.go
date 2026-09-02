package mail

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"strings"
	"time"
)

//go:embed templates/letter.html
var letterHTML string

//go:embed templates/letter.txt
var letterText string

var (
	htmlTpl = template.Must(template.New("letter").Parse(letterHTML))
	textTpl = template.Must(template.New("letter.txt").Parse(letterText))
)

// MSK — время в письмах всегда московское и всегда подписано. Человек читает
// письмо в поезде и в другом часовом поясе; «18:00» без пояса — это ошибка,
// которую замечают на вокзале.
var MSK = time.FixedZone("MSK", 3*60*60)

// KV — строка «поле → значение» в блоке «твои данные».
type KV struct {
	Label string
	Value string
}

// Kind различает два письма. Разделять шаблоны не стали намеренно: они
// обязаны выглядеть одинаково, а расхождение вёрстки между «подтверждением»
// и «напоминанием» человек читает как подделку.
type Kind string

const (
	KindConfirm  Kind = "confirm"
	KindReminder Kind = "reminder"
)

// Signup — всё, что письмо знает о конкретной записи.
type Signup struct {
	Kind      Kind
	Event     Event
	GuestName string
	Email     string
	// PassNote — строка организатора про пропуск («ФИО как в паспорте, вход
	// по спискам»). Пустая — блок не рисуется.
	PassNote string
	// Answers — то, что человек вписал в форму. Показываем обратно, чтобы
	// опечатку в ФИО он увидел сейчас, а не на входе.
	Answers []KV
	RegID   string
}

type letterData struct {
	Reminder     bool
	Kicker       string
	Headline     string
	Lead         string
	Title        string
	CoverURL     string
	WhenLong     string
	WhenTime     string
	Location     string
	MapURL       string
	PageURL      string
	OnlineURL    string
	PriceNote    string
	Organizer    string
	PassNote     string
	Answers      []KV
	GuestName    string
	RegShort     string
	CalendarHint string
}

// Render собирает предмет письма и оба тела.
func Render(s Signup) (subject, html, text string) {
	start := s.Event.Start.In(MSK)
	d := letterData{
		Reminder:  s.Kind == KindReminder,
		Title:     s.Event.Title,
		CoverURL:  s.Event.PhotoURL,
		WhenLong:  LongDate(start),
		WhenTime:  start.Format("15:04") + " МСК",
		Location:  s.Event.Location(),
		MapURL:    s.Event.MapURL(),
		PageURL:   s.Event.PageURL,
		OnlineURL: s.Event.OnlineURL,
		PriceNote: s.Event.PriceNote,
		Organizer: s.Event.Organizer,
		PassNote:  s.PassNote,
		Answers:   s.Answers,
		GuestName: firstName(s.GuestName),
		RegShort:  shortID(s.RegID),
	}
	if s.Kind == KindReminder {
		d.Kicker = "СЕГОДНЯ"
		d.Headline = "Через " + humanLeft(time.Until(s.Event.Start))
		d.Lead = "Напоминаем: ты записан(а). Ниже — время, адрес и что взять с собой."
		d.CalendarHint = "Карточка события снова во вложении — если ещё не в календаре, добавь."
		subject = fmt.Sprintf("Сегодня в %s — %s", start.Format("15:04"), s.Event.Title)
	} else {
		d.Kicker = "ТЫ В СПИСКЕ"
		d.Headline = "Записали"
		d.Lead = "Место за тобой. Ниже — всё, что нужно знать до входа."
		d.CalendarHint = "К письму приложена карточка события — открой вложение, и оно встанет в календарь с напоминанием."
		subject = fmt.Sprintf("Ты записан(а): %s — %s", s.Event.Title, ShortDate(start))
	}

	var hb, tb bytes.Buffer
	if err := htmlTpl.Execute(&hb, d); err != nil {
		// Шаблон вшит в бинарь и разобран на старте, поэтому сюда можно
		// попасть только с битыми данными. Письмо всё равно уходит —
		// текстовой части достаточно, чтобы человек узнал, куда идти.
		hb.Reset()
		hb.WriteString("<pre>" + template.HTMLEscapeString(plainBody(d)) + "</pre>")
	}
	if err := textTpl.Execute(&tb, d); err != nil {
		tb.Reset()
		tb.WriteString(plainBody(d))
	}
	return subject, hb.String(), tb.String()
}

func plainBody(d letterData) string {
	return strings.Join([]string{
		d.Title, d.WhenLong + ", " + d.WhenTime, d.Location, d.PageURL,
	}, "\n")
}

var ruMonths = [...]string{
	"января", "февраля", "марта", "апреля", "мая", "июня",
	"июля", "августа", "сентября", "октября", "ноября", "декабря",
}

var ruWeekdays = [...]string{
	"воскресенье", "понедельник", "вторник", "среда",
	"четверг", "пятница", "суббота",
}

// LongDate — «четверг, 10 сентября».
func LongDate(t time.Time) string {
	return fmt.Sprintf("%s, %d %s", ruWeekdays[int(t.Weekday())], t.Day(), ruMonths[int(t.Month())-1])
}

// ShortDate — «10 сентября».
func ShortDate(t time.Time) string {
	return fmt.Sprintf("%d %s", t.Day(), ruMonths[int(t.Month())-1])
}

// humanLeft печатает остаток по-русски с правильным падежом. Письмо
// отправляется не ровно за шесть часов (очередь тикает раз в минуту), и
// «через 6 часов» в письме, ушедшем за 5 ч 47 мин, — мелкая ложь, которую
// читатель замечает.
func humanLeft(d time.Duration) string {
	mins := int(d.Minutes())
	if mins < 1 {
		return "несколько минут"
	}
	if mins < 60 {
		return plural(mins, "минуту", "минуты", "минут")
	}
	hours := (mins + 30) / 60
	return plural(hours, "час", "часа", "часов")
}

func plural(n int, one, few, many string) string {
	form := many
	switch {
	case n%10 == 1 && n%100 != 11:
		form = one
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		form = few
	}
	return fmt.Sprintf("%d %s", n, form)
}

func firstName(full string) string {
	f := strings.Fields(strings.TrimSpace(full))
	switch len(f) {
	case 0:
		return ""
	case 1:
		return f[0]
	default:
		// ФИО пишут «Фамилия Имя Отчество» — обращаемся по второму слову.
		return f[1]
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
