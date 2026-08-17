package webreg

import (
	"strings"
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
		Slug: "shag",
		// The pre-004 shape: name + telegram + вуз, all mandatory. These cases
		// are the regression guard for events created before the form became
		// configurable, so they must keep asserting exactly that behaviour.
		Form:         LegacyForm(),
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

// ── configurable form (004) ──────────────────────────────────────────────

// ticketEvent is the shape the admin editor produces by default: name and
// email mandatory, telegram asked for but optional, вуз not asked at all.
func ticketEvent() *Event {
	return &Event{Slug: "expo", Form: DefaultForm(), TicketMode: TicketQR}
}

func TestForm_DisabledFieldIsNotDemanded(t *testing.T) {
	in := &RegisterInput{
		Name:    "Саша",
		Email:   "sasha@example.com",
		Consent: true,
		// вуз is switched off for this event, and telegram is optional —
		// neither may block a signup.
	}
	out, apiErr := validateRegistration(ticketEvent(), in)
	if apiErr != nil {
		t.Fatalf("unexpected error: %+v", apiErr.Fields)
	}
	if out.Affiliation != "" {
		t.Fatalf("affiliation leaked through a disabled field: %q", out.Affiliation)
	}
}

func TestForm_DisabledFieldIsNotStored(t *testing.T) {
	// A visitor (or a crafted request) sending a value for a field the
	// organizer switched off must not have it silently persisted — that is
	// personal data collected without being asked for.
	in := &RegisterInput{
		Name: "Саша", Email: "sasha@example.com",
		Phone: "+79031234567", Affiliation: "МГУ", FullName: "Иванов Саша",
		Consent: true,
	}
	out, apiErr := validateRegistration(ticketEvent(), in)
	if apiErr != nil {
		t.Fatalf("unexpected error: %+v", apiErr.Fields)
	}
	if out.Phone != "" || out.Affiliation != "" || out.FullName != "" {
		t.Fatalf("disabled fields stored: phone=%q affiliation=%q full_name=%q",
			out.Phone, out.Affiliation, out.FullName)
	}
}

func TestForm_OptionalFieldStillValidatedWhenFilled(t *testing.T) {
	// Optional must mean "may be empty", never "may be garbage": a malformed
	// telegram accepted silently is an attendee the organizer cannot reach.
	in := &RegisterInput{
		Name: "Саша", Email: "sasha@example.com", TGUsername: "ваня", Consent: true,
	}
	_, apiErr := validateRegistration(ticketEvent(), in)
	if apiErr == nil || apiErr.Fields["tg_username"] == "" {
		t.Fatalf("expected an error on the optional-but-filled telegram, got %+v", apiErr)
	}
}

func TestForm_EmailRequiredCarriesTheReason(t *testing.T) {
	in := &RegisterInput{Name: "Саша", Consent: true}
	_, apiErr := validateRegistration(ticketEvent(), in)
	if apiErr == nil {
		t.Fatal("expected a missing-email error")
	}
	if msg := apiErr.Fields["email"]; !strings.Contains(msg, "билет") {
		t.Fatalf("message must say why the email is needed, got %q", msg)
	}
}

func TestNormalizeEmail(t *testing.T) {
	for _, s := range []string{"  Sasha@Example.COM ", "sasha@example.com"} {
		got, ok := NormalizeEmail(s)
		if !ok || got != "sasha@example.com" {
			t.Fatalf("NormalizeEmail(%q) = (%q, %v)", s, got, ok)
		}
	}
	for _, s := range []string{"", "sasha", "sasha@", "@example.com", "a b@c.ru", "sasha@example"} {
		if _, ok := NormalizeEmail(s); ok {
			t.Fatalf("NormalizeEmail(%q) accepted an unusable address", s)
		}
	}
}

func TestNormalizePhone_RussianSpellingsCollapse(t *testing.T) {
	// One person, four spellings — they must dedupe to one row.
	for _, s := range []string{"+7 903 123-45-67", "8 (903) 123 45 67", "79031234567", "9031234567"} {
		got, ok := NormalizePhone(s)
		if !ok || got != "+79031234567" {
			t.Fatalf("NormalizePhone(%q) = (%q, %v)", s, got, ok)
		}
	}
	if _, ok := NormalizePhone("12345"); ok {
		t.Fatal("a 5-digit string is not a phone number")
	}
}

func TestDedupKey_PrefersEmailOverTelegram(t *testing.T) {
	in := &RegisterInput{Email: "sasha@example.com", TGUsername: "sasha_23"}
	got, err := dedupKey(DefaultForm(), in)
	if err != nil {
		t.Fatal(err)
	}
	if got != "e:sasha@example.com" {
		t.Fatalf("dedup key = %q, want the email — a telegram username can be changed", got)
	}
}

func TestDedupKey_LegacyFormUsesTelegram(t *testing.T) {
	in := &RegisterInput{TGUsername: "sasha_23"}
	got, err := dedupKey(LegacyForm(), in)
	if err != nil {
		t.Fatal(err)
	}
	if got != "t:sasha_23" {
		t.Fatalf("dedup key = %q, want the telegram key", got)
	}
}

func TestDedupKey_NoIdentityIsUniquePerSubmission(t *testing.T) {
	// With nothing stable to key on, two signups must not collide into one
	// row — an occasional duplicate beats one person overwriting another.
	form := FormConfig{Version: 1, Name: FieldToggle{Enabled: true, Required: true}}
	a, err := dedupKey(form, &RegisterInput{Name: "Саша"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := dedupKey(form, &RegisterInput{Name: "Саша"})
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatalf("two anonymous signups share the dedup key %q", a)
	}
}

func TestNewTicketCode_AvoidsAmbiguousGlyphs(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		code, err := newTicketCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 7 {
			t.Fatalf("code %q is not 7 chars", code)
		}
		// These four are what turns a code read aloud at a door into the
		// wrong code typed in.
		if strings.ContainsAny(code, "IO01") {
			t.Fatalf("code %q contains an ambiguous glyph", code)
		}
		seen[code] = true
	}
	if len(seen) < 190 {
		t.Fatalf("only %d distinct codes in 200 draws — not random enough", len(seen))
	}
}
