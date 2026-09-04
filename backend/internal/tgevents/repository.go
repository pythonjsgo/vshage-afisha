package tgevents

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNoCover — у события нет обложки (или самого события): хендлер отвечает 404.
var ErrNoCover = errors.New("обложки нет")

// ErrNotFound — карточки с таким id нет или она снята с витрины.
var ErrNotFound = errors.New("события нет")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// UpsertBulk идемпотентно записывает партию карточек: конфликт по id —
// обновление всех витринных полей. Кураторские колонки (hidden, а с 008 ещё
// venue, feed, anchor) НЕ трогаются ни в списке INSERT, ни в DO UPDATE:
// решение, принятое руками, не должно откатываться следующим импортом.
// Добавлять их сюда нельзя — новая колонка курации заводится так же, мимо
// этого запроса.
//
// Партия пишется одной транзакцией: наполовину применённый импорт хуже
// непримёнённого — витрина показала бы смесь старой и новой партии.
func (r *Repository) UpsertBulk(ctx context.Context, cards []Card) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	n := 0
	for _, c := range cards {
		if err := c.Validate(); err != nil {
			return 0, fmt.Errorf("карточка %s: %w", c.ID, err)
		}
		payload := []byte("{}")
		if c.Payload != nil {
			var err error
			if payload, err = json.Marshal(c.Payload); err != nil {
				return 0, fmt.Errorf("карточка %s: payload: %w", c.ID, err)
			}
		}
		var cover []byte
		var coverMime *string
		if c.CoverB64 != nil && *c.CoverB64 != "" {
			var err error
			if cover, err = base64.StdEncoding.DecodeString(*c.CoverB64); err != nil {
				return 0, fmt.Errorf("карточка %s: cover_b64 не base64: %w", c.ID, err)
			}
			if len(cover) > maxCoverBytes {
				return 0, fmt.Errorf("карточка %s: обложка %d байт > %d", c.ID, len(cover), maxCoverBytes)
			}
			coverMime = c.CoverMime
		}
		// cover через COALESCE: переимпорт БЕЗ обложек (или с недокачанной
		// одной) не должен стирать уже лежащие байты — тот же урок, что у
		// webreg-апсерта с EXCLUDED.x по телефону гостя.
		if _, err := tx.Exec(ctx, `
			INSERT INTO afisha_tg_events
				(id, title, annonce, date, date_end, time_start, city,
				 place_name, address, online, price_raw, is_free,
				 registration_url, access_level, segment, category, org_name,
				 source_url, source_key, payload, cover, cover_mime, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			        $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, NOW())
			ON CONFLICT (id) DO UPDATE SET
				title            = EXCLUDED.title,
				annonce          = EXCLUDED.annonce,
				date             = EXCLUDED.date,
				date_end         = EXCLUDED.date_end,
				time_start       = EXCLUDED.time_start,
				city             = EXCLUDED.city,
				place_name       = EXCLUDED.place_name,
				address          = EXCLUDED.address,
				online           = EXCLUDED.online,
				price_raw        = EXCLUDED.price_raw,
				is_free          = EXCLUDED.is_free,
				registration_url = EXCLUDED.registration_url,
				access_level     = EXCLUDED.access_level,
				segment          = EXCLUDED.segment,
				-- COALESCE, а не присвоение: NULL здесь означает «конвейер
				-- категорию не прислал», и он не должен стирать ни ранее
				-- присланную, ни поправленную куратором. Присланная — заменяет,
				-- как заменяют title и date: это данные импорта, а не курация.
				category         = COALESCE(EXCLUDED.category, afisha_tg_events.category),
				org_name         = EXCLUDED.org_name,
				source_url       = EXCLUDED.source_url,
				-- COALESCE: конвейер, который ключа ещё не шлёт (или прислал
				-- пустой), не должен обезличивать уже привязанную карточку —
				-- вместе с ней отвалилась бы и подписка на её организатора.
				source_key       = COALESCE(EXCLUDED.source_key, afisha_tg_events.source_key),
				payload          = EXCLUDED.payload,
				cover            = COALESCE(EXCLUDED.cover, afisha_tg_events.cover),
				cover_mime       = COALESCE(EXCLUDED.cover_mime, afisha_tg_events.cover_mime),
				updated_at       = NOW()`,
			c.ID, c.Title, c.Annonce, c.Date, c.DateEnd, c.TimeStart, c.City,
			c.PlaceName, c.Address, c.Online, c.PriceRaw, c.IsFree,
			c.RegistrationURL, c.AccessLevel, c.Segment, c.Category, c.OrgName,
			c.SourceURL, c.SourceKey, payload, cover, coverMime,
		); err != nil {
			return 0, fmt.Errorf("карточка %s: %w", c.ID, err)
		}
		n++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return n, nil
}

// AdminFlags — частичная правка курации: nil означает «поле не названо, не
// трогать». Именно частичная, а не подмена строки: снятие с витрины (hidden)
// и допуск на полку ленты (feed/anchor) — разные решения, и правка одного не
// должна возвращать другому умолчание.
type AdminFlags struct {
	Feed   *bool
	Anchor *bool
	Hidden *bool
}

// AdminState — состояние строки ПОСЛЕ правки. Отдаём его, а не «ok»: курация
// идёт руками, и единственный способ увидеть, что применилось именно то,
// что просили, — прочитать ответ.
type AdminState struct {
	ID     string `json:"id"`
	Feed   bool   `json:"feed"`
	Anchor bool   `json:"anchor"`
	Hidden bool   `json:"hidden"`
}

// AdminSetFlags применяет частичную правку курации. ErrNotFound, если карточки
// с таким id нет (скрытые правятся тоже — иначе снятое было бы не вернуть).
func (r *Repository) AdminSetFlags(ctx context.Context, id string, f AdminFlags) (AdminState, error) {
	var st AdminState
	// COALESCE с явным ::boolean: неназванное поле приезжает NULL-параметром,
	// и без каста тип параметра выводить не из чего.
	err := r.pool.QueryRow(ctx, `
		UPDATE afisha_tg_events SET
			feed       = COALESCE($2::boolean, feed),
			anchor     = COALESCE($3::boolean, anchor),
			hidden     = COALESCE($4::boolean, hidden),
			updated_at = NOW()
		WHERE id = $1
		RETURNING id, feed, anchor, hidden`,
		id, f.Feed, f.Anchor, f.Hidden).Scan(&st.ID, &st.Feed, &st.Anchor, &st.Hidden)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminState{}, ErrNotFound
	}
	return st, err
}

// AdminListItem — строка списка курации. Ни annonce, ни payload: во втором
// дословный чужой пост, а первый в списке не нужен — решение принимается по
// заголовку, дате и городу.
type AdminListItem struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Date   string  `json:"date"`
	City   *string `json:"city"`
	Feed   bool    `json:"feed"`
	Anchor bool    `json:"anchor"`
	Hidden bool    `json:"hidden"`
}

// AdminList — весь стор для курации, включая скрытое и прошедшее: список
// существует, чтобы решать судьбу карточек, и спрятанное от него было бы
// не вернуть на витрину.
func (r *Repository) AdminList(ctx context.Context) ([]AdminListItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, title, to_char(date, 'YYYY-MM-DD'), city, feed, anchor, hidden
		FROM afisha_tg_events
		ORDER BY date, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Непустой слайс: пустая витрина должна приехать как [], а не null —
	// иначе курации приходится различать «пусто» и «поле не пришло».
	out := []AdminListItem{}
	for rows.Next() {
		var it AdminListItem
		if err := rows.Scan(&it.ID, &it.Title, &it.Date, &it.City,
			&it.Feed, &it.Anchor, &it.Hidden); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Cover — байты обложки для отдачи браузеру. ErrNoCover, если события нет,
// оно скрыто или обложка не заливалась.
func (r *Repository) Cover(ctx context.Context, id string) ([]byte, string, error) {
	var data []byte
	var mime *string
	err := r.pool.QueryRow(ctx, `
		SELECT cover, cover_mime FROM afisha_tg_events
		WHERE id = $1 AND NOT hidden AND cover IS NOT NULL`, id).Scan(&data, &mime)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNoCover
	}
	if err != nil {
		return nil, "", err
	}
	m := "image/jpeg"
	if mime != nil && *mime != "" {
		m = *mime
	}
	return data, m, nil
}
