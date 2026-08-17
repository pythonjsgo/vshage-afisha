-- 004_webreg_forms.sql — organizer-configurable registration form, tickets,
-- and the two independent publish switches (founder directive 2026-08-17).
--
-- Three shifts land here:
--
--   1. The signup form stops being hardcoded. `form` holds a per-field
--      {enabled, required} toggle for the built-in identity block (имя,
--      ФИО как в паспорте, почта, телефон, телеграм, вуз/статус). An event
--      row written before this migration has form = '{}', which the Go layer
--      reads as "the legacy shape" — name+telegram+affiliation, all
--      required, no email. That mapping is deliberate: the ШАГ event is live
--      while this ships, and its form must not change under its visitors.
--
--   2. Email becomes the identity that survives. Telegram is now optional,
--      so it can no longer be the dedup key — `dedup_key` takes over and is
--      filled by the application from whichever field the event marks as its
--      identity (email first, telegram second). Backfilled from tg_username
--      so existing rows keep their idempotency guarantee.
--
--   3. Publishing splits in two. An event can appear on the афиша board, in
--      the Вшаге app, in both, or in neither — the registration page at
--      /e/<slug> is reachable by link regardless. Both default TRUE so
--      events created before this keep showing where they already showed.

ALTER TABLE webreg_events
    ADD COLUMN IF NOT EXISTS form           JSONB   NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS publish_afisha BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS publish_vshage BOOLEAN NOT NULL DEFAULT TRUE,
    -- 'qr' | 'code' | 'off' — what the visitor gets as an entry pass.
    ADD COLUMN IF NOT EXISTS ticket_mode    TEXT    NOT NULL DEFAULT 'qr';

-- The column default is what a NEW event gets. Events that already exist get
-- 'off': the ШАГ event is running while this ships, and handing its visitors
-- an entry code its organizer is not expecting to check at the door is a
-- change to a live event, not a feature. Same reasoning as the legacy form.
UPDATE webreg_events SET ticket_mode = 'off';

ALTER TABLE webreg_registrations
    ADD COLUMN IF NOT EXISTS email         TEXT,
    ADD COLUMN IF NOT EXISTS phone         TEXT,
    -- Passport-form full name, kept apart from the display name: a building
    -- pass list is checked against the document, not against «Саша».
    ADD COLUMN IF NOT EXISTS full_name     TEXT,
    ADD COLUMN IF NOT EXISTS ticket_code   TEXT,
    ADD COLUMN IF NOT EXISTS checked_in_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS dedup_key     TEXT;

-- Telegram is optional from here on; the NOT NULL would reject those signups.
ALTER TABLE webreg_registrations ALTER COLUMN tg_username DROP NOT NULL;
ALTER TABLE webreg_registrations ALTER COLUMN tg_display  DROP NOT NULL;
ALTER TABLE webreg_registrations ALTER COLUMN affiliation DROP NOT NULL;

-- Backfill before the new unique index exists, or the index build fails on
-- the NULLs. Postgres treats NULLs as distinct in a unique index, which would
-- silently turn every legacy row into "not deduped".
UPDATE webreg_registrations SET dedup_key = tg_username WHERE dedup_key IS NULL;

DROP INDEX IF EXISTS idx_webreg_reg_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_webreg_reg_dedup
    ON webreg_registrations (event_slug, dedup_key);

-- A ticket code is shown at the door, so a collision inside one event would
-- admit the wrong person. Partial index: rows from before tickets existed,
-- and events with ticket_mode='off', carry NULL and are exempt.
CREATE UNIQUE INDEX IF NOT EXISTS idx_webreg_reg_ticket
    ON webreg_registrations (event_slug, ticket_code)
    WHERE ticket_code IS NOT NULL;
