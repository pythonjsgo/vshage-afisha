package events

import "testing"

func TestClassifyPublicContact(t *testing.T) {
	cases := []struct {
		in                     string
		email, phone, telegram string
	}{
		{"sasha@mail.ru", "sasha@mail.ru", "", ""},
		{"+79161234567", "", "+79161234567", ""},
		{"89161234567", "", "89161234567", ""},
		{"@suvorov", "", "", "suvorov"},
		{"suvorov", "", "", "suvorov"},
		{"t.me/suvorov", "", "", "suvorov"},
		{"https://t.me/suvorov", "", "", "suvorov"},
		// "@" without a dot after it is not an email — treat as telegram-ish text
		{"user@host", "", "", "user@host"},
		// short digit strings are not phones
		{"1234", "", "", "1234"},
	}
	for _, c := range cases {
		// production path normalizes first
		e, p, tg := classifyPublicContact(normalizePublicContact(c.in))
		if e != c.email || p != c.phone || tg != c.telegram {
			t.Errorf("classify(%q) = (%q,%q,%q), want (%q,%q,%q)", c.in, e, p, tg, c.email, c.phone, c.telegram)
		}
	}
}
