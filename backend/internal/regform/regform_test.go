package regform

import (
	"testing"
)

// The rule this file exists for: an event that was configured before the
// feature — or not configured at all — must keep the exact two-field form it
// has today. A live event's form changing shape under the people filling it in
// is the failure mode that matters, and it is invisible until someone signs up.
func TestUnconfiguredEventKeepsTheOldTwoFieldForm(t *testing.T) {
	for _, raw := range []string{"", "{}", "null", "not json at all", `{"v":0}`} {
		f := Decode([]byte(raw))
		if !f.Name.Enabled || !f.Name.Required {
			t.Fatalf("%q: name must stay on and mandatory, got %+v", raw, f.Name)
		}
		if !f.Contact.Enabled || !f.Contact.Required {
			t.Fatalf("%q: contact must stay on and mandatory, got %+v", raw, f.Contact)
		}
		if f.FullName.Enabled || f.Email.Enabled || f.Phone.Enabled || f.TG.Enabled {
			t.Fatalf("%q: unconfigured form must not grow new fields: %+v", raw, f)
		}
		if !f.IsLegacy() {
			t.Fatalf("%q: must report itself as legacy", raw)
		}
	}
}

func TestConfiguredFormIsTakenLiterally(t *testing.T) {
	f := Decode([]byte(`{"v":1,
		"name":{"enabled":false,"required":false},
		"full_name":{"enabled":true,"required":true},
		"email":{"enabled":true,"required":true},
		"phone":{"enabled":true,"required":false},
		"tg":{"enabled":true,"required":false},
		"contact":{"enabled":false,"required":false},
		"pass_note":"нужно для пропуска"}`))
	if f.IsLegacy() {
		t.Fatal("v=1 must not be treated as legacy")
	}
	if f.Name.Enabled {
		t.Error("name was switched off and must stay off")
	}
	if !f.FullName.Required || !f.Email.Required {
		t.Error("full_name and email were marked required")
	}
	if f.Phone.Required || f.TG.Required {
		t.Error("phone and tg were marked optional")
	}
	if f.PassNote == "" {
		t.Error("pass_note lost")
	}
}

func passForm() FormConfig {
	return FormConfig{
		Version:  Version,
		FullName: FieldToggle{Enabled: true, Required: true},
		Email:    FieldToggle{Enabled: true, Required: true},
		Phone:    FieldToggle{Enabled: true},
		TG:       FieldToggle{Enabled: true},
	}
}

func passFields() []Field {
	return []Field{
		{Key: "birth_date", Label: "Дата рождения", Type: "text", Required: true, MaxLen: 10},
		{Key: "needs_pass", Label: "Нужен пропуск в здание", Type: "checkbox"},
	}
}

func TestPassportFormAcceptsAGoodSubmission(t *testing.T) {
	clean, errs := Validate(passForm(), passFields(), Input{
		FullName:   "  Иванов   Иван Иванович ",
		Email:      " Ivan@Example.COM ",
		Phone:      "8 (903) 123-45-67",
		TGUsername: "https://t.me/Ivanov",
		Answers:    map[string]string{"birth_date": "01.01.1990", "needs_pass": "Да"},
	})
	if errs != nil {
		t.Fatalf("valid submission rejected: %v", errs)
	}
	if clean.FullName != "Иванов Иван Иванович" {
		t.Errorf("full name not collapsed: %q", clean.FullName)
	}
	if clean.Email != "ivan@example.com" {
		t.Errorf("email not normalized: %q", clean.Email)
	}
	if clean.Phone != "+79031234567" {
		t.Errorf("phone not normalized: %q", clean.Phone)
	}
	if clean.TGUsername != "ivanov" || clean.TGDisplay != "@Ivanov" {
		t.Errorf("tg not normalized: %q / %q", clean.TGUsername, clean.TGDisplay)
	}
	if clean.DisplayName() != "Иванов Иван Иванович" {
		t.Errorf("document name must be the one shown to the organizer, got %q", clean.DisplayName())
	}
	if clean.DedupContact() != "ivan@example.com" {
		t.Errorf("email must be the dedup key, got %q", clean.DedupContact())
	}
	if clean.Answers["birth_date"] != "01.01.1990" || clean.Answers["needs_pass"] != "Да" {
		t.Errorf("answers lost: %v", clean.Answers)
	}
}

func TestEveryMissingMandatoryFieldGetsItsOwnMessage(t *testing.T) {
	_, errs := Validate(passForm(), passFields(), Input{})
	if errs == nil {
		t.Fatal("empty submission accepted")
	}
	for _, key := range []string{"full_name", "email", "answer:birth_date"} {
		if errs[key] == "" {
			t.Errorf("no message for %q; got %v", key, errs)
		}
	}
	// Optional fields must not produce noise.
	for _, key := range []string{"phone", "tg_username", "answer:needs_pass"} {
		if errs[key] != "" {
			t.Errorf("optional %q must not error, got %q", key, errs[key])
		}
	}
}

// A single word cannot be checked against a document at a pass desk.
func TestOneWordFullNameIsRejected(t *testing.T) {
	_, errs := Validate(passForm(), nil, Input{FullName: "Иван", Email: "a@b.co"})
	if errs["full_name"] == "" {
		t.Fatalf("one-word name accepted: %v", errs)
	}
}

func TestBadEmailAndPhoneAreReported(t *testing.T) {
	_, errs := Validate(passForm(), nil, Input{
		FullName: "Иванов Иван", Email: "ivan@@example", Phone: "12",
	})
	if errs["email"] == "" {
		t.Error("malformed email accepted")
	}
	if errs["phone"] == "" {
		t.Error("malformed phone accepted")
	}
}

func TestSelectRejectsValuesOutsideTheList(t *testing.T) {
	fields := []Field{{Key: "src", Label: "Откуда", Type: "select", Options: []string{"Телеграм", "От друзей"}}}
	form := passForm()
	base := Input{FullName: "Иванов Иван", Email: "a@b.co"}

	base.Answers = map[string]string{"src": "Из газеты"}
	if _, errs := Validate(form, fields, base); errs["answer:src"] == "" {
		t.Error("value outside options accepted")
	}
	base.Answers = map[string]string{"src": "от друзей"}
	if _, errs := Validate(form, fields, base); errs != nil {
		t.Errorf("case-insensitive match rejected: %v", errs)
	}
	fields[0].AllowOther = true
	base.Answers = map[string]string{"src": "Из газеты"}
	if _, errs := Validate(form, fields, base); errs != nil {
		t.Errorf("allow_other must accept a free answer: %v", errs)
	}
}

func TestAnswerLengthIsCapped(t *testing.T) {
	fields := []Field{{Key: "note", Label: "Заметка", Type: "text", MaxLen: 5}}
	_, errs := Validate(passForm(), fields, Input{
		FullName: "Иванов Иван", Email: "a@b.co",
		Answers: map[string]string{"note": "слишком длинная строка"},
	})
	if errs["answer:note"] == "" {
		t.Fatal("over-long answer accepted")
	}
}

// An organizer must not be able to claim a reserved key and have it stored.
func TestReservedKeysAreIgnored(t *testing.T) {
	fields := []Field{{Key: "__tg_display", Label: "x", Type: "text", Required: true}}
	clean, errs := Validate(passForm(), fields, Input{
		FullName: "Иванов Иван", Email: "a@b.co",
		Answers: map[string]string{"__tg_display": "подделка"},
	})
	if errs != nil {
		t.Fatalf("reserved key must be skipped, not fail the form: %v", errs)
	}
	if _, ok := clean.Answers["__tg_display"]; ok {
		t.Error("reserved key stored")
	}
}

// Without any way to reach the visitor the organizer has a name and nothing else.
func TestSignupWithNoContactAtAllIsRejected(t *testing.T) {
	form := FormConfig{Version: Version, Name: FieldToggle{Enabled: true}}
	_, errs := Validate(form, nil, Input{Name: "Иван"})
	if errs == nil {
		t.Fatal("signup with no contact accepted")
	}
}

func TestContactLineListsEveryWayToReachThem(t *testing.T) {
	clean, errs := Validate(passForm(), nil, Input{
		FullName: "Иванов Иван", Email: "a@b.co", Phone: "+79031234567", TGUsername: "@ivanov",
	})
	if errs != nil {
		t.Fatalf("unexpected errors: %v", errs)
	}
	want := "a@b.co · +79031234567 · @ivanov"
	if got := clean.ContactLine(); got != want {
		t.Errorf("contact line\n got %q\nwant %q", got, want)
	}
}

func TestLabelForFallsBackToTheKey(t *testing.T) {
	fields := passFields()
	if got := LabelFor(fields, "birth_date"); got != "Дата рождения" {
		t.Errorf("label lost: %q", got)
	}
	if got := LabelFor(fields, "unknown"); got != "unknown" {
		t.Errorf("unknown key must fall back to itself, got %q", got)
	}
}
