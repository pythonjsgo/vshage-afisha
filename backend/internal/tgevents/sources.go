package tgevents

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pythonjsgo/vshage-afisha/internal/events"
)

// Реестр внешних источников (миграция 017).
//
// Строка здесь — это личность организатора, события которого мы индексируем и
// у которого нет кабинета: канал вуза, сообщество, площадка города. Ключ той
// же формы, что читает core-api в `community_follows`, поэтому подписка,
// сделанная в приложении, указывает ровно на эту строку.
//
// Данные приезжают из конвейера (vshage-geo), а не собираются здесь: карточки
// каналов там уже строятся (`events/tg_channels.jsonl`), и второй сборщик
// рядом означал бы два расходящихся описания одного канала.

// ErrNoLogo — у источника нет логотипа (или самого источника).
var ErrNoLogo = errors.New("логотипа нет")

// ErrSourceNotFound — источника с таким ключом нет.
var ErrSourceNotFound = errors.New("источника нет")

// sourceKeyRe — форма ключа. Двоеточие разделяет пространство имён и адрес:
// `tg:ranepasport`, `vk:theacademy`, `kudago:place:34843`. Проверяется здесь,
// потому что здесь граница доверия между конвейером и публичной поверхностью,
// и потому что ключ уезжает в URL страницы источника.
var sourceKeyRe = regexp.MustCompile(`^[a-z][a-z0-9]{1,15}:[A-Za-z0-9_.:-]{1,120}$`)

// sourcePlatforms — закрытый словарь платформ. `api` — источник, отдающий
// события структурой (KudaGo), а не постами.
var sourcePlatforms = map[string]bool{"tg": true, "vk": true, "api": true}

// Source — строка реестра. Всё, кроме key/platform/handle/title,
// необязательно: у источника может не быть ни описания, ни числа
// подписчиков, и это не повод его не заводить.
type Source struct {
	Key        string         `json:"key"`
	Platform   string         `json:"platform"`
	Handle     string         `json:"handle"`
	Title      string         `json:"title"`
	URL        *string        `json:"url,omitempty"`
	Descr      *string        `json:"descr,omitempty"`
	Subs       *int           `json:"subs,omitempty"`
	Kind       *string        `json:"kind,omitempty"`
	University *string        `json:"university,omitempty"`
	Segments   map[string]any `json:"segments,omitempty"`
	Stats      map[string]any `json:"stats,omitempty"`
	Flags      []string       `json:"flags,omitempty"`
	// Enabled — источник опрашивается и его карточки показываются. Отзыв
	// владельцем ставится сюда; гашение карточек делает UpsertSources, а не
	// читатели (см. комментарий миграции 017).
	Enabled bool `json:"enabled"`
	// Логотип на входе — base64 + mime, как обложка карточки. Наружу отдаётся
	// ссылкой на свой эндпоинт: чужой CDN протухает.
	LogoB64  *string `json:"logo_b64,omitempty"`
	LogoMime *string `json:"logo_mime,omitempty"`
	LogoURL  string  `json:"logo_url,omitempty"`
}

// maxLogoBytes ограничивает РАСКОДИРОВАННЫЙ логотип. Аватар — десятки КБ;
// мегабайт означает, что скачали не аватар.
const maxLogoBytes = 1 << 20

// Validate проверяет строку реестра перед записью.
func (s *Source) Validate() error {
	if !sourceKeyRe.MatchString(s.Key) {
		return fmt.Errorf("key %q не вида <пространство>:<адрес>", s.Key)
	}
	if !sourcePlatforms[s.Platform] {
		return fmt.Errorf("platform %q вне словаря", s.Platform)
	}
	if strings.TrimSpace(s.Handle) == "" {
		return fmt.Errorf("%s: пустой handle", s.Key)
	}
	// Заголовок — единственное, что человек увидит в списке подписок. Пустой
	// сделал бы там безымянную строку, на которую нельзя осмысленно нажать.
	if strings.TrimSpace(s.Title) == "" {
		return fmt.Errorf("%s: пустой title", s.Key)
	}
	if err := validateURL("url", s.URL); err != nil {
		return fmt.Errorf("%s: %w", s.Key, err)
	}
	if s.LogoB64 != nil && *s.LogoB64 != "" {
		if s.LogoMime == nil || !coverMimes[*s.LogoMime] {
			return fmt.Errorf("%s: logo_mime %v вне словаря картинок", s.Key, deref(s.LogoMime))
		}
		raw, err := base64.StdEncoding.DecodeString(*s.LogoB64)
		if err != nil {
			return fmt.Errorf("%s: логотип не base64: %w", s.Key, err)
		}
		if len(raw) > maxLogoBytes {
			return fmt.Errorf("%s: логотип %d байт — больше потолка %d", s.Key, len(raw), maxLogoBytes)
		}
	}
	return nil
}

// UpsertSources идемпотентно пишет реестр одной транзакцией и в ней же гасит
// карточки отозванных источников.
//
// Гашение здесь, а не в читателях, — намеренно. Читателей у карточки трое
// (лента приложения, витрина сайта, страница события), и джойн на реестр в
// каждом стоил бы дороже, а главное — разъехался бы: достаточно забыть один,
// и отозванный источник останется виден ровно в том месте, которое забыли.
// Один писатель ставит `hidden`, читатели остаются как есть — то же правило,
// по которому сделана отложенная публикация.
//
// Возвращает (записано источников, погашено карточек).
func (r *Repository) UpsertSources(ctx context.Context, sources []Source) (int, int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	n := 0
	for _, s := range sources {
		if err := s.Validate(); err != nil {
			return 0, 0, err
		}
		segments, err := jsonOrNull(s.Segments)
		if err != nil {
			return 0, 0, fmt.Errorf("%s: segments: %w", s.Key, err)
		}
		stats, err := jsonOrNull(s.Stats)
		if err != nil {
			return 0, 0, fmt.Errorf("%s: stats: %w", s.Key, err)
		}
		flags, err := jsonOrNull(s.Flags)
		if err != nil {
			return 0, 0, fmt.Errorf("%s: flags: %w", s.Key, err)
		}
		var logo []byte
		var logoMime *string
		if s.LogoB64 != nil && *s.LogoB64 != "" {
			if logo, err = base64.StdEncoding.DecodeString(*s.LogoB64); err != nil {
				return 0, 0, fmt.Errorf("%s: logo_b64 не base64: %w", s.Key, err)
			}
			logoMime = s.LogoMime
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO afisha_sources
				(key, platform, handle, title, url, descr, subs, kind, university,
				 segments, stats, flags, enabled, logo, logo_mime, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,now())
			ON CONFLICT (key) DO UPDATE SET
				platform   = EXCLUDED.platform,
				handle     = EXCLUDED.handle,
				title      = EXCLUDED.title,
				url        = EXCLUDED.url,
				descr      = EXCLUDED.descr,
				subs       = EXCLUDED.subs,
				kind       = EXCLUDED.kind,
				university = EXCLUDED.university,
				segments   = EXCLUDED.segments,
				stats      = EXCLUDED.stats,
				flags      = EXCLUDED.flags,
				enabled    = EXCLUDED.enabled,
				-- COALESCE по той же причине, что у обложки карточки: прогон,
				-- в котором логотип не скачался, не должен стирать уже лежащий.
				logo       = COALESCE(EXCLUDED.logo, afisha_sources.logo),
				logo_mime  = COALESCE(EXCLUDED.logo_mime, afisha_sources.logo_mime),
				-- claimed_by НЕ трогается: это решение человека, а не импорта.
				updated_at = now()`,
			s.Key, s.Platform, s.Handle, s.Title, s.URL, s.Descr, s.Subs, s.Kind,
			s.University, segments, stats, flags, s.Enabled, logo, logoMime,
		); err != nil {
			return 0, 0, fmt.Errorf("источник %s: %w", s.Key, err)
		}
		n++
	}

	// Отозванный источник гаснет одним действием. Обратной дороги нет
	// сознательно: вернуть карточки на витрину — это решение человека
	// (снять hidden в админке), а не побочный эффект того, что источник
	// снова включили.
	tag, err := tx.Exec(ctx, `
		UPDATE afisha_tg_events e SET hidden = TRUE, updated_at = now()
		FROM afisha_sources s
		WHERE s.key = e.source_key AND s.enabled = FALSE AND e.hidden = FALSE`)
	if err != nil {
		return 0, 0, fmt.Errorf("гашение отозванных: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("commit: %w", err)
	}
	return n, int(tag.RowsAffected()), nil
}

// jsonOrNull маршалит значение в JSONB или отдаёт NULL для пустого. nil и
// пустая коллекция — это «не знаем», а не «пусто»: писать в базу `{}` значило
// бы утверждать, что у канала нет ни одной рубрики.
func jsonOrNull(v any) (any, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		if len(t) == 0 {
			return nil, nil
		}
	case []string:
		if len(t) == 0 {
			return nil, nil
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// GetSource — строка реестра для страницы источника. Отключённый источник
// не отдаётся: его страница — это витрина того, чего мы больше не показываем.
func (r *Repository) GetSource(ctx context.Context, key string) (Source, error) {
	var s Source
	var segments, stats, flags []byte
	var hasLogo bool
	err := r.pool.QueryRow(ctx, `
		SELECT key, platform, handle, title, url, descr, subs, kind, university,
		       segments, stats, flags, enabled, (logo IS NOT NULL)
		FROM afisha_sources WHERE key = $1 AND enabled`, key).Scan(
		&s.Key, &s.Platform, &s.Handle, &s.Title, &s.URL, &s.Descr, &s.Subs,
		&s.Kind, &s.University, &segments, &stats, &flags, &s.Enabled, &hasLogo)
	if errors.Is(err, pgx.ErrNoRows) {
		return Source{}, ErrSourceNotFound
	}
	if err != nil {
		return Source{}, err
	}
	_ = json.Unmarshal(segments, &s.Segments)
	_ = json.Unmarshal(stats, &s.Stats)
	_ = json.Unmarshal(flags, &s.Flags)
	if hasLogo {
		s.LogoURL = "/api/sources/" + key + "/logo"
	}
	return s, nil
}

// SourceLogo — байты логотипа со своего origin.
func (r *Repository) SourceLogo(ctx context.Context, key string) ([]byte, string, error) {
	var data []byte
	var mime *string
	err := r.pool.QueryRow(ctx, `
		SELECT logo, logo_mime FROM afisha_sources
		WHERE key = $1 AND enabled AND logo IS NOT NULL`, key).Scan(&data, &mime)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNoLogo
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

// SourceEvents — будущие события источника для его страницы. Тот же предикат
// витрины (не снято, не прошло) и тот же порядок: страница источника — это
// срез общей витрины, а не отдельная выдача со своими правилами.
func (r *Repository) SourceEvents(ctx context.Context, key string, limit int) ([]events.PublicEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	// «Сегодня» по МСК и параметром — как в List: CURRENT_DATE в контейнере
	// это UTC, и с полуночи до трёх ночи он другой день.
	today := time.Now().In(msk).Format(dateLayout)
	rows, err := r.pool.Query(ctx, selectCard+`
		WHERE NOT hidden AND source_key = $2 AND COALESCE(date_end, date) >= $1`+orderCard+`
		LIMIT $3`, today, key, limit)
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
