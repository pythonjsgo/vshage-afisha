-- Configurable signup form for a board event.
--
-- The board's registration was two hardcoded fields, name and contact. A venue
-- with a pass desk needs more: ФИО exactly as in the passport, a date of birth,
-- and whether a pass is needed at all. Those questions differ per event, so
-- they live with the event rather than in the page's markup.
--
-- Both blocks are additive and default to the shape that already exists:
-- an event that has no configuration decodes to the legacy two-field form
-- (regform.LegacyForm), so a live event's form does not change under the
-- people filling it in. No existing row is rewritten.

ALTER TABLE organizer_event_details
    ADD COLUMN IF NOT EXISTS reg_form   JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS reg_fields JSONB NOT NULL DEFAULT '[]'::jsonb;

-- What the visitor actually typed. signup_name / signup_contact (migration 009)
-- stay as they are — they carry the display name and the contact we dedupe on,
-- and every existing reader keeps working. The columns below only add what the
-- old two-field form could not ask for.
ALTER TABLE event_registrations
    ADD COLUMN IF NOT EXISTS signup_full_name TEXT,
    ADD COLUMN IF NOT EXISTS signup_email     TEXT,
    ADD COLUMN IF NOT EXISTS signup_phone     TEXT,
    ADD COLUMN IF NOT EXISTS signup_tg        TEXT,
    ADD COLUMN IF NOT EXISTS signup_answers   JSONB;
