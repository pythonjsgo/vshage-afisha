-- 002_event_photos.sql — gallery support for afisha events

CREATE TABLE IF NOT EXISTS afisha_event_photos (
    id         BIGSERIAL PRIMARY KEY,
    event_id   UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    url        TEXT NOT NULL,
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_afisha_photos_event
    ON afisha_event_photos (event_id, position ASC);
