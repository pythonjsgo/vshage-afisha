-- 001_init.sql — afisha-backend owned tables

CREATE TABLE IF NOT EXISTS afisha_featured (
    event_id   TEXT PRIMARY KEY REFERENCES events(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL DEFAULT 100,
    pinned_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    pinned_by  TEXT
);

CREATE INDEX IF NOT EXISTS idx_afisha_featured_position
    ON afisha_featured (position ASC, pinned_at DESC);

CREATE TABLE IF NOT EXISTS afisha_event_views (
    id         BIGSERIAL PRIMARY KEY,
    event_id   TEXT NOT NULL,
    viewed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    country    TEXT,
    ua_hash    TEXT
);

CREATE INDEX IF NOT EXISTS idx_afisha_views_event
    ON afisha_event_views (event_id, viewed_at DESC);

CREATE TABLE IF NOT EXISTS afisha_shares (
    id         BIGSERIAL PRIMARY KEY,
    event_id   TEXT NOT NULL,
    shared_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    channel    TEXT NOT NULL  -- 'copy' | 'telegram' | 'whatsapp' | 'other'
);

CREATE INDEX IF NOT EXISTS idx_afisha_shares_event
    ON afisha_shares (event_id, shared_at DESC);
