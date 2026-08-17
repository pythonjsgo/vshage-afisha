package webreg

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxNameLen        = 80
	maxFullNameLen    = 160
	maxEmailLen       = 254 // RFC 5321 addr-spec ceiling
	maxAffiliationLen = 120
	maxAnswerLen      = 500
	maxUALen          = 300
	maxSourceLen      = 80
)

// Deliberately permissive: the only thing worth rejecting here is an address
// that cannot possibly receive the ticket. Every stricter pattern people
// write by hand rejects addresses that are in fact valid and deliverable, and
// each of those is a lost attendee staring at an error they cannot fix.
var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]{2,}$`)

// NormalizeEmail lowercases and trims; the local part is case-sensitive per
// RFC but no mail provider people actually use treats it that way, and
// lowercasing is what makes the address usable as a dedup key.
func NormalizeEmail(raw string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" || utf8.RuneCountInString(s) > maxEmailLen || !emailRe.MatchString(s) {
		return "", false
	}
	return s, true
}

// NormalizePhone reduces a Russian number to +7XXXXXXXXXX so that «8 903…»,
// «+7 (903) …» and «903…» — three spellings of one phone — dedupe together.
// Anything that is not a 10/11-digit Russian number is kept as digits with a
// leading '+' when it looks international, rather than rejected: an organizer
// collecting phones abroad should not be blocked by our country's format.
func NormalizePhone(raw string) (string, bool) {
	var digits []rune
	for _, c := range raw {
		if c >= '0' && c <= '9' {
			digits = append(digits, c)
		}
	}
	s := string(digits)
	switch {
	case len(s) == 11 && (s[0] == '8' || s[0] == '7'):
		return "+7" + s[1:], true
	case len(s) == 10 && s[0] == '9':
		return "+7" + s, true
	case len(s) >= 8 && len(s) <= 15:
		return "+" + s, true
	default:
		return "", false
	}
}

// Telegram usernames are 5–32 chars of [A-Za-z0-9_]. We accept a leading '@',
// a full t.me/ or telegram.me/ URL, and any case — people paste all three.
var tgUsernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{5,32}$`)

var tgPrefixes = []string{
	"https://t.me/", "http://t.me/", "t.me/",
	"https://telegram.me/", "http://telegram.me/", "telegram.me/",
	"https://telegram.dog/", "telegram.dog/",
}

// NormalizeTG strips the shapes people actually paste and returns the bare
// lowercase username used as the dedup key, plus the display form (original
// casing, '@'-prefixed) for the organizer's list.
//
// Returns ok=false when the result is not a valid Telegram username; the
// caller turns that into a message that explains how to set one, because
// "невалидный юзернейм" with no explanation loses a real attendee.
func NormalizeTG(raw string) (key, display string, ok bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", false
	}
	// Trim a trailing slash / query so a copied profile URL works.
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	lower := strings.ToLower(s)
	for _, p := range tgPrefixes {
		if strings.HasPrefix(lower, p) {
			s = s[len(p):]
			break
		}
	}
	s = strings.TrimSpace(strings.Trim(s, "/"))
	s = strings.TrimPrefix(s, "@")
	s = strings.TrimSpace(s)

	if !tgUsernameRe.MatchString(s) {
		return "", "", false
	}
	return strings.ToLower(s), "@" + s, true
}

// validateRegistration checks the payload against the event's field config and
// returns a per-field error map so the form can highlight exactly what to fix.
//
// Every built-in field is now conditional on ev.Form: a field the organizer
// switched off is not validated and not stored, and an optional field that
// came back empty passes. A field that arrives filled in is still validated
// even when optional — a malformed email silently accepted is a ticket that
// silently never arrives.
func validateRegistration(ev *Event, in *RegisterInput) (*RegisterInput, *APIError) {
	fieldErrs := map[string]string{}
	out := &RegisterInput{Answers: map[string]string{}}
	form := ev.Form

	if form.Name.Enabled {
		out.Name = collapseSpaces(in.Name)
		switch {
		case out.Name == "":
			if form.Name.Required {
				fieldErrs["name"] = "Как тебя зовут?"
			}
		case utf8.RuneCountInString(out.Name) > maxNameLen:
			fieldErrs["name"] = fmt.Sprintf("Слишком длинно — максимум %d символов", maxNameLen)
		}
	}

	if form.FullName.Enabled {
		out.FullName = collapseSpaces(in.FullName)
		switch {
		case out.FullName == "":
			if form.FullName.Required {
				fieldErrs["full_name"] = "Впиши ФИО полностью, как в документе"
			}
		case utf8.RuneCountInString(out.FullName) > maxFullNameLen:
			fieldErrs["full_name"] = "Слишком длинно"
		case len(strings.Fields(out.FullName)) < 2:
			// A pass list checked at a desk needs at least фамилия + имя;
			// one word gets the visitor turned away at the door.
			fieldErrs["full_name"] = "Нужны хотя бы фамилия и имя"
		}
	}

	if form.Email.Enabled {
		raw := strings.TrimSpace(in.Email)
		if raw == "" {
			if form.Email.Required {
				fieldErrs["email"] = "Нужна почта — на неё придёт билет"
			}
		} else if email, ok := NormalizeEmail(raw); ok {
			out.Email = email
		} else {
			fieldErrs["email"] = "Проверь адрес — похоже, в нём опечатка"
		}
	}

	if form.Phone.Enabled {
		raw := strings.TrimSpace(in.Phone)
		if raw == "" {
			if form.Phone.Required {
				fieldErrs["phone"] = "Нужен телефон"
			}
		} else if phone, ok := NormalizePhone(raw); ok {
			out.Phone = phone
		} else {
			fieldErrs["phone"] = "Проверь номер — например +7 903 123-45-67"
		}
	}

	var tgDisplay string
	if form.TG.Enabled {
		raw := strings.TrimSpace(in.TGUsername)
		if raw == "" {
			if form.TG.Required {
				fieldErrs["tg_username"] = "Нужен юзернейм из Телеграма — например @ivanov. " +
					"Если его нет: Телеграм → Настройки → Имя пользователя"
			}
		} else if key, display, ok := NormalizeTG(raw); ok {
			out.TGUsername, tgDisplay = key, display
		} else {
			fieldErrs["tg_username"] = "Нужен юзернейм из Телеграма — например @ivanov. " +
				"Если его нет: Телеграм → Настройки → Имя пользователя"
		}
	}

	if form.Affiliation.Enabled {
		out.Affiliation = collapseSpaces(in.Affiliation)
		switch {
		case out.Affiliation == "":
			if form.Affiliation.Required {
				fieldErrs["affiliation"] = "Выбери вуз или статус"
			}
		case utf8.RuneCountInString(out.Affiliation) > maxAffiliationLen:
			fieldErrs["affiliation"] = "Слишком длинно"
		}
	}

	if !in.Consent {
		fieldErrs["consent"] = "Без согласия на обработку данных не сможем записать"
	}

	for _, f := range ev.Fields {
		raw := collapseSpaces(in.Answers[f.Key])
		limit := f.MaxLen
		if limit <= 0 || limit > maxAnswerLen {
			limit = maxAnswerLen
		}
		if utf8.RuneCountInString(raw) > limit {
			fieldErrs[f.Key] = fmt.Sprintf("Слишком длинно — максимум %d символов", limit)
			continue
		}
		if f.Type == "select" && raw != "" && !f.AllowOther && !containsFold(f.Options, raw) {
			fieldErrs[f.Key] = "Выбери вариант из списка"
			continue
		}
		if f.Required && raw == "" {
			fieldErrs[f.Key] = "Заполни это поле"
			continue
		}
		if raw != "" {
			out.Answers[f.Key] = raw
		}
	}

	if len(fieldErrs) > 0 {
		return nil, &APIError{
			Status:  http.StatusBadRequest,
			Code:    "validation_failed",
			Message: "Проверь отмеченные поля",
			Fields:  fieldErrs,
		}
	}

	out.Consent = true
	out.Source = truncate(collapseSpaces(in.Source), maxSourceLen)
	if tgDisplay != "" {
		out.Answers["__tg_display"] = tgDisplay
	}
	return out, nil
}

// dedupKey is what makes a double tap on a flaky connection land as one row.
// It follows the event's identity field, and falls back to a per-submission
// random value when the visitor gave nothing stable — better an occasional
// duplicate than an insert that collides with an unrelated person under a
// shared empty key.
func dedupKey(form FormConfig, in *RegisterInput) (string, error) {
	switch form.identity() {
	case "email":
		if in.Email != "" {
			return "e:" + in.Email, nil
		}
	case "tg":
		if in.TGUsername != "" {
			return "t:" + in.TGUsername, nil
		}
	case "phone":
		if in.Phone != "" {
			return "p:" + in.Phone, nil
		}
	}
	// Second chance on whatever else the visitor did give, in the same order
	// of stability, before giving up and going random.
	switch {
	case in.Email != "":
		return "e:" + in.Email, nil
	case in.TGUsername != "":
		return "t:" + in.TGUsername, nil
	case in.Phone != "":
		return "p:" + in.Phone, nil
	}
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("dedup key: %w", err)
	}
	return "x:" + hex.EncodeToString(buf), nil
}

// newManageKey mints an organizer's secret link segment. 128 bits, because
// this single value is the whole access control on an attendee list.
func newManageKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("manage key: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// ticketAlphabet drops I, O, 0, 1 — a code is read aloud at a door and
// copied off a phone screen, and those four are where that goes wrong.
const ticketAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// newTicketCode returns a 7-character entry code. 32^7 ≈ 3.4e10 keeps
// guessing pointless at event scale, and the uniqueness index is what
// actually guarantees no two holders share one.
func newTicketCode() (string, error) {
	buf := make([]byte, 7)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("ticket code: %w", err)
	}
	out := make([]byte, len(buf))
	for i, b := range buf {
		out[i] = ticketAlphabet[int(b)%len(ticketAlphabet)]
	}
	return string(out), nil
}

func containsFold(opts []string, v string) bool {
	for _, o := range opts {
		if strings.EqualFold(strings.TrimSpace(o), v) {
			return true
		}
	}
	return false
}

// collapseSpaces trims and squeezes internal whitespace runs (including the
// newlines a mobile keyboard's paste can smuggle in) into single spaces.
func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}
