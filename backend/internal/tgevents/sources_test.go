package tgevents

import (
	"encoding/base64"
	"strings"
	"testing"
)

func validSource() Source {
	u := "https://t.me/ranepasport"
	return Source{
		Key: "tg:ranepasport", Platform: "tg", Handle: "ranepasport",
		Title: "РАНХиГС — спорт", URL: &u, Enabled: true,
	}
}

func TestSourceValidateAccepts(t *testing.T) {
	s := validSource()
	if err := s.Validate(); err != nil {
		t.Fatalf("валидный источник отклонён: %v", err)
	}
}

// TestSourceKeyShapes — ключ уезжает В ДВА места сразу: в адрес страницы
// источника на афише и в `community_follows` на стороне core-api. Разойдись
// форма — подписка не совпадёт с карточкой, и человек увидит пустую ленту
// «моих сообществ», не получив ни одной ошибки.
func TestSourceKeyShapes(t *testing.T) {
	good := []string{
		"tg:ranepasport",
		"tg:HSEafisha",             // регистр в адресе значим: t.me его хранит
		"vk:theacademy",
		"vk:id351283035",
		"kudago:place:34843",       // у агрегатора организатор — площадка
		"vk:konstantinova.maria",   // точка в screen_name встречается
	}
	for _, k := range good {
		s := validSource()
		s.Key = k
		if err := s.Validate(); err != nil {
			t.Errorf("ключ %q отклонён: %v", k, err)
		}
	}
	bad := map[string]string{
		"без двоеточия":            "ranepasport",
		"пустое пространство имён": ":ranepasport",
		"пустой адрес":             "tg:",
		"пробел в адресе":          "tg:ranepa sport",
		"верхний регистр в пространстве": "TG:ranepasport",
		// Ключ становится сегментом пути — обход каталога тут не должен даже
		// доехать до маршрутизатора.
		"обход каталога": "tg:../secret",
		"слэш в адресе":  "tg:ranepa/sport",
	}
	for name, k := range bad {
		s := validSource()
		s.Key = k
		if err := s.Validate(); err == nil {
			t.Errorf("%s: ключ %q принят, а не должен", name, k)
		}
	}
}

func TestSourceValidateRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Source)
		want   string
	}{
		{"платформа вне словаря", func(s *Source) { s.Platform = "rss" }, "platform"},
		{"пустой handle", func(s *Source) { s.Handle = "  " }, "handle"},
		// Заголовок — единственное, что человек видит в списке подписок.
		{"пустой title", func(s *Source) { s.Title = "" }, "title"},
		{"url не http(s)", func(s *Source) { u := "tg://resolve?x"; s.URL = &u }, "url"},
		{"логотип без mime", func(s *Source) { b := "aGkh"; s.LogoB64 = &b }, "logo_mime"},
		{"логотип с не-картиночным mime", func(s *Source) {
			b, m := "aGkh", "text/html"
			s.LogoB64, s.LogoMime = &b, &m
		}, "logo_mime"},
		{"логотип не base64", func(s *Source) {
			b, m := "не base64 вовсе!!", "image/jpeg"
			s.LogoB64, s.LogoMime = &b, &m
		}, "base64"},
		{"логотип больше потолка", func(s *Source) {
			b, m := base64.StdEncoding.EncodeToString(make([]byte, maxLogoBytes+1)), "image/jpeg"
			s.LogoB64, s.LogoMime = &b, &m
		}, "потолка"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := validSource()
			c.mutate(&s)
			err := s.Validate()
			if err == nil {
				t.Fatalf("принят, а не должен")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("ошибка %q не содержит %q — читателю лога непонятно, что чинить", err, c.want)
			}
		})
	}
}

// TestCardSourceKeyValidated — карточка несёт ключ реестра, и он проверяется
// той же регуляркой. Иначе мусорный ключ уехал бы в базу и подписка на этого
// организатора просто никогда бы не сработала — молча.
func TestCardSourceKeyValidated(t *testing.T) {
	c := validCard()
	if err := c.Validate(); err != nil {
		t.Fatalf("образец карточки невалиден: %v", err)
	}
	good := "tg:ranepasport"
	c.SourceKey = &good
	if err := c.Validate(); err != nil {
		t.Fatalf("карточка с валидным ключом отклонена: %v", err)
	}
	bad := "ranepasport"
	c.SourceKey = &bad
	if err := c.Validate(); err == nil {
		t.Error("карточка с ключом без пространства имён принята")
	}
	// Отсутствие ключа законно: карточки старше реестра, и обезличивать их
	// отказом заливки нельзя — они уже на витрине.
	empty := ""
	c.SourceKey = &empty
	if err := c.Validate(); err != nil {
		t.Errorf("пустой ключ отклонён, а это то же «нет ключа»: %v", err)
	}
}
