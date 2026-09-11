# External event mapping and curation

This legacy-named module stores external announcements and exposes them in
the public board. source_url and org_name preserve external attribution;
registration stays off-site. Import upserts do not reset curation flags.

PriceRaw is public PriceText; explicit IsFree maps to free/paid while nil
stays unknown. No price_min/max is inferred from text. selectCard carries an
updated_at revision into photo_url, so replacement covers get a fresh URL
while /api/tg-events/{id}/cover remains the compatible image endpoint.

Keep scanCard aligned with selectCard. The PostgreSQL fixture mirrors every
selected column; run with a disposable AFISHA_TEST_DATABASE_URL. Image URLs
and pricing additions are covered by afisha_test.go. See docs/event-actions.md.
