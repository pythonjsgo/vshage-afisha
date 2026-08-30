// Package tgevents — события, импортированные из телеграм-каналов конвейером
// vshage-geo (tg_harvest → грок-разбор → tg_event_cards.jsonl).
//
// Самостоятельный модуль по образцу webreg: свои таблицы (006_tg_events.sql),
// ноль зависимостей от общей схемы events. С 30.08 подключается в ОБЩУЮ ленту
// афиши через events.ExtraSource (см. afisha.go) — отдельной страницы больше
// нет. Три препятствия, из-за которых 23.08 завели отдельный контур, сняты:
// шов стал многослотовым и пагинируемым, а одиночную карточку отдаёт
// GET /api/tg-events/{id}, и фронт по префиксу ev_ идёт туда, а не в
// веб-регистрацию.
//
// В таблицу events эти события НЕ пишутся сознательно: её читает приложение
// (core-api фильтрует только status='published'), и строка там означала бы
// как минимум чужие анонсы в ленте у сторовых билдов, а как максимум —
// «регистрацию» на мероприятие, которым мы не управляем.
package tgevents

import (
	"fmt"
	"regexp"
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
const timeLayout = "15:04"

// idRe — контракт id карточки (vshage-geo/collect/tg_event_cards.py:
// "ev_" + первые 12 знаков sha1). Проверяется здесь, потому что здесь
// граница доверия между конвейером и публичной поверхностью.
var idRe = regexp.MustCompile(`^ev_[0-9a-f]{6,}$`)

// Validate проверяет карточку перед записью. Возвращает описание первого
// дефекта — импортёр печатает его рядом с id, чтобы брак был виден построчно.
func (c *Card) Validate() error {
	// Формат id — не косметика: по нему фронт решает, в какой бэкенд идти за
	// карточкой (routes/[id]/+page.server.ts). Контракт живёт в vshage-geo,
	// а исполняется здесь: разойдётся генератор — карточка молча уедет в
	// веб-регистрацию и даст посетителю 404, по одной, невидимо в агрегате.
	if !idRe.MatchString(c.ID) {
		return fmt.Errorf("id %q не вида ev_<hex>", c.ID)
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
	// «19:00» разбирается, «19.00», «9:00», «19:00-21:00» и «весь день» — нет,
	// и молча становились бы полуночью, то есть неотличимы от «времени нет».
	// Различить после записи уже невозможно — данные одинаковые.
	if c.TimeStart != nil && strings.TrimSpace(*c.TimeStart) != "" {
		if _, err := time.Parse(timeLayout, *c.TimeStart); err != nil {
			return fmt.Errorf("time_start %q не HH:MM", *c.TimeStart)
		}
	}
	// Ссылки рендерятся фронтом как href, а приходят из чужих постов —
	// схема проверяется на границе доверия (здесь), а не только в экспортёре.
	if err := validateURL("registration_url", c.RegistrationURL); err != nil {
		return err
	}
	if err := validateURL("source_url", c.SourceURL); err != nil {
		return err
	}
	// Источник ОБЯЗАТЕЛЕН, а не желателен: показывать чужой анонс мы вправе
	// только с подписанной ссылкой на первоисточник, и фронт по этому же полю
	// понимает, что событие не наше и кнопки записи у него быть не должно.
	// Пустое поле дало бы карточку с НАШЕЙ кнопкой «Зарегистрироваться» на
	// чужое мероприятие — ровно то, ради чего мы вообще не пишем в events.
	if c.SourceURL == nil || strings.TrimSpace(*c.SourceURL) == "" {
		return fmt.Errorf("пустой source_url")
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
