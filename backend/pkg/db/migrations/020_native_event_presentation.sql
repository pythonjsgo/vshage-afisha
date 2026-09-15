-- Optional presentation facts for native events. Existing panel saves leave
-- these columns intact; deleting the event cascades through its details row.
ALTER TABLE organizer_event_details
    ADD COLUMN IF NOT EXISTS start_time_known boolean,
    ADD COLUMN IF NOT EXISTS cover_fit text CHECK (cover_fit IN ('cover', 'contain')),
    ADD COLUMN IF NOT EXISTS organizer_url text;
