# Public board reads

List and facet handlers capture one clock and pass it to every source.
NotEndedSQL defines eligibility for timestamped main/webreg events: explicit
end > now, or unknown end with a start on/after today's Moscow boundary.
Calendar-only external events use their final date. Never restore the old
24-hour grace window or treat pins as an exemption from eligibility.

Filtering precedes ordering, pagination and counts. Preserve editorial
priority from the integrated release. GetByID uses direct-link visibility and
deliberately has no current-board time filter. CacheKey has an active-version
namespace and a Moscow day, including for unfiltered lists.

See docs/active-events.md and the real PostgreSQL expiry_sql_test.go cases.
