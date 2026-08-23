-- 006_tg_events.sql — события, импортированные из телеграм-каналов
-- (конвейер vshage-geo: tg_harvest → грок-разбор → tg_event_cards.jsonl).
--
-- По шаблону webreg (003): собственные таблицы, ноль внешних ключей в
-- сторону приложения — переезжают с афишей в изолированную БД одним
-- переключением DATABASE_URL. FK на events(id) не ставить: так приросли
-- afisha_featured и afisha_event_photos, и теперь они не переедут.
--
-- Юридическая рамка витрины: показываем ТОЛЬКО наш сгенерированный текст
-- (annonce) и ссылку на первоисточник. Чужие тексты и медиа из телеги на
-- витрину не идут, поэтому у таблицы нет ни text_full, ни колонок медиа —
-- полная карточка лежит в payload для отладки и переимпорта, наружу из неё
-- ничего не отдаётся.

CREATE TABLE IF NOT EXISTS afisha_tg_events (
    -- ev_<hash> из vshage-geo; стабилен между импортами — ключ идемпотентного
    -- апсерта (дедуп конструкцией схемы, а не кодом).
    id               TEXT PRIMARY KEY,
    title            TEXT        NOT NULL,
    -- Наш витринный текст, 2-3 предложения. Генерируется гроком с запретом
    -- копировать фразы поста (EVENTS-CONTRACT.md §annonce).
    annonce          TEXT        NOT NULL,
    date             DATE        NOT NULL,
    date_end         DATE,
    time_start       TEXT,
    city             TEXT,
    place_name       TEXT,
    address          TEXT,
    online           BOOLEAN     NOT NULL DEFAULT FALSE,
    price_raw        TEXT,
    is_free          BOOLEAN,
    registration_url TEXT,
    -- open | university | invite | unknown — уровень доступа, который грок
    -- прочитал из текста поста (816/890 с дословной цитатой).
    access_level     TEXT        NOT NULL DEFAULT 'unknown',
    segment          TEXT,
    org_name         TEXT,
    -- Ссылка на пост-первоисточник (t.me/...). Обязательная часть витрины.
    source_url       TEXT,
    -- Полная карточка как пришла из конвейера — для отладки и будущих полей.
    payload          JSONB       NOT NULL DEFAULT '{}'::jsonb,
    -- Ручное снятие с витрины без удаления: скрытое не вернётся при
    -- следующем импорте, потому что апсерт hidden не трогает.
    hidden           BOOLEAN     NOT NULL DEFAULT FALSE,
    imported_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Витрина читает «предстоящие и не скрытые» — индекс ровно под этот запрос.
CREATE INDEX IF NOT EXISTS idx_afisha_tg_events_upcoming
    ON afisha_tg_events (date) WHERE NOT hidden;
