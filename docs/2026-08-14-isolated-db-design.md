# Афиша: своя изолированная БД

**Решение фаундера 14.08:** у афиши должна быть полностью своя база, изолированная
от БД приложения Вшаге. Руки развязаны.

**Почему это правильно, а не просто «удобно».** Разведка 14.08 нашла три блокера
импорта, и все три — следствие того, что афиша живёт в чужой таблице:

1. схемы `events` на DEV и PROD разошлись (на PROD есть `address`/`latitude`/
   `longitude`/`visibility` и `start_time NOT NULL`, на DEV их нет) — импорт,
   отлаженный на DEV, на PROD упал бы;
2. внешнего идентификатора для дедупа нет ни в одной миграции — повторный краул
   удвоил бы афишу;
3. привязки события к месту не существует: место — текст в `varchar(200)`, а
   координаты на PROD принимаются в теле запроса и **молча выбрасываются** до SQL
   (на чтении возвращается литеральный `NULL::double precision`).

Своя база убирает все три разом: мы не достраиваем чужое, а проектируем под
задачу. Плюс снимается риск, которого никто не заказывал, — краулер, пишущий в
боевую базу приложения с живыми юзерами.

## Что уже есть (и за что держаться)

`vshage-afisha/backend` уже почти изолирован: конфиг берёт **один**
`DATABASE_URL` (`internal/config/config.go`), и у него **свои миграции** —
`afisha_featured`, `afisha_event_views`, `afisha_shares`, `afisha_event_photos`
(`pkg/db/migrations/001_init.sql`, `002_event_photos.sql`). Единственная нить,
которой он привязан к общей базе, — внешние ключи на `events(id)`.

То есть изоляция — это смена одной переменной окружения плюс перенос владения
таблицей событий к себе. Не переписывание сервиса.

## Схема

```sql
-- Источники: чтобы у каждой строки был паспорт происхождения.
CREATE TABLE sources (
    id          SMALLSERIAL PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,        -- 'kudago' | 'tg:vzmoscow' | 'site:garagemca'
    title       TEXT NOT NULL,
    kind        TEXT NOT NULL,               -- 'api' | 'html' | 'tg' | 'manual'
    license     TEXT,                        -- что можно показывать и при каких условиях
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    last_run_at TIMESTAMPTZ,
    last_ok     BOOLEAN
);

-- Площадки афиши — СВОИ. Связь с гео-корпусом опциональна: замер 14.08 показал,
-- что автоматически привязывается 44.6% событий, и это нормально.
CREATE TABLE venues (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title          TEXT NOT NULL,
    address        TEXT,
    lat            DOUBLE PRECISION,
    lon            DOUBLE PRECISION,
    venue_vid      TEXT,            -- vid из гео-корпуса, NULL пока не склеено
    vid_confidence REAL,            -- 0..1 — насколько уверен матчер
    vid_method     TEXT,            -- 'name+coord' | 'manual' | 'source_id'
    site_url       TEXT,
    tg_channel     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON venues (venue_vid) WHERE venue_vid IS NOT NULL;
CREATE INDEX ON venues (lat, lon);

CREATE TABLE events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id     SMALLINT NOT NULL REFERENCES sources(id),
    external_id   TEXT NOT NULL,               -- id события В ИСТОЧНИКЕ
    content_hash  TEXT NOT NULL,               -- чтобы ночной краул трогал только изменённое
    title         TEXT NOT NULL,
    description   TEXT,
    short_description TEXT,
    category      TEXT,                        -- наша рубрика, не чужая
    tags          JSONB NOT NULL DEFAULT '[]',
    venue_id      UUID REFERENCES venues(id),
    price_type    TEXT NOT NULL DEFAULT 'unknown'
                  CHECK (price_type IN ('free','paid','donation','unknown')),
    price_min     INTEGER,
    price_max     INTEGER,
    currency      TEXT NOT NULL DEFAULT 'RUB',
    age_limit     SMALLINT,
    url           TEXT,                        -- страница события в источнике
    image_url     TEXT,
    status        TEXT NOT NULL DEFAULT 'published'
                  CHECK (status IN ('draft','published','hidden','expired')),
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_id, external_id)            -- ДЕДУП ПО КОНСТРУКЦИИ
);

-- Время отдельной таблицей: выставка идёт месяц, концерт — один вечер, а
-- планировщику нужен вопрос «что идёт в четверг в 19:00 рядом».
CREATE TABLE event_slots (
    id         BIGSERIAL PRIMARY KEY,
    event_id   UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    starts_at  TIMESTAMPTZ NOT NULL,
    ends_at    TIMESTAMPTZ,
    all_day    BOOLEAN NOT NULL DEFAULT FALSE   -- «работает в часы площадки»
);
CREATE INDEX ON event_slots (starts_at, ends_at);
CREATE INDEX ON event_slots (event_id);
```

Существующие таблицы афиши (`afisha_featured`, `afisha_event_photos`,
`afisha_event_views`, `afisha_shares`) переезжают как есть — их FK начинают
указывать на локальную `events(id)`, и это единственная правка в них.

## Три решения, которые стоит объяснить

**`UNIQUE (source_id, external_id)` — дедуп не кодом, а схемой.** Импорт делает
`INSERT … ON CONFLICT (source_id, external_id) DO UPDATE` и не может размножить
событие, даже если краулер запустят десять раз подряд. `content_hash` рядом
отвечает на другой вопрос — «изменилось ли», чтобы ночной проход не переписывал
всю таблицу ради двух правок.

**`venue_vid` nullable с полем «как склеено».** Замер 14.08: события садятся на
наш корпус в 44.6% случаев по правилу «≤250 м и пересечение названий». Раньше
я намерил 74% по правилу «≤150 м» — и это оказалось артефактом плотности центра
(«Театр ОДЕОН» матчился на «Subjoy» в двадцати метрах). Поэтому связь —
обогащение с записанной уверенностью и методом, а не условие существования
события. Событие без vid живёт и показывается; событие с vid дополнительно
наследует всё, что мы знаем о месте.

**Слоты отдельной таблицей.** Одна выставка = одно событие + один длинный слот;
цикл кинопоказов = одно событие + десять коротких. Без этого «что идёт в
четверг вечером» превращается в перебор строк с разбором текста — ровно та
задача, которую в среду пришлось делать руками.

## Как переезжаем

1. Отдельная БД `vshage_afisha` в том же инстансе Postgres, свой пользователь,
   свой пароль. Общая база остаётся приложению; краулер к ней не подходит.
2. Миграции 003+ в `vshage-afisha/backend/pkg/db/migrations/` создают схему
   выше; `afisha_*` FK перенаправляются на локальную `events`.
3. `DATABASE_URL` афиши в `.env.afisha` переключается на новую БД
   (compose уже читает env-файл — правка одной строки).
4. 33 события с DEV и 0 с PROD переносятся скриптом как источник
   `manual`/`organizer` — истории терять не надо, объём смешной.
5. События организаторов (через `POST /organizer/events`) продолжают жить в
   приложении и **зеркалятся** в афишу односторонним фидом с
   `source='organizer'` и `external_id=<uuid события>`. Одностороннее зеркало,
   а не общая таблица: приложение и афиша больше не связаны схемой.

## Что это меняет в смете

Прошлая оценка «минимальный срез 28–38 ч» считалась для достройки чужой таблицы
с миграцией на двух разъехавшихся базах. Со своей БД пропадает вся возня со
схемным дрейфом и с organizer-JWT для импорта (пишем прямо в свою базу), но
добавляется переезд: новая БД, миграции, перенаправление FK, зеркало
организаторских событий.

Итого сопоставимо — **26–34 часа** на минимальный срез, но результат чище: у
афиши появляется собственная модель предметной области, а не арендованная.
