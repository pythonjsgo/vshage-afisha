package tgevents

import (
	"strings"
	"testing"
)

func validCard() Card {
	end := "2026-09-02"
	src := "https://t.me/some_channel/123"
	return Card{
		ID:          "ev_deadbeef0001",
		Title:       "Лекция о городе",
		Annonce:     "Наш текст о событии. Два предложения, как велит контракт.",
		Date:        "2026-09-01",
		DateEnd:     &end,
		AccessLevel: "open",
		SourceURL:   &src,
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
		// Формат id решает, в какой бэкенд пойдёт фронт за карточкой: не тот
		// формат — переход из ленты даёт 404 по одной карточке и невидимо.
		{"id не вида ev_<hex>", func(c *Card) { c.ID = "ev_2026_ab12" }, "id"},
		{"id слишком короткий", func(c *Card) { c.ID = "ev_ab1" }, "id"},
		{"пустой title", func(c *Card) { c.Title = "" }, "title"},
		{"пустой annonce — витрина показала бы чужой текст",
			func(c *Card) { c.Annonce = " " }, "annonce"},
		{"дата не ISO", func(c *Card) { c.Date = "01.09.2026" }, "date"},
		{"date_end не ISO", func(c *Card) { bad := "скоро"; c.DateEnd = &bad }, "date_end"},
		{"access вне словаря", func(c *Card) { c.AccessLevel = "vip" }, "access_level"},
		{"registration_url не http(s) — уйдёт в href",
			func(c *Card) { bad := "javascript:alert(1)"; c.RegistrationURL = &bad }, "registration_url"},
		{"source_url не http(s)", func(c *Card) { bad := "tg://resolve?x"; c.SourceURL = &bad }, "source_url"},
		// Без первоисточника показывать чужой анонс нельзя (юр-рамка), и фронт
		// по этому же полю понимает, что событие не наше.
		{"source_url отсутствует", func(c *Card) { c.SourceURL = nil }, "source_url"},
		{"source_url пустой", func(c *Card) { e := "   "; c.SourceURL = &e }, "source_url"},
		// «19.00» и «весь день» молча стали бы полуночью — то есть неотличимы
		// от «времени нет», и различить это после записи уже невозможно.
		{"time_start не HH:MM", func(c *Card) { bad := "19.00"; c.TimeStart = &bad }, "time_start"},
		{"time_start диапазоном", func(c *Card) { bad := "19:00-21:00"; c.TimeStart = &bad }, "time_start"},
		// Категория едет в ранжировщик ленты, и неизвестный код он свернёт в
		// `other` молча — опечатка конвейера проявилась бы как «почему-то всё
		// в прочем», через неделю и без следа в логах.
		{"category вне словаря ленты",
			func(c *Card) { bad := "koncert"; c.Category = &bad }, "category"},
		{"category значением сегмента, а не кодом словаря",
			func(c *Card) { bad := "culture"; c.Category = &bad }, "category"},
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

// TestValidateAcceptsEveryFeedCategory — словарь категорий продублирован из
// ленты вручную (общей библиотеки между сервисами нет), поэтому он обязан
// проверяться перечислением: пропущенный при пополнении код отклонял бы
// заливку целой рубрики, и увидели бы мы это ночным прогоном, а не здесь.
func TestValidateAcceptsEveryFeedCategory(t *testing.T) {
	for _, code := range []string{
		"concert", "party", "lecture", "workshop", "exhibition", "market",
		"sport", "theatre_cinema", "networking", "excursion", "campus",
		"family", "dating", "nightlife", "spiritual", "health", "other",
	} {
		c := validCard()
		c.Category = &code
		if err := c.Validate(); err != nil {
			t.Errorf("код словаря %q отклонён: %v", code, err)
		}
	}
	if len(feedCategories) != 17 {
		t.Errorf("в словаре %d кодов, в ленте 0.7 их 17 — списки разъехались",
			len(feedCategories))
	}
}

// TestCategoryIsOptional — старый конвейер категорию не шлёт, и это законно:
// читатель выводит её из segment по-прежнему. Отказ здесь остановил бы
// заливку студенческих карточек, которые ездят с 02.09.
func TestCategoryIsOptional(t *testing.T) {
	c := validCard()
	if c.Category != nil {
		t.Fatal("образец карточки не должен нести категорию")
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("карточка без категории отклонена: %v", err)
	}
	empty := ""
	c.Category = &empty
	if err := c.Validate(); err != nil {
		t.Fatalf("пустая категория отклонена, а это то же «не прислали»: %v", err)
	}
}
