// Package tgevents — витрина событий, импортированных из телеграм-каналов
// конвейером vshage-geo (tg_harvest → грок-разбор → tg_event_cards.jsonl).
//
// Самостоятельный модуль по образцу webreg: свои таблицы (006_tg_events.sql),
// ноль зависимостей от общей схемы events. В общую ленту через ExtraSource
// сознательно НЕ подключается: у ExtraSource один слот (занят webreg) и нет
// пагинации, а [id]-роут фронта принял бы наши не-UUID id за слаги
// веб-регистрации. Своя страница /uni + свой эндпоинт /api/tg-events.
package tgevents

import (
	"fmt"
	"strings"
	"time"
)

// Card — витринная форма события. Одна структура на вход импорта и выход
// публичного API: импортёр (vshage-geo/collect/export_afisha.py) шлёт ровно
// те поля, которые видит посетитель, плюс payload для отладки.
type Card struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Annonce         string  `json:"annonce"`
	Date            string  `json:"date"`               // YYYY-MM-DD
	DateEnd         *string `json:"date_end,omitempty"` // YYYY-MM-DD
	TimeStart       *string `json:"time_start,omitempty"`
	City            *string `json:"city,omitempty"`
	PlaceName       *string `json:"place_name,omitempty"`
	Address         *string `json:"address,omitempty"`
	Online          bool    `json:"online"`
	PriceRaw        *string `json:"price_raw,omitempty"`
	IsFree          *bool   `json:"is_free,omitempty"`
	RegistrationURL *string `json:"registration_url,omitempty"`
	AccessLevel     string  `json:"access_level"`
	Segment         *string `json:"segment,omitempty"`
	OrgName         *string `json:"org_name,omitempty"`
	SourceURL       *string `json:"source_url,omitempty"`
	// Payload принимается при импорте и хранится, но наружу не отдаётся:
	// внутри полная карточка с чужим текстом поста (юр. рамка витрины).
	Payload map[string]any `json:"payload,omitempty"`

	// Обложка (решение фаундера 23.08: превью поста на витрине, источник
	// подписан). На входе импорта — байты base64 + mime; наружу List отдаёт
	// только CoverURL на наш же эндпоинт: телеграмный CDN протухает за дни.
	CoverB64  *string `json:"cover_b64,omitempty"`
	CoverMime *string `json:"cover_mime,omitempty"`
	CoverURL  string  `json:"cover_url,omitempty"`
}

// coverMimes — что витрина согласна отдать браузеру как картинку.
var coverMimes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true,
}

// maxCoverBytes ограничивает РАСКОДИРОВАННУЮ обложку: превью постов — сотни
// КБ; мегабайты — признак того, что импортёр скачал не то.
const maxCoverBytes = 3 << 20

// accessLevels — закрытый словарь уровня доступа из контракта карточек
// (vshage-geo EVENTS-CONTRACT.md). Неизвестное значение — брак импорта,
// а не повод молча записать "unknown".
var accessLevels = map[string]bool{
	"open": true, "university": true, "invite": true, "unknown": true,
}

const dateLayout = "2006-01-02"

// Validate проверяет карточку перед записью. Возвращает описание первого
// дефекта — импортёр печатает его рядом с id, чтобы брак был виден построчно.
func (c *Card) Validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("пустой id")
	}
	if strings.TrimSpace(c.Title) == "" {
		return fmt.Errorf("пустой title")
	}
	// Витрина без нашего текста показывала бы чужой — annonce обязателен.
	if strings.TrimSpace(c.Annonce) == "" {
		return fmt.Errorf("пустой annonce")
	}
	if _, err := time.Parse(dateLayout, c.Date); err != nil {
		return fmt.Errorf("date %q не YYYY-MM-DD", c.Date)
	}
	if c.DateEnd != nil {
		if _, err := time.Parse(dateLayout, *c.DateEnd); err != nil {
			return fmt.Errorf("date_end %q не YYYY-MM-DD", *c.DateEnd)
		}
	}
	if !accessLevels[c.AccessLevel] {
		return fmt.Errorf("access_level %q вне словаря", c.AccessLevel)
	}
	// Ссылки рендерятся фронтом как href, а приходят из чужих постов —
	// схема проверяется на границе доверия (здесь), а не только в экспортёре.
	if err := validateURL("registration_url", c.RegistrationURL); err != nil {
		return err
	}
	if err := validateURL("source_url", c.SourceURL); err != nil {
		return err
	}
	if c.CoverB64 != nil && *c.CoverB64 != "" {
		if c.CoverMime == nil || !coverMimes[*c.CoverMime] {
			return fmt.Errorf("cover_mime %v вне словаря картинок", deref(c.CoverMime))
		}
	}
	return nil
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func validateURL(field string, v *string) error {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil
	}
	if !strings.HasPrefix(*v, "http://") && !strings.HasPrefix(*v, "https://") {
		return fmt.Errorf("%s %q не http(s)", field, *v)
	}
	return nil
}
