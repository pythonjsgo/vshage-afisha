-- An announcement may not specify whether attendance is free or paid.
-- Public board and native-feed readers already handle NULL as unknown.
-- Keep the default for legacy organizer clients, but allow an explicit NULL
-- instead of publishing an invented promise of free admission.
ALTER TABLE organizer_event_details ALTER COLUMN price_type DROP NOT NULL;
