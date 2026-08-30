package tgevents

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/pythonjsgo/vshage-afisha/internal/events"
)

// Студсобытия в ОБЩЕЙ ленте афиши (директива фаундера 30.08).
//
// 23.08 их увели на отдельную страницу /uni из-за трёх препятствий: шов
// events.ExtraSource был однослотовым и уже занят веб-регистрацией, пагинации
// у него не было, а [id]-роут фронта трактовал любой не-UUID как слаг
// веб-регистрации, и ev_* уходили в 404. Все три сняты, а не обойдены:
// шов стал слайсом с пагинацией (internal/events/handler.go), окно режет
// events.MergePage, одиночную карточку отдаёт Handler.Get ниже по файлу.
//
// Почему НЕ строкой в таблице events, хотя так было бы «правильнее». events —
// общая поверхность: те же строки core-api отдаёт в приложение, фильтруя
// только по status='published' (ни источника, ни visibility там нет), а
// регистрация проверяет лишь статус и вместимость. Студсобытие в events
// немедленно всплыло бы у всех живых билдов, включая сторовый 120, и человек
// смог бы «зарегистрироваться» на чужую выставку, получив успех и строку в
// event_registrations — регистрацию, которой в реальности не существует.
// Решение фаундера 30.08: лента афиши читает оба стора, приложение не трогаем.
//
// Юр-рамка: наружу едут ТОЛЬКО наш нормализованный заголовок, наш annonce,
// ссылка на первоисточник и превью. Дословный чужой текст (title_raw,
// text_full) лежит в payload и наружу не выходит — здесь его нет ни в одном
// SELECT, и добавлять нельзя.

// UpcomingForAfisha реализует events.ExtraSource.
//
// `since` приходит от ленты как «сейчас минус сутки» и переводится в дату по
// МСК: у карточек нет времени с точностью до минуты, у 9 из 30 его нет вовсе,
// и сравнивать сутки правильнее календарно. Зона фиксированная, не
// LoadLocation, — образ без tzdata молча уехал бы в UTC и до 03:00 МСК прятал
// бы сегодняшние события.
// selectCard — общий список колонок. Эффективные дата и время (eff_date,
// eff_time) считаются ЗДЕСЬ и только здесь: идущая многодневная программа
// сортируется и показывается как сегодняшняя, а не как начавшаяся полгода
// назад. Раньше этот сдвиг жил в маппере, а порядок — в ORDER BY, и они
// дважды разошлись: сначала на бестаймовых, потом на самом сдвиге (замер на
// DEV дал два события сразу на первой и второй странице). Одно выражение на
// оба применения — единственная защита, которую нельзя забыть продублировать.
const selectCard = `
		SELECT * FROM (
		SELECT id, title, annonce,
		       to_char(date, 'YYYY-MM-DD') AS d,
		       to_char(date_end, 'YYYY-MM-DD') AS de,
		       -- Сдвигаем на сегодня только программу, которая ЕЩЁ ИДЁТ.
		       -- Условие «date_end >= сегодня» обязательно: лента держит
		       -- вчерашнее ещё сутки (окно -24ч), и без него уже кончившаяся
		       -- программа получала сегодняшний старт — карточка говорила
		       -- «СЕГОДНЯ · до 29 АВГ», то есть сама себе противоречила.
		       CASE WHEN date_end IS NOT NULL AND date < $1::date AND date_end >= $1::date
		            THEN $1::date ELSE date END AS eff_date,
		       CASE WHEN date_end IS NOT NULL AND date < $1::date AND date_end >= $1::date
		            THEN NULL ELSE time_start END AS eff_time,
		       city, place_name, address, online,
		       price_raw, is_free, registration_url, access_level,
		       segment, org_name, source_url, venue,
		       (cover IS NOT NULL) AS has_cover
		FROM afisha_tg_events`

// orderCard — порядок ровно по тому же ключу, каким сливает events.MergePage.
//
// Выборка обёрнута в подзапрос НЕ для красоты: Postgres разрешает ссылаться на
// псевдоним SELECT в ORDER BY только голым именем, а нам нужен
// COALESCE(eff_time, ...) — внутри выражения он ищет колонку таблицы и падает
// с `column "eff_time" does not exist`. Повторить CASE в ORDER BY значило бы
// снова завести второе определение сдвига, которое уже дважды разъезжалось.
const orderCard = `) t ORDER BY eff_date, COALESCE(eff_time, '00:00'), id`

func (r *Repository) UpcomingForAfisha(ctx context.Context, since time.Time, limit, offset int) ([]events.PublicEvent, error) {
	// Клампинг, а не откат: просили больше потолка — отдаём потолок.
	// Откат к 30 означал бы «страница набрана», когда она не набрана.
	if limit <= 0 {
		limit = 30
	}
	if limit > 300 {
		limit = 300
	}
	if offset < 0 {
		offset = 0
	}
	day := since.In(msk).Format(dateLayout)
	// «Сегодня» считаем по МСК и передаём параметром: CURRENT_DATE в
	// контейнере — UTC, и с полуночи до трёх ночи это другой день.
	today := time.Now().In(msk).Format(dateLayout)

	rows, err := r.pool.Query(ctx, selectCard+`
		WHERE NOT hidden AND COALESCE(date_end, date) >= $2`+orderCard+`
		LIMIT $3 OFFSET $4`, today, day, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []events.PublicEvent{}
	for rows.Next() {
		row, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, toPublic(row))
	}
	return out, rows.Err()
}

// CountUpcomingForAfisha — тот же WHERE, что и в выборке. Разойдутся условия —
// разойдётся «показано N из M», и заметит это человек, долиставший до конца.
func (r *Repository) CountUpcomingForAfisha(ctx context.Context, since time.Time) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM afisha_tg_events
		WHERE NOT hidden AND COALESCE(date_end, date) >= $1
	`, since.In(msk).Format(dateLayout)).Scan(&n)
	return n, err
}

// AfishaSourceName — как источник называется в поле degraded ленты.
func (r *Repository) AfishaSourceName() string { return "tg" }

// GetByID — одиночная карточка для страницы события. Без неё каждая карточка
// в ленте вела бы в 404: id вида ev_* уходил бы во фронте в /api/e/<slug> как
// слаг веб-регистрации (это и было третьим препятствием 23.08).
// GetByID — одиночная карточка для страницы события. Без неё каждая карточка
// в ленте вела бы в 404: id вида ev_* уходил бы во фронте в /api/e/<slug> как
// слаг веб-регистрации (это и было третьим препятствием 23.08).
func (r *Repository) GetByID(ctx context.Context, id string) (events.PublicEvent, error) {
	today := time.Now().In(msk).Format(dateLayout)
	rows, err := r.pool.Query(ctx, selectCard+`
		WHERE id = $2 AND NOT hidden) t`, today, id)
	if err != nil {
		return events.PublicEvent{}, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return events.PublicEvent{}, err
		}
		return events.PublicEvent{}, ErrNotFound
	}
	row, err := scanCard(rows)
	if err != nil {
		return events.PublicEvent{}, err
	}
	return toPublic(row), rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

// eff — эффективные дата и время карточки, посчитанные в SQL (см. selectCard).
// eff.Time == nil означает «времени нет»: либо его не было в анонсе, либо
// программа идёт с прошлой даты и сдвинута на сегодня.
type eff struct {
	Date string
	Time *string
}

// cardRow — одна строка выборки витрины. Структурой, а не пятью возвратами:
// колонок у выборки прибавляется, и каждая новая иначе разъезжается по двум
// сигнатурам плюс двум вызовам.
type cardRow struct {
	Card     Card
	Eff      eff
	HasCover bool
	// Venue — сырой кураторский JSONB (008_feed_curation.sql). Разбирается в
	// parseVenue, а не сканируется в структуру: поле правится руками, и его
	// может не быть вовсе.
	Venue []byte
}

func scanCard(rows scanner) (cardRow, error) {
	var row cardRow
	var effDate time.Time
	err := rows.Scan(&row.Card.ID, &row.Card.Title, &row.Card.Annonce,
		&row.Card.Date, &row.Card.DateEnd,
		&effDate, &row.Eff.Time,
		&row.Card.City, &row.Card.PlaceName, &row.Card.Address, &row.Card.Online,
		&row.Card.PriceRaw, &row.Card.IsFree, &row.Card.RegistrationURL,
		&row.Card.AccessLevel, &row.Card.Segment, &row.Card.OrgName,
		&row.Card.SourceURL, &row.Venue, &row.HasCover)
	row.Eff.Date = effDate.Format(dateLayout)
	return row, err
}

// venueGeo — то, что витрина берёт из кураторского venue: координаты и метро.
// Остальное содержимое объекта игнорируется сознательно — наружу едет ровно
// то, что названо в контракте ленты, а не всё, что курация туда положила.
type venueGeo struct {
	Lat   *float64
	Lon   *float64
	Metro *string
}

// parseVenue разбирает venue максимально терпимо: поле правится руками, и
// битый объект не должен уносить карточку целиком — без гео она всё ещё
// показывается, а с nil-координатами клиент просто не рисует точку.
func parseVenue(raw []byte) venueGeo {
	var g venueGeo
	if len(raw) == 0 {
		return g
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		log.Printf("tgevents: venue не разбирается: %v", err)
		return g
	}
	g.Lat = venueCoord(m["lat"], 90)
	g.Lon = venueCoord(m["lon"], 180)
	if s, ok := m["metro"].(string); ok && s != "" {
		g.Metro = &s
	}
	return g
}

// venueCoord принимает и число, и строку («55.75» из ручной правки), и
// проверяет диапазон: перепутанные местами широта с долготой — самая частая
// опечатка курации, и её лучше не показать вовсе, чем поставить точку в море.
func venueCoord(v any, limit float64) *float64 {
	var f float64
	switch t := v.(type) {
	case float64:
		f = t
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err != nil {
			return nil
		}
		f = parsed
	default:
		return nil
	}
	if f < -limit || f > limit {
		log.Printf("tgevents: venue-координата %v вне диапазона ±%v", f, limit)
		return nil
	}
	return &f
}

// toPublic переводит карточку витрины в контракт ленты.
//
// Время. Отсутствие времени кодируется полуночью МСК, и это решение записано
// здесь, а не подразумевается: иначе через месяц кто-нибудь прочитает полночь
// как «событие ночью». Дата собирается СРАЗУ в МСК — наивный разбор через UTC
// сдвинул бы все карточки без времени на день назад.
//
// Сдвиг идущих многодневных программ на сегодня здесь НЕ делается: он
// посчитан в SQL (selectCard) и приезжает готовым, потому что тем же
// выражением сортируется выборка. Считать его в двух местах уже пробовали —
// разошлись дважды.
func toPublic(row cardRow) events.PublicEvent {
	c, e, hasCover := row.Card, row.Eff, row.HasCover
	start := parseMSK(e.Date, e.Time)
	timeKnown := e.Time != nil && *e.Time != ""
	source := "tg"

	ev := events.PublicEvent{
		ID:               c.ID,
		Title:            c.Title,
		ShortDescription: strPtr(c.Annonce),
		Description:      strPtr(c.Annonce),
		StartTime:        start,
		Status:           "published",
		Tags:             json.RawMessage("[]"),
		Photos:           []string{},
		SourceURL:        c.SourceURL,
		AccessLevel:      strPtr(c.AccessLevel),
		City:             c.City,
		VenueName:        c.PlaceName,
		Address:          c.Address,
		Source:           &source,
		StartTimeKnown:   &timeKnown,
	}
	if c.DateEnd != nil && *c.DateEnd != "" && *c.DateEnd != c.Date {
		// Конец многодневной программы — конец её последнего дня, иначе
		// выставка «до 20 сентября» исчезала бы из ленты утром 20-го.
		end := parseMSK(*c.DateEnd, nil).Add(23*time.Hour + 59*time.Minute)
		ev.EndTime = &end
	}
	if hasCover {
		// Свой origin, не CDN телеги: тот протухает за дни (замер 23.08).
		ev.PhotoURL = strPtr("/api/tg-events/" + c.ID + "/cover")
	}
	if c.PlaceName != nil && *c.PlaceName != "" {
		ev.Location = c.PlaceName
	} else if c.Address != nil && *c.Address != "" {
		ev.Location = c.Address
	}
	if c.OrgName != nil && *c.OrgName != "" {
		ev.OrganizerName = c.OrgName
	}
	if c.IsFree != nil && *c.IsFree {
		ev.PriceType = strPtr("free")
	}
	// Регистрация всегда ВНЕШНЯЯ: мы не ведём списки на чужие события.
	ev.RegistrationMode = strPtr("external")
	if c.RegistrationURL != nil && *c.RegistrationURL != "" {
		ev.ExternalRegURL = c.RegistrationURL
	}
	if c.Online {
		ev.OnlineURL = c.RegistrationURL
	}
	// Гео места из кураторского venue. Отдельными полями, а не объектом:
	// PublicEvent — общий контракт ленты, и вложенный объект пришлось бы
	// заводить всем источникам, у которых его нет.
	g := parseVenue(row.Venue)
	ev.VenueLat, ev.VenueLon, ev.VenueMetro = g.Lat, g.Lon, g.Metro
	return ev
}

func parseMSK(day string, hhmm *string) time.Time {
	t, err := time.ParseInLocation(dateLayout, day, msk)
	if err != nil {
		return time.Time{}
	}
	if hhmm == nil || *hhmm == "" {
		return t
	}
	withTime, err := time.ParseInLocation(dateLayout+" 15:04", day+" "+*hhmm, msk)
	if err != nil {
		return t
	}
	return withTime
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
