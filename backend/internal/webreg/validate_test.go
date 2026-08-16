package webreg

import (
	"testing"
	"time"
)

func TestNormalizeTG(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		wantKey     string
		wantDisplay string
		wantOK      bool
	}{
		// The five shapes people actually paste into the field.
		{"bare", "ivanov23", "ivanov23", "@ivanov23", true},
		{"at prefix", "@Ivanov23", "ivanov23", "@Ivanov23", true},
		{"t.me link", "t.me/Ivanov23", "ivanov23", "@Ivanov23", true},
		{"https link", "https://t.me/ivanov23", "ivanov23", "@ivanov23", true},
		{"link with query", "https://t.me/ivanov23?start=x", "ivanov23", "@ivanov23", true},
		{"telegram.me", "https://telegram.me/ivanov23", "ivanov23", "@ivanov23", true},
		{"trailing slash", "t.me/ivanov23/", "ivanov23", "@ivanov23", true},
		{"padded", "  @ivanov23  ", "ivanov23", "@ivanov23", true},

		// Case only affects the dedup key, never the display form: the
		// organizer sees what the visitor typed.
		{"mixed case keeps display", "@IvanOV_23", "ivanov_23", "@IvanOV_23", true},

		{"empty", "", "", "", false},
		{"too short", "ivan", "", "", false},
		{"too long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "", "", false},
		{"cyrillic", "иванов23", "", "", false},
		{"spaces inside", "iva nov23", "", "", false},
		{"phone number", "+79161234567", "", "", false},
		{"email", "ivan@mail.ru", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, display, ok := NormalizeTG(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("NormalizeTG(%q) ok = %v, want %v", tc.in, ok, tc.wantOK)
			}
			if key != tc.wantKey || display != tc.wantDisplay {
				t.Fatalf("NormalizeTG(%q) = (%q, %q), want (%q, %q)",
					tc.in, key, display, tc.wantKey, tc.wantDisplay)
			}
		})
	}
}

// The dedup key is what makes a double tap idempotent instead of a duplicate
// row. If these ever diverge, the same person lands in the list twice.
func TestNormalizeTG_DedupKeyIsStableAcrossShapes(t *testing.T) {
	shapes := []string{"ivanov23", "@ivanov23", "@Ivanov23", "t.me/IVANOV23", "https://t.me/ivanov23/"}
	want := "ivanov23"
	for _, s := range shapes {
		key, _, ok := NormalizeTG(s)
		if !ok || key != want {
			t.Fatalf("NormalizeTG(%q) = (%q, ok=%v), want key %q", s, key, ok, want)
		}
	}
}

func testEvent() *Event {
	return &Event{
		Slug:         "shag",
		Affiliations: []string{"НИУ ВШЭ", "МГУ"},
		Fields: []Field{
			{Key: "source", Label: "Откуда узнал", Type: "select",
				Options: []string{"Телеграм", "От друзей"}, Required: true},
			{Key: "note", Label: "Комментарий", Type: "text", MaxLen: 10},
		},
	}
}

func TestValidateRegistration_Valid(t *testing.T) {
	in := &RegisterInput{
		Name:        "  Саша   Иванов ",
		TGUsername:  "@Sasha_23",
		Affiliation: "НИУ ВШЭ",
		Answers:     map[string]string{"source": "Телеграм", "note": "привет"},
		Consent:     true,
	}
	out, apiErr := validateRegistration(testEvent(), in)
	if apiErr != nil {
		t.Fatalf("unexpected error: %+v", apiErr)
	}
	if out.Name != "Саша Иванов" {
		t.Fatalf("name not collapsed: %q", out.Name)
	}
	if out.TGUsername != "sasha_23" {
		t.Fatalf("tg key = %q", out.TGUsername)
	}
	if out.Answers["__tg_display"] != "@Sasha_23" {
		t.Fatalf("display = %q", out.Answers["__tg_display"])
	}
	if out.Answers["source"] != "Телеграм" {
		t.Fatalf("answers not carried: %+v", out.Answers)
	}
}

func TestValidateRegistration_Errors(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(*RegisterInput)
		wantField string
	}{
		{"no name", func(in *RegisterInput) { in.Name = "  " }, "name"},
		{"bad tg", func(in *RegisterInput) { in.TGUsername = "ваня" }, "tg_username"},
		{"no affiliation", func(in *RegisterInput) { in.Affiliation = "" }, "affiliation"},
		{"no consent", func(in *RegisterInput) { in.Consent = false }, "consent"},
		{"required custom field missing", func(in *RegisterInput) { delete(in.Answers, "source") }, "source"},
		{"select value off-list", func(in *RegisterInput) { in.Answers["source"] = "Радио" }, "source"},
		{"custom field too long", func(in *RegisterInput) { in.Answers["note"] = "12345678901234567890" }, "note"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := &RegisterInput{
				Name:        "Саша",
				TGUsername:  "@sasha_23",
				Affiliation: "МГУ",
				Answers:     map[string]string{"source": "Телеграм"},
				Consent:     true,
			}
			tc.mutate(in)

			_, apiErr := validateRegistration(testEvent(), in)
			if apiErr == nil {
				t.Fatal("expected a validation error, got none")
			}
			if _, ok := apiErr.Fields[tc.wantField]; !ok {
				t.Fatalf("expected error on field %q, got %+v", tc.wantField, apiErr.Fields)
			}
			// Every message is shown to a visitor verbatim, so none may be empty.
			for f, msg := range apiErr.Fields {
				if msg == "" {
					t.Fatalf("empty message for field %q", f)
				}
			}
		})
	}
}

// allow_other exists so an organizer can offer «Другое» with a free-text
// answer; the off-list check must not fire then.
func TestValidateRegistration_AllowOtherAcceptsFreeText(t *testing.T) {
	ev := testEvent()
	ev.Fields[0].AllowOther = true

	in := &RegisterInput{
		Name: "Саша", TGUsername: "@sasha_23", Affiliation: "МГУ",
		Answers: map[string]string{"source": "увидел плакат в столовой"},
		Consent: true,
	}
	if _, apiErr := validateRegistration(ev, in); apiErr != nil {
		t.Fatalf("unexpected error: %+v", apiErr)
	}
}

func TestValidSlug(t *testing.T) {
	ok := []string{"shag", "shag-2026", "a1", "shag-open-day-2026"}
	bad := []string{"", "a", "ШАГ", "shag 2026", "Shag", "shag_2026", "shag/2026"}
	for _, s := range ok {
		if !validSlug(s) {
			t.Errorf("validSlug(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if validSlug(s) {
			t.Errorf("validSlug(%q) = true, want false", s)
		}
	}
}

func TestValidateUpsert_RejectsReservedFieldKey(t *testing.T) {
	in := &EventUpsert{
		Slug: "shag", Title: "ШАГ",
		StartsAt: nowForTest(),
		Fields:   []Field{{Key: "__tg_display", Label: "x", Type: "text"}},
	}
	if err := validateUpsert(in); err == nil {
		t.Fatal("expected __-prefixed field key to be rejected")
	}
}

func TestValidateUpsert_RejectsUnknownTimezone(t *testing.T) {
	in := &EventUpsert{Slug: "shag", Title: "ШАГ", StartsAt: nowForTest(), Timezone: "Mars/Olympus"}
	if err := validateUpsert(in); err == nil {
		t.Fatal("expected unknown timezone to be rejected")
	}
}

func TestHashKeyIsNotIdentity(t *testing.T) {
	key := "s3cret-manage-key"
	h := HashKey(key)
	if h == key || len(h) != 64 {
		t.Fatalf("HashKey(%q) = %q — expected a 64-char sha256 hex digest", key, h)
	}
	if HashKey(key) != h {
		t.Fatal("HashKey is not deterministic")
	}
}

// nowForTest keeps upsert fixtures readable; any non-zero time works.
func nowForTest() time.Time { return time.Date(2026, 8, 18, 19, 0, 0, 0, time.UTC) }
