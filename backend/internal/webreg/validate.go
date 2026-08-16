package webreg

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxNameLen        = 80
	maxAffiliationLen = 120
	maxAnswerLen      = 500
	maxUALen          = 300
	maxSourceLen      = 80
)

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
func validateRegistration(ev *Event, in *RegisterInput) (*RegisterInput, *APIError) {
	fieldErrs := map[string]string{}
	out := &RegisterInput{Answers: map[string]string{}}

	out.Name = collapseSpaces(in.Name)
	switch {
	case out.Name == "":
		fieldErrs["name"] = "Как тебя зовут?"
	case utf8.RuneCountInString(out.Name) > maxNameLen:
		fieldErrs["name"] = fmt.Sprintf("Слишком длинно — максимум %d символов", maxNameLen)
	}

	key, display, ok := NormalizeTG(in.TGUsername)
	if !ok {
		fieldErrs["tg_username"] = "Нужен юзернейм из Телеграма — например @ivanov. " +
			"Если его нет: Телеграм → Настройки → Имя пользователя"
	}
	out.TGUsername = key
	// Display form travels in Answers-free fields; the repository reads both.
	tgDisplay := display

	out.Affiliation = collapseSpaces(in.Affiliation)
	switch {
	case out.Affiliation == "":
		fieldErrs["affiliation"] = "Выбери вуз или статус"
	case utf8.RuneCountInString(out.Affiliation) > maxAffiliationLen:
		fieldErrs["affiliation"] = "Слишком длинно"
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
	out.Answers["__tg_display"] = tgDisplay
	return out, nil
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
