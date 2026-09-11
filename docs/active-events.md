# Current board eligibility

The public afisha used a now-minus-24-hours cutoff, so already-ended events
remained in lists, featured banners and counts. Pins amplified this by placing
an expired event above future ones. Eligibility now runs before ranking and
pagination, using one request clock for all sources.

- Explicit end timestamp: visible only while the end is later than now.
- No end timestamp: retain through the Moscow day of its start; do not invent
  a duration or assume that the start is also the finish.
- Date-only external events: keep through their declared final Moscow day.
- Ongoing multi-day/open-ended programs remain eligible. Publication, city,
  access and curation predicates still apply; a pin does not bypass them.
- Direct event links and stored records are unchanged. No automatic deletion,
  cancellation, unpinning or registration-data changes.

Lists, banners, totals and facets share the rule. The list cache namespace is
versioned and always includes the Moscow day, so deployment or midnight cannot
reuse yesterday's grace-window list. Existing minute-scale cache TTL remains.

No API fields or database schema change. Native/core feed code is not part of
this website fix. Real PostgreSQL tests cover all three stores, expired pinned
events, explicit end boundaries, ongoing events, missing ends, Moscow midnight,
facet/list agreement and direct access to a past card.
