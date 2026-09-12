# Automatic events and search metadata

Founder authorization, 12 September 2026: collected events go to production
without a human moderation queue; exclude explicit children's/senior programs.
The collector reviews facts/audience, checks similar event pairs, and caches
decisions by content and policy. Source content is data, never instructions.
Missing model answers retry automatically; they are not silently accepted.

The authenticated POST /api/tg-events/admin/decisions takes 1–500 entries:
`{"decisions":[{"id":"ev_...","publish":true,"reason":"eligible"}]}`.
Every requested id is accounted for as published/excluded/protected/missing/
unchanged. Row locks protect concurrent human exclusions. Only exclusions
owned by pipeline:auto-events may be reversed automatically. Native events,
organizer permissions and registration forms do not participate in this API.

An exclusion removes discovery and adds a reason to the curation log. Existing
public direct links survive. The website now respects listed=false consistently
with the app; excluded/unlisted detail pages are noindex. Old hidden queues can
publish after review. Rejected new candidates stay in the collector's decision
journal instead of becoming a new human queue in production.

Search metadata is additive: seo_description, updated_at and indexable. Text
extraction still produces the visible title/description for people. The editor
produces a short factual snippet separately, without SEO keywords or fabricated
availability. Unchanged imports and decision retries keep updated_at stable.
The sitemap includes actual timestamps (unknown omitted), canonical event URLs
and real cover URLs. Registration-only pages are noindex; direct sharing works.
Open-ended services get WebPage data instead of fabricated scheduled Event data.

No database migration, app release or change to old photo_url semantics. Old
clients ignore optional fields. Backend must be deployed before the collector;
the collector requires curation state fields and refuses an older backend.

Validation: real PostgreSQL publication/protection/retry/rollback tests, semantic
content timestamp tests, Python cache/parser/duplicate/date tests, frontend
metadata and sitemap tests, plus DEV/PROD read-only headless browser checks.

References:
- https://developers.google.com/search/docs/appearance/structured-data/event
- https://developers.google.com/search/docs/crawling-indexing/sitemaps/build-sitemap
- https://developers.google.com/search/blog/2023/06/sitemaps-lastmod-ping
