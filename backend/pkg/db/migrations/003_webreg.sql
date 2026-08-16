-- 003_webreg.sql — standalone web registration for vshage.app/e/<slug>
--
-- Deliberately NOT built on the shared `events` / `event_registrations`
-- tables. Two reasons, both measured:
--   1. event_registrations.profile_id is NOT NULL with an FK to profiles —
--      a web signup without an account cannot be stored there without
--      minting junk profiles inside the live app database.
--   2. the shared `events` schema has drifted between DEV and PROD
--      (docs/2026-08-14-isolated-db-design.md) — anything built on it would
--      pass on DEV and fail on PROD.
-- These tables own their data end to end and carry zero foreign keys into
-- app-owned tables, so they move with afisha when it gets its own database.

CREATE TABLE IF NOT EXISTS webreg_events (
    slug              TEXT PRIMARY KEY,
    title             TEXT NOT NULL,
    tagline           TEXT,
    description       TEXT,
    cover_url         TEXT,
    starts_at         TIMESTAMPTZ NOT NULL,
    ends_at           TIMESTAMPTZ,
    timezone          TEXT        NOT NULL DEFAULT 'Europe/Moscow',
    -- Denormalised venue card. Copied from the geo catalog at event-creation
    -- time on purpose: the page must never depend on an 11MB catalog file or
    -- a second service being up while a Telegram announcement lands.
    venue             JSONB       NOT NULL DEFAULT '{}'::jsonb,
    -- Organizer-defined extra questions, rendered in order.
    fields            JSONB       NOT NULL DEFAULT '[]'::jsonb,
    -- Options for the вуз/статус picker; "Другое" is always appended.
    affiliations      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    -- Post-registration bridge into the network (TestFlight / waitlist / TG).
    bridge            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    organizer_title   TEXT,
    capacity          INTEGER,
    registration_open BOOLEAN     NOT NULL DEFAULT TRUE,
    -- SHA-256 hex of the organizer's secret manage key. The key itself is
    -- never stored, so a database leak does not hand over the attendee list.
    manage_key_hash   TEXT        NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS webreg_registrations (
    id           BIGSERIAL PRIMARY KEY,
    event_slug   TEXT        NOT NULL REFERENCES webreg_events(slug) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    -- Normalised: lowercase, no '@', no t.me/ prefix. Dedup key.
    tg_username  TEXT        NOT NULL,
    -- Exactly what the visitor typed, for the organizer's eyes.
    tg_display   TEXT        NOT NULL,
    affiliation  TEXT        NOT NULL,
    answers      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    consent      BOOLEAN     NOT NULL DEFAULT FALSE,
    source       TEXT,
    ip_hash      TEXT,
    user_agent   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Makes a re-submit idempotent instead of a duplicate row: a visitor on a
-- flaky mobile connection who taps «Иду» twice lands in the list once.
CREATE UNIQUE INDEX IF NOT EXISTS idx_webreg_reg_unique
    ON webreg_registrations (event_slug, tg_username);

CREATE INDEX IF NOT EXISTS idx_webreg_reg_event_time
    ON webreg_registrations (event_slug, created_at DESC);

-- Android users have no app to install yet; capture them instead of losing
-- them. No FK to webreg_events — this list outlives any single event.
CREATE TABLE IF NOT EXISTS webreg_waitlist (
    id          BIGSERIAL PRIMARY KEY,
    event_slug  TEXT,
    platform    TEXT        NOT NULL,
    tg_username TEXT        NOT NULL,
    tg_display  TEXT        NOT NULL,
    name        TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_webreg_waitlist_unique
    ON webreg_waitlist (platform, tg_username);
