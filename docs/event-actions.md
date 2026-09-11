# Event purchase, registration and sharing

The detail page has one primary attendance action before its description:
paid external website → buy ticket; Telegram organizer/bot → register via
Telegram; native/legacy /e forms → internal registration. An imported event
without a registration link can open its source, but never acquires our form.
Telegram share URLs are not registration URLs. Sharing is a collapsed,
separately named control after the description; copying reports failures.
New interface copy lives in event-copy.ts in Russian and English.

External price_raw is exposed as optional price_text, unchanged, and an
explicit is_free=false becomes price_type=paid. Unknown prices stay unknown;
numeric prices are never guessed from text. Existing typed native prices
still use price_min/max/currency. Restricted-access labels remain; the vague
open-entry pill is omitted from the detail page.

The wide cover and narrower title/readable text column prevent the original
portrait/oversized-heading presentation. Hartmann's actual card uses an
unchanged landscape photo from the organizer's first screen, the verified
program/tariffs, BIZFAKT and a ticket link to #tarif. No checkout or messages
are submitted by the verification script.

## Compatibility

- Old API clients ignore the optional price_text. Existing image fields still
  carry an image URL, not a new media type. The core/native API is unchanged.
- No migrations or bulk data rewrites. Only the requested Hartmann content
  and image are updated through the existing import API, preserving curation.
- Afisha photo URLs include the existing row's updated_at as a revision, so
  a replaced cover does not reuse the day-long cached URL. The underlying
  cover route and JPEG response remain compatible. This does not modify the
  separate native core feed's image-cache policy.
- Meaningful checks: real PostgreSQL queries, paid/free/unknown mapping,
  changed cover revision, action routing, invalid URLs, and headless browser
  purchase/share/native registration flows. Existing video covers retain
  their silent playback and no-controls policy.

Run `npm run test:unit -- --run`, `npm run check`, and `npm run build` from
frontend. Run backend tests with a disposable AFISHA_TEST_DATABASE_URL to
exercise real SQL. `frontend/scripts/verify-event-actions.mjs` supports local,
DEV and PROD origins via AFISHA_CHECK_BASE, always headlessly and read-only.
AFISHA_CHECK_BROWSER=webkit selects the second engine. The website source was
also inspected through headless Playwright: https://arena.unicornfellowship.ru/.
