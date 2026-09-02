-- Registrations on a /e/<slug> page reach the founder's Telegram, the same way
-- registrations on an afisha event already do.
--
-- Why here and not in Go. Migration 010 gave the board's own flow an outbox and
-- a background sender (internal/events/notify.go); the sender reads whatever is
-- in `registration_notify_outbox` and knows nothing about who wrote it. The web
-- registration flow (internal/webreg) never called it, so an event whose signup
-- lives on /e/<slug> was silent — and that is precisely the flow used when the
-- form needs passport fields for a building pass. This trigger writes the same
-- row, in the same transaction as the registration, exactly as the Go path does.
-- The proper home is a call in webreg.Repository.Register; until that ships,
-- this keeps the two flows behaving alike rather than one of them being mute.
--
-- Scope is every web-registration event, matching the board flow, which is
-- global too. Nothing is retroactive: the trigger fires on INSERT only, so a
-- re-submitted form (ON CONFLICT DO UPDATE) does not send a second message.

CREATE OR REPLACE FUNCTION webreg_notify_registration() RETURNS trigger AS $$
DECLARE
    ev_title    TEXT;
    ev_start    TIMESTAMPTZ;
    ev_capacity INTEGER;
    taken       INTEGER;
    who         TEXT;
    contact     TEXT;
    seats       TEXT;
    extra       TEXT := '';
    k           TEXT;
    v           TEXT;
    lbl         TEXT;
BEGIN
    SELECT title, starts_at, capacity INTO ev_title, ev_start, ev_capacity
      FROM webreg_events WHERE slug = NEW.event_slug;
    IF ev_title IS NULL THEN
        RETURN NEW;                        -- событие исчезло: молчим, не падаем
    END IF;

    SELECT COUNT(*) INTO taken FROM webreg_registrations WHERE event_slug = NEW.event_slug;

    -- Имя: ФИО как в документе важнее короткого имени — по нему выписывают пропуск.
    who := COALESCE(NULLIF(NEW.full_name, ''), NULLIF(NEW.name, ''), '—');

    -- Контакт: всё, что человек оставил, в порядке пригодности для связи.
    contact := concat_ws(' · ',
        NULLIF(NEW.email, ''),
        NULLIF(NEW.phone, ''),
        CASE WHEN COALESCE(NEW.tg_username, '') <> '' THEN '@' || NEW.tg_username END);
    IF contact = '' OR contact IS NULL THEN contact := '—'; END IF;

    IF ev_capacity IS NULL THEN
        seats := taken::text;
    ELSE
        seats := taken::text || ' из ' || ev_capacity::text;
    END IF;

    -- Ответы на поля организатора (дата рождения, нужен ли пропуск и прочее)
    -- идут строками как есть: список полей у каждого события свой, и захардкоженный
    -- набор устареет на первом же событии с другой формой.
    IF NEW.answers IS NOT NULL THEN
        FOR k, v IN SELECT key, value FROM jsonb_each_text(NEW.answers) ORDER BY key LOOP
            IF v <> '' AND left(k, 2) <> '__' THEN
                -- Подпись поля, а не его ключ: сообщение читает человек, и
                -- «birth_date: 01.01.1990» ему приходится расшифровывать.
                -- Сброс перед SELECT обязателен — при отсутствии строки
                -- SELECT INTO оставляет прежнее значение, и подпись уехала бы
                -- на соседний ответ.
                lbl := NULL;
                SELECT f->>'label' INTO lbl
                  FROM webreg_events e, jsonb_array_elements(e.fields) f
                 WHERE e.slug = NEW.event_slug AND f->>'key' = k
                 LIMIT 1;
                extra := extra || E'\n' || COALESCE(NULLIF(lbl, ''), k) || ': ' || v;
            END IF;
        END LOOP;
    END IF;

    INSERT INTO registration_notify_outbox (payload) VALUES (
        '✍️ Новая запись — ' || ev_title ||
        E'\nИмя: ' || who ||
        E'\nКонтакт: ' || contact ||
        extra ||
        E'\nСтатус: registered' ||
        E'\nЗанято: ' || seats ||
        E'\nСобытие: ' || to_char(ev_start AT TIME ZONE 'Europe/Moscow', 'DD.MM HH24:MI') || ' МСК' ||
        E'\nЗаписан(а): ' || to_char(NEW.created_at AT TIME ZONE 'Europe/Moscow', 'DD.MM HH24:MI:SS') || ' МСК' ||
        E'\nСтраница: /e/' || NEW.event_slug ||
        E'\nID записи: webreg#' || NEW.id::text
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_webreg_notify_registration ON webreg_registrations;
CREATE TRIGGER trg_webreg_notify_registration
    AFTER INSERT ON webreg_registrations
    FOR EACH ROW EXECUTE FUNCTION webreg_notify_registration();
