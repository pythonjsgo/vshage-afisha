package mail

import (
	"fmt"
	"strings"
	"time"
)

// Event — то, что нужно календарю и письму. Собирается один раз в
// internal/events и передаётся сюда: пакет mail ничего не знает про базу.
type Event struct {
	ID          string
	Title       string
	Description string
	Start       time.Time
	End         *time.Time // nil ⇒ считаем два часа
	VenueName   string
	Address     string
	City        string
	OnlineURL   string
	PhotoURL    string
	PageURL     string // публичная страница события
	Organizer   string
	PriceNote   string // «Бесплатно» / «от 500 ₽»
}

// Location собирает адрес одной строкой в том виде, в каком его показывают
// человеку и кладут в карточку календаря.
func (e Event) Location() string {
	parts := make([]string, 0, 3)
	for _, p := range []string{e.VenueName, e.Address, e.City} {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		dup := false
		for _, seen := range parts {
			if strings.EqualFold(seen, p) || strings.Contains(strings.ToLower(seen), strings.ToLower(p)) {
				dup = true
				break
			}
		}
		if !dup {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 && e.OnlineURL != "" {
		return "Онлайн"
	}
	return strings.Join(parts, ", ")
}

// MapURL — ссылка на карту по адресу. Яндекс, а не Google: аудитория в РФ,
// и у Google карт там неполный поиск по адресам.
func (e Event) MapURL() string {
	loc := e.Location()
	if loc == "" || loc == "Онлайн" {
		return ""
	}
	return "https://yandex.ru/maps/?text=" + urlQueryEscape(loc)
}

func (e Event) endOrDefault() time.Time {
	if e.End != nil && e.End.After(e.Start) {
		return *e.End
	}
	return e.Start.Add(2 * time.Hour)
}

// ICS собирает карточку календаря (RFC 5545). Приложена к обоим письмам:
// человек кладёт событие в свой календарь одним касанием, и дальше о нём
// напоминает его собственный телефон, а не только мы.
//
// alarmBefore — за сколько до начала календарь сам напомнит. Ноль ⇒ без
// напоминания.
func ICS(e Event, uid string, alarmBefore time.Duration) []byte {
	var b strings.Builder
	w := func(line string) { b.WriteString(fold(line)) }

	w("BEGIN:VCALENDAR")
	w("VERSION:2.0")
	w("PRODID:-//Vshage//Afisha//RU")
	w("CALSCALE:GREGORIAN")
	w("METHOD:PUBLISH")
	w("BEGIN:VEVENT")
	w("UID:" + uid + "@afisha.vshage.app")
	w("DTSTAMP:" + utc(time.Now()))
	w("DTSTART:" + utc(e.Start))
	w("DTEND:" + utc(e.endOrDefault()))
	w("SUMMARY:" + esc(e.Title))
	if d := descriptionFor(e); d != "" {
		w("DESCRIPTION:" + esc(d))
	}
	if loc := e.Location(); loc != "" {
		w("LOCATION:" + esc(loc))
	}
	if e.PageURL != "" {
		w("URL:" + esc(e.PageURL))
	}
	if e.Organizer != "" {
		w("ORGANIZER;CN=" + esc(e.Organizer) + ":MAILTO:noreply@vshage.app")
	}
	w("STATUS:CONFIRMED")
	w("TRANSP:OPAQUE")
	if alarmBefore > 0 {
		w("BEGIN:VALARM")
		w("ACTION:DISPLAY")
		w(fmt.Sprintf("TRIGGER:-PT%dM", int(alarmBefore.Minutes())))
		w("DESCRIPTION:" + esc(e.Title))
		w("END:VALARM")
	}
	w("END:VEVENT")
	w("END:VCALENDAR")
	return []byte(b.String())
}

func descriptionFor(e Event) string {
	parts := make([]string, 0, 3)
	if d := strings.TrimSpace(e.Description); d != "" {
		if len([]rune(d)) > 600 {
			d = string([]rune(d)[:600]) + "…"
		}
		parts = append(parts, d)
	}
	if e.OnlineURL != "" {
		parts = append(parts, "Подключение: "+e.OnlineURL)
	}
	if e.PageURL != "" {
		parts = append(parts, e.PageURL)
	}
	return strings.Join(parts, "\n\n")
}

func utc(t time.Time) string { return t.UTC().Format("20060102T150405Z") }

// esc экранирует спецсимволы значения по RFC 5545 §3.3.11. Порядок важен:
// обратный слэш первым, иначе он подменит слэши, добавленные следом.
func esc(s string) string {
	r := strings.NewReplacer(
		"\\", "\\\\",
		";", "\\;",
		",", "\\,",
		"\r\n", "\\n",
		"\n", "\\n",
		"\r", "\\n",
	)
	return r.Replace(s)
}

// fold режет строку на 75 ОКТЕТОВ (не рун) по RFC 5545 §3.1 и не рвёт при
// этом многобайтную букву — иначе кириллица в календаре превращается в
// вопросительные знаки. Продолжение начинается с пробела.
func fold(line string) string {
	const limit = 75
	b := []byte(line)
	if len(b) <= limit {
		return line + "\r\n"
	}
	var out strings.Builder
	first := true
	for len(b) > 0 {
		max := limit
		if !first {
			max = limit - 1 // ведущий пробел тоже считается
		}
		if len(b) <= max {
			if !first {
				out.WriteByte(' ')
			}
			out.Write(b)
			out.WriteString("\r\n")
			break
		}
		cut := max
		// Не резать по середине UTF-8: откатываемся к началу символа.
		for cut > 0 && b[cut]&0xC0 == 0x80 {
			cut--
		}
		if cut == 0 {
			cut = max
		}
		if !first {
			out.WriteByte(' ')
		}
		out.Write(b[:cut])
		out.WriteString("\r\n")
		b = b[cut:]
		first = false
	}
	return out.String()
}

// urlQueryEscape — минимальное экранирование для ссылки на карту. Стандартный
// url.QueryEscape ставит «+» вместо пробела, а Яндекс.Карты его показывают
// плюсом в строке поиска.
func urlQueryEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
