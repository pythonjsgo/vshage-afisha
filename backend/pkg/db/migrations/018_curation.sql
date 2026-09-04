-- 018_curation.sql — курация витрины: закрепление, скрытие с причиной, журнал.
--
-- Директива фаундера 05.09: партнёр (Ваня, сообщество ШАГ) просит «ШАГ выше,
-- лишнее убрать», партнёры увидят афишу на неделе. Это курация, а не редизайн.
--
-- ЗАКРЕПЛЕНИЕ — НЕ ВЕС. `anchor` (008) уже участвует в формуле ленты, но
-- весом 0.05 из единицы: он ничего наверх не поднимет, и «Панелька первая у
-- всех ШАГ-юзеров» через него недостижима в принципе. Поэтому `featured` —
-- отдельное поле, которое лента применяет ЗАКРЕПЛЕНИЕМ в пост-обработке, а не
-- слагаемым. `anchor` остаётся как был: «событие недели» и закрепление — два
-- разных кураторских действия, и склеивать их значило бы отобрать одно.
--
-- `featured_until` пустой = «до конца события». Просроченное закрепление
-- гасит тот же минутный свип кабинета, что уже гасит `publish_at`: один
-- писатель меняет строки, читатели остаются как есть.
--
-- СКРЫТИЕ — ТРЕТЬЕ СОСТОЯНИЕ, А НЕ ПРАВКА СМЫСЛА `hidden`. Сегодня `hidden`
-- закрывает и прямую ссылку (`GetByID ... AND NOT hidden`), и этим же полем
-- очередь источников помечает «не принято модерацией». Фаундер просит другое:
-- «скрытое не видно в ленте и на web-афише, но открывается по прямой ссылке —
-- регистрации не ломать». Это `listed = FALSE`: карточки нет в списках, но
-- она открывается тому, у кого есть ссылка. `hidden` не тронут.
ALTER TABLE afisha_tg_events
    ADD COLUMN IF NOT EXISTS featured       BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS featured_until TIMESTAMPTZ,
    -- listed=TRUE у всех живых строк, и это ровно их сегодняшнее поведение:
    -- новая колонка с DEFAULT не меняет смысла ни одной существующей записи
    -- (в отличие от `ticket_mode DEFAULT 'qr'`, который однажды раздал билеты
    -- идущему мероприятию).
    ADD COLUMN IF NOT EXISTS listed         BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS hide_reason    TEXT,
    ADD COLUMN IF NOT EXISTS hidden_by      TEXT,
    ADD COLUMN IF NOT EXISTS hidden_at      TIMESTAMPTZ;

COMMENT ON COLUMN afisha_tg_events.featured IS
    'Закрепление куратором. Лента применяет его закреплением в пост-обработке, а не весом: вес anchor=0.05 наверх не поднимает.';
COMMENT ON COLUMN afisha_tg_events.listed IS
    'FALSE = снято со списков (лента, витрина), но открывается по прямой ссылке — розданные ссылки и записи не ломаются. Это НЕ hidden: hidden означает «не показывать нигде».';

-- Витрина ленты читается core-api этим предикатом; закрепление участвует в
-- порядке, поэтому индекс несёт его первым.
CREATE INDEX IF NOT EXISTS idx_afisha_tg_events_featured
    ON afisha_tg_events (featured, date) WHERE feed AND NOT hidden AND listed;

-- Журнал кураторских решений: кто, что, когда и почему. Без него на вопрос
-- «почему это скрыто» отвечать нечем, а с двумя кураторами (кабинет партнёра
-- и наша админка) это вопрос ближайшей недели.
CREATE TABLE IF NOT EXISTS afisha_curation_log (
    id         BIGSERIAL PRIMARY KEY,
    event_id   TEXT NOT NULL,
    -- Кто: id профиля из кабинета либо 'admin' для внутренней админки. TEXT
    -- без внешнего ключа по той же причине, что и claimed_by в 017: таблицей
    -- владеет афиша, профилями — core-api.
    actor      TEXT NOT NULL,
    action     TEXT NOT NULL,       -- feature | unfeature | hide | unhide | edit
    reason     TEXT,
    -- Снимок изменённых полей: «было/стало» одной строкой, чтобы разбор не
    -- требовал реконструкции из соседних записей.
    changes    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_afisha_curation_log_event
    ON afisha_curation_log (event_id, created_at DESC);

COMMENT ON TABLE afisha_curation_log IS
    'Кто, когда и почему закрепил/скрыл/поправил карточку. Пишется каждым кураторским действием, читается при вопросе «почему это так».';
