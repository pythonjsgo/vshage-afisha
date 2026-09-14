-- Administrative opt-in for retrospective/demo registration. Ordinary
-- organizer create/update inputs do not expose this override.
ALTER TABLE organizer_event_details
    ADD COLUMN IF NOT EXISTS allow_past_registration boolean;
