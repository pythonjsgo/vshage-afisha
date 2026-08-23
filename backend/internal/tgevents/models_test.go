package tgevents

import (
	"strings"
	"testing"
)

func validCard() Card {
	end := "2026-09-02"
	return Card{
		ID:          "ev_deadbeef0001",
		Title:       "Лекция о городе",
		Annonce:     "Наш текст о событии. Два предложения, как велит контракт.",
		Date:        "2026-09-01",
		DateEnd:     &end,
		AccessLevel: "open",
	}
}

func TestValidateAccepts(t *testing.T) {
	c := validCard()
	if err := c.Validate(); err != nil {
		t.Fatalf("валидная карточка отклонена: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Card)
		want   string // подстрока текста ошибки — читатель лога должен понять, что чинить
	}{
		{"пустой id", func(c *Card) { c.ID = "  " }, "id"},
		{"пустой title", func(c *Card) { c.Title = "" }, "title"},
		{"пустой annonce — витрина показала бы чужой текст",
			func(c *Card) { c.Annonce = " " }, "annonce"},
		{"дата не ISO", func(c *Card) { c.Date = "01.09.2026" }, "date"},
		{"date_end не ISO", func(c *Card) { bad := "скоро"; c.DateEnd = &bad }, "date_end"},
		{"access вне словаря", func(c *Card) { c.AccessLevel = "vip" }, "access_level"},
		{"registration_url не http(s) — уйдёт в href",
			func(c *Card) { bad := "javascript:alert(1)"; c.RegistrationURL = &bad }, "registration_url"},
		{"source_url не http(s)", func(c *Card) { bad := "tg://resolve?x"; c.SourceURL = &bad }, "source_url"},
		{"обложка без mime", func(c *Card) { b := "aGkh"; c.CoverB64 = &b }, "cover_mime"},
		{"обложка с не-картиночным mime",
			func(c *Card) { b, m := "aGkh", "text/html"; c.CoverB64, c.CoverMime = &b, &m }, "cover_mime"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validCard()
			tc.mutate(&c)
			err := c.Validate()
			if err == nil {
				t.Fatal("брак принят без ошибки")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ошибка %q не называет поле %q", err, tc.want)
			}
		})
	}
}
