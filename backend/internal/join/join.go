// Package join — анкета на вход в закрытую сеть (vshage.app/join).
//
// Страница стоит под платным трафиком Яндекс.Директа. Из этого вытекает вся
// осторожность пакета: отказать настоящему человеку здесь дороже, чем принять
// мусорную строку, — за клик уже заплачено, а второй раз он не придёт. Поэтому
// лимитер широкий, honeypot молчаливый, а каждое сообщение об ошибке говорит,
// ЧТО именно поправить.
package join

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"unicode/utf8"

	// Разбор юзернейма берём у веб-регистрации, а не пишем свой: правило
	// «что человек может вставить в это поле» обязано быть одно на сервис.
	// Направление импорта безопасно — webreg зависит от events, events ни от
	// кого из них, цикла нет.
	"github.com/pythonjsgo/vshage-afisha/internal/webreg"
)

// Universities — вузы в выпадающем списке.
//
// ВТОРАЯ КОПИЯ ЭТОГО СПИСКА ЖИВЁТ НА ФРОНТЕ: frontend/src/lib/join.ts. Списки
// связаны только через код (msu, hse, …), и разъехавшись, они дают тихий
// отказ: человек выбирает вуз, которого сервер не знает, и теряется на
// валидации. Поэтому у каждой стороны есть тест, перечисляющий коды целиком
// (join_test.go::TestUniversityCodes и join.test.ts) — правка одной стороны
// обязана уронить её тест, а не всплыть на живом трафике.
//
// Порядок здесь не важен, его задаёт фронт; важен ровно набор кодов.
var Universities = map[string]string{
	"msu":       "МГУ",
	"hse":       "ВШЭ",
	"mgimo":     "МГИМО",
	"ranepa":    "РАНХиГС",
	"finu":      "Финансовый университет",
	"mipt":      "МФТИ",
	"bmstu":     "Бауманка",
	"plekhanov": "РЭУ им. Плеханова",
	"other":     "Другой",
}

// Courses — курс обучения. "graduate" = выпускник.
var Courses = map[string]string{
	"1": "1 курс", "2": "2 курс", "3": "3 курс",
	"4": "4 курс", "5": "5 курс", "6": "6 курс",
	"graduate": "выпускник",
}

const (
	maxName  = 80
	maxAbout = 140
	maxMeta  = 400 // потолок для utm/referrer/user-agent: хранить, не давиться
)

// Submission — то, что прислала форма. Метки кампании приезжают скрытыми
// полями со страницы: Метрика знает про клик, но не знает, кого мы взяли, —
// связать кампанию с принятым человеком может только эта строка.
type Submission struct {
	Name            string `json:"name"`
	University      string `json:"university"`
	UniversityOther string `json:"university_other"`
	Course          string `json:"course"`
	About           string `json:"about"`
	Telegram        string `json:"telegram"`
	Consent         bool   `json:"consent"`

	// HP — honeypot. Поле спрятано от человека; заполнено ⇒ это бот.
	//
	// Имя НЕ `website`/`url`/`company`: по таким именам поле заполняет и
	// автозаполнение Safari из карточки контакта, и менеджеры паролей —
	// `autocomplete="off"` они для contact-card не соблюдают. Ложное
	// срабатывание тут стоит оплаченного лида, поэтому имя намеренно
	// бессмысленное, а само срабатывание пишется в лог целиком.
	HP string `json:"hp_note"`

	UTMSource   string `json:"utm_source"`
	UTMMedium   string `json:"utm_medium"`
	UTMCampaign string `json:"utm_campaign"`
	UTMContent  string `json:"utm_content"`
	UTMTerm     string `json:"utm_term"`
	YClid       string `json:"yclid"`
	Referrer    string `json:"referrer"`
}

// Clean — проверенная анкета, готовая к записи. Отдельный тип, чтобы в
// репозиторий физически нельзя было передать непроверенный ввод.
type Clean struct {
	Name            string
	University      string
	UniversityOther string
	Course          string
	About           string
	Telegram        string // без '@', нижний регистр

	UTMSource   string
	UTMMedium   string
	UTMCampaign string
	UTMContent  string
	UTMTerm     string
	YClid       string
	Referrer    string
	UserAgent   string
	IP          string
}

// hashHint — первые символы хеша адреса, чтобы отказ можно было связать с
// конкретным отправителем в логе, не печатая ни адреса, ни юзернейма.
// Соль не нужна: это не проверка, а метка для глаз.
func (c Clean) hashHint() string {
	sum := sha256.Sum256([]byte(c.IP))
	return hex.EncodeToString(sum[:])[:8]
}

// TelegramDisplay — то, как юзернейм показывают человеку. Хранится он голым
// (см. миграцию 019), отображаемая форма собирается на чтении.
func TelegramDisplay(handle string) string {
	if handle == "" {
		return ""
	}
	return "@" + handle
}

// Validate проверяет анкету и возвращает КАРТУ ошибок по полям, а не первую
// найденную: человек должен увидеть все свои промахи разом, иначе форма
// заставляет отправлять её по кругу.
func Validate(in Submission) (Clean, map[string]string) {
	errs := map[string]string{}
	out := Clean{}

	out.Name = collapseSpaces(in.Name)
	switch {
	case out.Name == "":
		errs["name"] = "Как тебя зовут?"
	case utf8.RuneCountInString(out.Name) < 2:
		errs["name"] = "Слишком коротко"
	case utf8.RuneCountInString(out.Name) > maxName:
		errs["name"] = "Слишком длинно"
	}

	out.University = strings.TrimSpace(in.University)
	if _, ok := Universities[out.University]; !ok {
		errs["university"] = "Выбери вуз из списка"
	}
	if out.University == "other" {
		out.UniversityOther = collapseSpaces(in.UniversityOther)
		switch {
		case out.UniversityOther == "":
			errs["university_other"] = "Напиши, какой вуз"
		case utf8.RuneCountInString(out.UniversityOther) > maxName:
			errs["university_other"] = "Слишком длинно"
		}
	}

	out.Course = strings.TrimSpace(in.Course)
	if _, ok := Courses[out.Course]; !ok {
		errs["course"] = "Выбери курс"
	}

	out.About = collapseSpaces(in.About)
	switch {
	case out.About == "":
		errs["about"] = "Пара слов о том, чем занимаешься"
	case utf8.RuneCountInString(out.About) > maxAbout:
		errs["about"] = "Не больше 140 знаков"
	}

	// Юзернейм разбирает та же функция, что и веб-регистрация: люди вставляют
	// и '@ivanov', и 't.me/ivanov', и ссылку с хвостом. Своя копия правила
	// разошлась бы с ней на первой же вставке нового вида ссылки.
	if handle, ok := normalizeTelegram(in.Telegram); ok {
		out.Telegram = handle
	} else {
		errs["telegram"] = "Нужен юзернейм из Телеграма — например @ivanov"
	}

	if !in.Consent {
		errs["consent"] = "Без согласия отправить не получится"
	}

	// collapseSpaces, а не только clip: метки целиком в руках того, кто открыл
	// ссылку, и перевод строки внутри utm_campaign дорисовал бы лишнюю строку
	// в сообщение модератору («\nТелеграм: @чужой»). Подделки самого телеграма
	// тут нет (шлём без parse_mode) — подделывается текст для глаз.
	out.UTMSource = collapseSpaces(clip(in.UTMSource, maxMeta))
	out.UTMMedium = collapseSpaces(clip(in.UTMMedium, maxMeta))
	out.UTMCampaign = collapseSpaces(clip(in.UTMCampaign, maxMeta))
	out.UTMContent = collapseSpaces(clip(in.UTMContent, maxMeta))
	out.UTMTerm = collapseSpaces(clip(in.UTMTerm, maxMeta))
	out.YClid = collapseSpaces(clip(in.YClid, maxMeta))
	out.Referrer = collapseSpaces(clip(in.Referrer, maxMeta))

	if len(errs) > 0 {
		return Clean{}, errs
	}
	return out, nil
}

// NotifyText — сообщение в чат. Собирается в момент записи, когда все поля под
// рукой: отправщику (internal/events/notify.go) не нужны ни JOIN'ы, ни живая
// строка анкеты.
func NotifyText(c Clean) string {
	uni := Universities[c.University]
	if c.University == "other" && c.UniversityOther != "" {
		uni = c.UniversityOther
	}
	var b strings.Builder
	b.WriteString("Новая анкета\n")
	b.WriteString("Имя: " + c.Name + "\n")
	b.WriteString("Вуз: " + uni + ", " + Courses[c.Course] + "\n")
	b.WriteString("Занимается: " + c.About + "\n")
	b.WriteString("Телеграм: " + TelegramDisplay(c.Telegram))
	if c.UTMCampaign != "" {
		b.WriteString("\nКампания: " + c.UTMCampaign)
	}
	return b.String()
}

// clientIP — адрес посетителя. Фронт прокидывает его заголовком (см.
// frontend/src/routes/join/+page.server.ts): без этого сюда приезжает адрес
// SSR-контейнера, один на всех, и per-IP лимитер теряет смысл.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			xff = xff[:i]
		}
		return strings.TrimSpace(xff)
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		host = host[:i]
	}
	return strings.Trim(strings.TrimSpace(host), "[]")
}

func normalizeTelegram(raw string) (string, bool) {
	key, _, ok := webreg.NormalizeTG(raw)
	return key, ok
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	// Режем по границе руны, иначе в базу уедет битый UTF-8.
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
