# Imported event publication

This legacy-named module stores external announcements. source_url and
org_name preserve attribution; registration stays off-site. Import upserts
do not reset curation flags. PriceRaw is public PriceText; explicit IsFree
maps to free/paid while nil stays unknown. Never infer price_min/max from prose.
photo_url keeps the compatible image endpoint with an updated_at revision.
Keep scanCard aligned with selectCard and the disposable PostgreSQL fixtures.
See docs/event-actions.md for existing price/image action behavior.

Automatic publication is an explicit authenticated batch decision, after the
collector checks facts, audience and duplicates. AdminList exposes curation
state so the collector never guesses whether hidden means queue or rejection.
AutomaticDecisions locks each row and protects every human exclusion. Only
pipeline:auto-events may reverse its own exclusion. No-op retries do not
change timestamps or add curation records. Native/organizer events are untouched.

No schema changes. Existing admin imports keep their behavior. The collector
must require the new list fields before using /admin/decisions; an older
backend is a retryable deployment mismatch, not permission to publish blindly.
