---
name: public-api-test
description: Curl the public afisha API endpoints with visibility filters and registration flow. Use to verify backend changes, smoke-test after deploy, or reproduce a user-reported "I don't see my event" / "registration didn't work" issue.
allowed-tools: Bash(curl:*) Bash(jq:*)
argument-hint: [base-url]
---

# /public-api-test — smoke the public afisha API

Argument: `$1` — base URL. Default: `https://afisha.vshage.app`.

## Run

```!
BASE="${0:-https://afisha.vshage.app}"
echo "=== smoking $BASE ==="

# 1. Frontend reachable
curl -fsS -o /dev/null -w 'frontend / : HTTP %{http_code} in %{time_total}s\n' "$BASE/"

# 2. Public events list (default visibility)
echo "--- /api/events (default) ---"
curl -fsS "$BASE/api/events" | jq '{count: length, ids: [.[].id][0:5]}'

# 3. Visibility filters (if backend supports query params)
for vis in public unlisted private; do
  printf '%-10s ' "$vis:"
  curl -fsS "$BASE/api/events?visibility=$vis" | jq -r 'length' || echo "n/a"
done

# 4. Single event detail (uses first id)
EID=$(curl -fsS "$BASE/api/events" | jq -r '.[0].id // empty')
if [ -n "$EID" ]; then
  echo "--- /api/events/$EID ---"
  curl -fsS "$BASE/api/events/$EID" | jq '{id, title, visibility, capacity, registered_count}'
fi

# 5. OG image render (sanity — no validation, just HTTP)
if [ -n "$EID" ]; then
  curl -fsS -o /dev/null -w 'og image : HTTP %{http_code} in %{time_total}s\n' \
    "$BASE/api/og/event/$EID" 2>/dev/null || echo "og endpoint missing or differs"
fi
```

## What to flag

- `length: 0` for default `/api/events` on PROD when you know there are public
  events → backend is filtering everything (likely a JWT/grant issue or
  missing visibility=public default)
- HTTP 5xx on detail with a valid id → backend bug, check
  `docker compose logs --tail=200 afisha-backend`
- HTTP 200 but JSON missing `registered_count` → contract drift, openapi out of sync
- Slow response (> 2s) → DB query missing an index, or N+1

## Don't

- Don't run this against PROD with `-X POST /api/events/<id>/register` to test
  registration flow — that creates a real `event_registrations` row visible to
  organizers. Use s2 (DEV) for register-flow tests.
