# Event details and actions

EventDetail displays a wide cover, bounded title, date/venue/address and a
primary participation action before the announcement. event-actions.ts owns
destination/price semantics, event-copy.ts owns the new RU/EN UI text.

ShareSheet is separate and collapsed: its Telegram link sends the afisha
URL to friends, not to the organizer. Primary Telegram registration remains
explicitly named. Keep native /events/{id}/register and legacy /e/{slug}
forms working; imported events without registration must not gain our form.

EventCover/cover-motion still implement silent background playback and image
fallback. Runtime data-ready CSS selectors must remain :global in Svelte;
clock advancement alone is not visual verification. See docs/event-actions.md
and frontend/scripts/verify-event-actions.mjs for headless checks.
