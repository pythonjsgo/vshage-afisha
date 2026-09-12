# Event structured data: explicit source facts

Search Console's five reported properties are recommended fields. Missing
values do not by themselves make an Event ineligible for rich results.
Google reference: https://developers.google.com/search/docs/appearance/structured-data/event

The former adapter discarded numeric `payload.price.amount`, closing times
and the organizer's native profile URL. The adapter now preserves those
facts; source dates retain their precision. `end_date` is a date or an
ISO timestamp, separate from `end_time`'s legacy sorting boundary. A supplied
same-day end date is kept. An earlier end clock without an explicit next-day
date is omitted, not turned into an assumed overnight event.

Native organizer URLs refer to the actual MyVshage profile. External URLs
require explicit `payload.org.url`; the announcement's `source_url` and its
publisher channel are not necessarily the event's organizer.

The importer also accepts explicit `payload.performers` entries with `type`
(Person or PerformingGroup) and `name`, and an ISO `payload.registration.valid_from`.
They are optional. The collector currently does not provide these facts;
`mentions` is a count of reposts, not a list of performers. Imported/extracted
and publication timestamps are not evidence of a ticket sale opening.
Do not fabricate participants or validFrom to clear a warning.

Offer prices use numeric source data (integer API contract), with explicit
currency; complete ruble price literals such as "600 рублей" are also preserved, while ranges/discounts/prose are rejected. No zero default
for unknown prices. Offer URLs use a valid registration URL or the page's
visible external action. Unknown price continues to omit offers.

The event page exposes the same organizer link, exact end date/time and
explicit performer/sale information to readers. Original event descriptions,
registration forms, curation, ranking and time eligibility remain unchanged.

Compatibility: old clients ignore four added optional fields. No migrations
or event-row changes; existing sorting dates are untouched. Tests cover SQL
projection, absent/invalid facts, legacy schema, real source amounts, unknown
prices, safe URLs and JSON-LD. PostgreSQL tests use a disposable database only.
Search Console itself requires a subsequent Google crawl; local JSON-LD
checks do not constitute a Google revalidation or guarantee a rich result.
