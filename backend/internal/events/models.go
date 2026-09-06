package events

import (
	"encoding/json"
	"time"
)

type PublicEvent struct {
	ID               string          `json:"id"`
	Title            string          `json:"title"`
	ShortDescription *string         `json:"short_description,omitempty"`
	Description      *string         `json:"description,omitempty"`
	Location         *string         `json:"location,omitempty"`
	StartTime        time.Time       `json:"start_time"`
	EndTime          *time.Time      `json:"end_time,omitempty"`
	Category         *string         `json:"category,omitempty"`
	Tags             json.RawMessage `json:"tags"`
	MaxAttendees     *int            `json:"max_attendees,omitempty"`
	AttendeeCount    int             `json:"attendee_count"`
	PhotoURL         *string         `json:"photo_url,omitempty"`
	Status           string          `json:"status"`
	RegistrationMode *string         `json:"registration_mode,omitempty"`
	ExternalRegURL   *string         `json:"external_registration_url,omitempty"`
	RegDeadline      *time.Time      `json:"registration_deadline,omitempty"`
	PriceType        *string         `json:"price_type,omitempty"`
	PriceMin         *int            `json:"price_min,omitempty"`
	PriceMax         *int            `json:"price_max,omitempty"`
	Currency         *string         `json:"currency,omitempty"`
	City             *string         `json:"city,omitempty"`
	VenueName        *string         `json:"venue_name,omitempty"`
	Address          *string         `json:"address,omitempty"`
	OnlineURL        *string         `json:"online_url,omitempty"`
	AgeLimit         *string         `json:"age_limit,omitempty"`
	AttendeesNote    *string         `json:"attendees_note,omitempty"`
	IsFeatured       bool            `json:"is_featured"`
	FeaturedPosition *int            `json:"featured_position,omitempty"`
	OrganizerName    *string         `json:"organizer_name,omitempty"`
	OrganizerPhoto   *string         `json:"organizer_photo,omitempty"`
	Photos           []string        `json:"photos"`
	// WebregSlug помечает событие, живущее в собственных таблицах
	// веб-регистрации (см. internal/webreg). Фронт по нему строит ссылку
	// на /e/<slug> вместо /<uuid>.
	WebregSlug string `json:"webreg_slug,omitempty"`
	// SourceURL — ссылка на первоисточник анонса (см. internal/tgevents).
	// Для импортированных событий это не украшение, а условие, на котором
	// мы их вообще показываем: наш текст плюс подписанный источник.
	SourceURL *string `json:"source_url,omitempty"`
	// AccessLevel — «open» / «university» / «invite» у импортированных
	// событий. Человеку важно до клика понимать, пустят ли его.
	AccessLevel *string `json:"access_level,omitempty"`
	// Source — откуда событие: пусто у наших, "tg" у импортированных.
	// ЯВНЫЙ дискриминатор, а не вывод из наличия source_url: тот nullable,
	// и карточка без него превращалась бы в «наше» событие с нашей кнопкой
	// записи на чужое мероприятие.
	Source *string `json:"source,omitempty"`
	// ActualStartDate — настоящий первый день программы, `YYYY-MM-DD`.
	//
	// Стоит ТОЛЬКО у карточек, чей StartTime сдвинут ради сортировки: идущая
	// многодневная выставка показывается в ленте как сегодняшняя, иначе она
	// утонула бы в июне. Человеку это читается верно («СЕГОДНЯ · до 30 СЕН»),
	// а вот в разметке для поисковика сдвиг превращается в утверждение
	// «выставка начинается сегодня», причём новое каждый день — то есть мы
	// от своего имени сообщаем ложный факт о чужом событии. Здесь лежит
	// правда, из которой строится schema.org/Event.
	ActualStartDate *string `json:"actual_start_date,omitempty"`
	// StartTimeKnown=false означает «дата известна, времени нет». Без этого
	// признака полночь неотличима от настоящего начала в 00:00, а фронт
	// печатает её как время события.
	StartTimeKnown *bool `json:"start_time_known,omitempty"`
	// VenueLat / VenueLon / VenueMetro — гео места из кураторского venue
	// (см. internal/tgevents, миграция 008). Опциональны и у большинства
	// событий отсутствуют: курация проставляет их вручную, и потребитель
	// обязан переживать их отсутствие, а не считать нулём.
	VenueLat   *float64 `json:"venue_lat,omitempty"`
	VenueLon   *float64 `json:"venue_lon,omitempty"`
	VenueMetro *string  `json:"venue_metro,omitempty"`
	// RegForm / RegFields — конфигурация формы записи (см. internal/regform).
	// Отдаются сырым JSON: сервер их не интерпретирует при выдаче, страница
	// рисует по ним поля. Отсутствие или `v` меньше 1 означает прежнюю форму
	// из двух полей — так живое событие не меняет форму под теми, кто её
	// сейчас заполняет.
	RegForm   json.RawMessage `json:"reg_form,omitempty"`
	RegFields json.RawMessage `json:"reg_fields,omitempty"`
}

type ListQuery struct {
	OnlyFeatured bool
	Limit        int
	Offset       int
	Since        time.Time
	// Filter — раздел доски: город, категория, «когда», «бесплатно».
	// Нулевое значение означает «фильтра нет» и отдаёт полную доску: через
	// HTTP так не приходит (ParseFilter всегда ставит город), но репозиторий
	// зовут и напрямую, и пустая доска у забывшего заполнить структуру была
	// бы отказом, который выглядит как «событий нет».
	Filter Filter
}

type ListResult struct {
	Featured []PublicEvent `json:"featured"`
	All      []PublicEvent `json:"all"`
	Total    int           `json:"total"`
	// Degraded перечисляет источники ленты, которые не ответили. Пустой
	// список — все живы. Без этого поля отказ источника выглядит как «в нём
	// ничего нет»: ответ 200, лента непустая, просто без половины событий, и
	// отличить одно от другого нечем — логи PROD в Loki не доезжают.
	Degraded []string `json:"degraded,omitempty"`
}

// PublicRegistrationInput is what the signup form sends. Name and Contact are
// the two fields the board has always had; the rest exist only when the event
// configures them (see internal/regform) and arrive empty otherwise, so an old
// page posting just {name, contact} keeps working unchanged.
type PublicRegistrationInput struct {
	Name       string            `json:"name"`
	Contact    string            `json:"contact"`
	FullName   string            `json:"full_name"`
	Email      string            `json:"email"`
	Phone      string            `json:"phone"`
	TGUsername string            `json:"tg_username"`
	Answers    map[string]string `json:"answers"`
}

type PublicRegistrationResult struct {
	RegistrationID    string `json:"registration_id"`
	EventID           string `json:"event_id"`
	Status            string `json:"status"`
	AlreadyRegistered bool   `json:"already_registered,omitempty"`
}

type RegistrationError struct {
	Code        string  `json:"code"`
	Message     string  `json:"message"`
	Status      int     `json:"-"`
	ExternalURL *string `json:"external_registration_url,omitempty"`
	// Fields — сообщение под каждым полем, которое не прошло проверку.
	// Форма настраиваемая, полей может быть с десяток, и один общий текст
	// сверху заставляет человека гадать, какое из них сервер не принял.
	Fields map[string]string `json:"fields,omitempty"`
}

func (e *RegistrationError) Error() string {
	return e.Message
}

// FacetsResponse — ответ GET /api/events/facets.
//
// Плитки разделов и переключатели «когда» / «бесплатно» рисуются по этим
// числам, поэтому число рядом с названием — обещание: столько карточек
// откроется по клику. Считается это теми же выражениями, что и список
// (см. CountFacets), а не похожими.
type FacetsResponse struct {
	City       City            `json:"city"`
	Cities     []CityCount     `json:"cities"`
	Total      int             `json:"total"`
	Categories []CategoryCount `json:"categories"`
	When       map[string]int  `json:"when"`
	Free       int             `json:"free"`
	// Degraded — сторы, которые не ответили. Поля нет в исходном контракте
	// фасетов, и оно добавлено по той же причине, что и у списка: молча
	// заниженное число выглядит достоверным, а логи прода в Loki не доезжают.
	Degraded []string `json:"degraded,omitempty"`
}

// CityCount — город словаря вместе с числом событий под текущим фильтром.
// City встроен, а не вложен: фронту нужны те же падежи, что и в поле city.
type CityCount struct {
	City
	Count int `json:"count"`
}

// CategoryCount — раздел и сколько в нём. Отдаются только непустые: плитка с
// нулём — это тупик, по которому человек кликает и получает пустой экран.
type CategoryCount struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
}
