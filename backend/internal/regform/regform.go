// Package regform is the configurable signup form for an afisha event.
//
// The board's own registration used to be two hardcoded fields, name and
// contact. That is enough for a bar, and not enough for a venue with a pass
// desk: РАНХиГС wants ФИО exactly as written in the passport, a date of birth
// and an answer to «нужен ли пропуск». Those questions differ per event, so
// they belong in the event's configuration rather than in the page's markup.
//
// The shapes here mirror internal/webreg, which grew the same feature first
// for its own /e/<slug> pages. They are copied rather than imported because
// webreg imports internal/events, and events needs this package — importing
// back would close a cycle. Keep the two in step: a change to validation
// rules here almost certainly belongs there too.
package regform

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	// Version is what a form configured through the panel carries. Zero means
	// «written before this feature existed», which is a different statement
	// from «configured with everything switched off» — see Decode.
	Version = 1

	maxNameLen     = 80
	maxFullNameLen = 120
	maxEmailLen    = 160
	maxContactLen  = 120
	maxAnswerLen   = 500
)

// FieldToggle is one built-in question: shown at all, and if shown, mandatory.
type FieldToggle struct {
	Enabled  bool `json:"enabled"`
	Required bool `json:"required"`
}

// FormConfig is the identity block of the form. Anything beyond it is an
// organizer-defined Field.
type FormConfig struct {
	Version  int         `json:"v"`
	Name     FieldToggle `json:"name"`
	FullName FieldToggle `json:"full_name"`
	Email    FieldToggle `json:"email"`
	Phone    FieldToggle `json:"phone"`
	TG       FieldToggle `json:"tg"`
	// Contact is the legacy free-form «телефон / email / telegram» box the
	// board has always had. Kept as a toggle so an event can go on using it.
	Contact FieldToggle `json:"contact"`
	// PassNote explains why a document name is being asked for. Asking for
	// passport data without saying why is the fastest way to lose a visitor.
	PassNote string `json:"pass_note,omitempty"`
}

// Field is one organizer-defined question.
type Field struct {
	Key        string   `json:"key"`
	Label      string   `json:"label"`
	Type       string   `json:"type"`
	Required   bool     `json:"required,omitempty"`
	Options    []string `json:"options,omitempty"`
	AllowOther bool     `json:"allow_other,omitempty"`
	Hint       string   `json:"hint,omitempty"`
	MaxLen     int      `json:"max_len,omitempty"`
}

// LegacyForm is the shape every board event had before this package: name and
// contact, both mandatory. Applied to events whose configuration predates the
// feature, so a live event's form does not change under the people filling it.
//
// It deliberately keeps Version at zero, i.e. IsLegacy() stays true. The
// version is not decoration: the caller branches on it to keep answering an
// unconfigured event with the old per-field error codes (invalid_name,
// invalid_contact) that the current page already understands. Stamping the
// current version here would make every untouched event take the new path and
// start returning a code its page has never seen.
func LegacyForm() FormConfig {
	on := FieldToggle{Enabled: true, Required: true}
	return FormConfig{Name: on, Contact: on}
}

// IsLegacy reports whether this configuration came from before the feature.
func (f FormConfig) IsLegacy() bool { return f.Version < Version }

// Decode reads the stored JSON. Anything unreadable, empty, or predating the
// feature decodes to the legacy form — never to «a form with no fields»,
// which would silently strip an event's registration down to nothing.
func Decode(raw []byte) FormConfig {
	if len(raw) == 0 {
		return LegacyForm()
	}
	var f FormConfig
	if err := json.Unmarshal(raw, &f); err != nil || f.IsLegacy() {
		return LegacyForm()
	}
	return f
}

// DecodeFields reads the organizer's own questions. Unreadable JSON yields no
// questions rather than an error: the identity block still lets people sign up.
func DecodeFields(raw []byte) []Field {
	if len(raw) == 0 {
		return nil
	}
	var out []Field
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

// Input is what the visitor submitted.
type Input struct {
	Name       string            `json:"name"`
	FullName   string            `json:"full_name"`
	Email      string            `json:"email"`
	Phone      string            `json:"phone"`
	TGUsername string            `json:"tg_username"`
	Contact    string            `json:"contact"`
	Answers    map[string]string `json:"answers"`
}

// Clean is the validated, normalized result.
type Clean struct {
	Name       string
	FullName   string
	Email      string
	Phone      string
	TGUsername string
	TGDisplay  string
	Contact    string
	Answers    map[string]string
}

// DisplayName is the name to show the organizer: the document name wins,
// because that is the one on the pass list at the door.
func (c Clean) DisplayName() string {
	if c.FullName != "" {
		return c.FullName
	}
	return c.Name
}

// DedupContact is the value that identifies this visitor across submissions.
// Email first — a person can change a Telegram handle, and a second signup
// would otherwise land as a second row on the door list. Falls back through
// the other collected fields so the caller always has a key.
func (c Clean) DedupContact() string {
	switch {
	case c.Email != "":
		return c.Email
	case c.Phone != "":
		return c.Phone
	case c.TGUsername != "":
		return "@" + c.TGUsername
	default:
		return c.Contact
	}
}

// ContactLine is every way to reach this person, for the organizer's notice.
func (c Clean) ContactLine() string {
	parts := make([]string, 0, 4)
	for _, v := range []string{c.Email, c.Phone} {
		if v != "" {
			parts = append(parts, v)
		}
	}
	if c.TGDisplay != "" {
		parts = append(parts, c.TGDisplay)
	}
	if c.Contact != "" {
		parts = append(parts, c.Contact)
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, " · ")
}

// Deliberately permissive: the only address worth rejecting is one that
// cannot possibly receive mail. Stricter hand-written patterns reject
// deliverable addresses, and each of those is a lost attendee.
var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]{2,}$`)

// NormalizeEmail mirrors webreg.NormalizeEmail.
func NormalizeEmail(raw string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" || utf8.RuneCountInString(s) > maxEmailLen || !emailRe.MatchString(s) {
		return "", false
	}
	return s, true
}

// NormalizePhone mirrors webreg.NormalizePhone: «8 903…», «+7 (903) …» and
// «903…» are three spellings of one number and must dedupe together.
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

var tgUsernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{5,32}$`)

var tgPrefixes = []string{
	"https://t.me/", "http://t.me/", "t.me/",
	"https://telegram.me/", "http://telegram.me/", "telegram.me/",
	"https://telegram.dog/", "telegram.dog/",
}

// NormalizeTG mirrors webreg.NormalizeTG: people paste «@name», «t.me/name»
// and a bare handle interchangeably. Returns the lowercase key and the
// display form.
func NormalizeTG(raw string) (key, display string, ok bool) {
	s := strings.TrimSpace(raw)
	for _, p := range tgPrefixes {
		if len(s) >= len(p) && strings.EqualFold(s[:len(p)], p) {
			s = s[len(p):]
			break
		}
	}
	s = strings.TrimPrefix(s, "@")
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if !tgUsernameRe.MatchString(s) {
		return "", "", false
	}
	return strings.ToLower(s), "@" + s, true
}

func collapseSpaces(s string) string { return strings.Join(strings.Fields(s), " ") }

// Validate checks the submission against the configuration. It returns the
// cleaned values and, when something is wrong, a per-field map of messages
// keyed by the form field name the page uses — so the page can put each
// message under the field it belongs to instead of one banner at the top.
func Validate(form FormConfig, fields []Field, in Input) (Clean, map[string]string) {
	errs := map[string]string{}
	out := Clean{Answers: map[string]string{}}

	if form.Name.Enabled {
		out.Name = collapseSpaces(in.Name)
		switch {
		case out.Name == "":
			if form.Name.Required {
				errs["name"] = "Как тебя зовут?"
			}
		case utf8.RuneCountInString(out.Name) > maxNameLen:
			errs["name"] = fmt.Sprintf("Слишком длинно — максимум %d символов", maxNameLen)
		}
	}

	if form.FullName.Enabled {
		out.FullName = collapseSpaces(in.FullName)
		switch {
		case out.FullName == "":
			if form.FullName.Required {
				errs["full_name"] = "Впиши ФИО полностью, как в документе"
			}
		case utf8.RuneCountInString(out.FullName) > maxFullNameLen:
			errs["full_name"] = "Слишком длинно"
		case len(strings.Fields(out.FullName)) < 2:
			// A pass list checked at a desk needs фамилия + имя at minimum;
			// one word gets the visitor turned away at the door.
			errs["full_name"] = "Нужны хотя бы фамилия и имя"
		}
	}

	if form.Email.Enabled {
		raw := strings.TrimSpace(in.Email)
		if raw == "" {
			if form.Email.Required {
				errs["email"] = "Нужна почта — на неё придёт напоминание"
			}
		} else if v, ok := NormalizeEmail(raw); ok {
			out.Email = v
		} else {
			errs["email"] = "Проверь адрес — похоже, в нём опечатка"
		}
	}

	if form.Phone.Enabled {
		raw := strings.TrimSpace(in.Phone)
		if raw == "" {
			if form.Phone.Required {
				errs["phone"] = "Нужен телефон"
			}
		} else if v, ok := NormalizePhone(raw); ok {
			out.Phone = v
		} else {
			errs["phone"] = "Проверь номер — например +7 903 123-45-67"
		}
	}

	if form.TG.Enabled {
		raw := strings.TrimSpace(in.TGUsername)
		if raw == "" {
			if form.TG.Required {
				errs["tg_username"] = "Нужен телеграм — по нему с тобой свяжутся"
			}
		} else if key, disp, ok := NormalizeTG(raw); ok {
			out.TGUsername, out.TGDisplay = key, disp
		} else {
			errs["tg_username"] = "Похоже на не-юзернейм. Он выглядит так: @username"
		}
	}

	if form.Contact.Enabled {
		out.Contact = collapseSpaces(in.Contact)
		switch {
		case out.Contact == "":
			if form.Contact.Required {
				errs["contact"] = "Укажите контакт для связи"
			}
		case utf8.RuneCountInString(out.Contact) < 5:
			errs["contact"] = "Укажите контакт для связи"
		case utf8.RuneCountInString(out.Contact) > maxContactLen:
			errs["contact"] = "Слишком длинно"
		}
	}

	for _, f := range fields {
		// Keys starting with __ are reserved for values the page passes
		// alongside the answers; an organizer must not be able to claim one.
		if f.Key == "" || strings.HasPrefix(f.Key, "__") {
			continue
		}
		raw := collapseSpaces(in.Answers[f.Key])
		if raw == "" {
			if f.Required {
				errs["answer:"+f.Key] = "Заполни это поле"
			}
			continue
		}
		limit := f.MaxLen
		if limit <= 0 || limit > maxAnswerLen {
			limit = maxAnswerLen
		}
		if utf8.RuneCountInString(raw) > limit {
			errs["answer:"+f.Key] = fmt.Sprintf("Максимум %d символов", limit)
			continue
		}
		if f.Type == "select" && !f.AllowOther && !containsFold(f.Options, raw) {
			errs["answer:"+f.Key] = "Выбери вариант из списка"
			continue
		}
		out.Answers[f.Key] = raw
	}

	// Nothing to reach the visitor by is not a valid signup: the organizer
	// would have a name on the list and no way to tell them anything.
	if len(errs) == 0 && out.DedupContact() == "" {
		errs["contact"] = "Оставь хотя бы один способ связи"
	}
	if len(errs) == 0 {
		return out, nil
	}
	return out, errs
}

func containsFold(list []string, v string) bool {
	for _, s := range list {
		if strings.EqualFold(strings.TrimSpace(s), v) {
			return true
		}
	}
	return false
}

// LabelFor returns the organizer's own wording for a question key, so the
// notice they receive reads «Дата рождения: …» rather than «birth_date: …».
func LabelFor(fields []Field, key string) string {
	for _, f := range fields {
		if f.Key == key && strings.TrimSpace(f.Label) != "" {
			return f.Label
		}
	}
	return key
}
