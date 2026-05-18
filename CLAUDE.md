# vshage-afisha

> Public events board for Вшаге. SvelteKit 2 + Svelte 5 frontend, Go 1.25 backend. Self-hosted at `afisha.vshage.app`.

## Architecture

```
afisha-frontend (SvelteKit 2)  ──→  afisha-backend (Go :3004)  ──→  PostgreSQL (shared)
        │
        └─→ /api/* (SvelteKit server routes for OG images, etc. via satori)
```

- **Public**: anyone can list events, register for events. No auth required.
- **Shared DB**: reads `events` table; writes `event_registrations` row on register.
- **Restricted access**: organizer-api owns event lifecycle; afisha is read-only on most tables.
- **Domain**: `afisha.vshage.app` (separate from `s1.vshage.app/s2.vshage.app` which serves the core app + organizer).

## Tech Stack

- **Backend**: Go 1.25, chi/v5, sqlc + pgx/v5, port 3004
- **Frontend**: SvelteKit 2, Svelte 5 (runes + snippets), TypeScript, Vite, Node 22
- **OG images**: `@resvg/resvg-js` + `satori` (generated at build time, served from `/api/og/*`)
- **Tests**: Playwright for frontend e2e, Go test for backend
- **DB**: shared with core/organizer at port 5432 inside compose net

## Environments

| Env | Domain | Branch trigger |
|-----|--------|----------------|
| DEV | https://afisha.dev.vshage.app/ (Senko-dev) | `dev` (default) — CI auto-deploys |
| PROD | https://afisha.vshage.app/ — **live since 2026-05-18** | `main` (no auto-deploy pipeline yet — see umbrella) |

> PROD `.env` pins `AFISHA_*_TAG` to an exact image sha (not the moving `:dev`
> tag). Promoting a new build to PROD is currently a manual step until a
> `main`→PROD deploy workflow lands.

## CI/CD (per-repo, working today)

`.github/workflows/`:
- `backend-ci.yml` — go vet + test + docker build/push on push to dev/main + paths backend/**
- `frontend-ci.yml` — npm + svelte-check + docker build/push on push to dev/main + paths frontend/**
- `deploy-dev.yml` — workflow_run cascade: when both CIs succeed on `dev`, ssh deploy to s2

Secrets: `SSH_PRIVATE_KEY_DEV` (ssh root@s2) + GHCR token (auto via GITHUB_TOKEN with packages:write).

## Commands

```bash
# Backend
cd backend && go run ./cmd/api/        # local server :3004
cd backend && go test ./... -count=1
cd backend && docker build --platform linux/amd64 -t ghcr.io/pythonjsgo/afisha-backend:dev .

# Frontend
cd frontend && npm install && npm run dev   # Vite at :5173
cd frontend && npm run check                # svelte-check (typecheck)
cd frontend && npm run test                 # Playwright e2e
cd frontend && docker build --platform linux/amd64 -t ghcr.io/pythonjsgo/afisha-frontend:dev .

# Deploy DEV manually (CI normally handles it)
../scripts/deploy-dev.sh afisha-backend  <tag>
../scripts/deploy-dev.sh afisha-frontend <tag>

# Public API smoke
curl -s https://afisha.vshage.app/api/events | jq '.[].id' | head
```

## Repo layout

```
backend/
  cmd/api/                 # entry point
  internal/                # domain modules
  Dockerfile               # in-repo
frontend/
  src/
    lib/                   # shared components, utilities
    routes/                # SvelteKit routes
    routes/api/            # server routes (OG images, etc.)
    routes/events/         # event list + detail
  static/                  # static assets
  Dockerfile
.claude/                   # this layer (skills, agents, hooks, settings)
.github/workflows/         # per-repo CI/CD
```

## Rules

- Russian UI strings, English code/comments
- `--platform linux/amd64` on Docker builds
- Frontend: Svelte 5 runes (`$state`, `$derived`, `$effect`) — no `$:` reactive blocks
- Backend: sqlc for ALL SQL (no raw `pool.Query`)
- Public registration is rate-limited at the proxy (Caddy); don't disable
- DB grants: read-only on most tables; write only on `event_registrations`
- PR target: `dev` for normal work; `main` only when promoting to PROD

## Cross-repo references

- Compose / monitoring / runbook: umbrella `vshage-umbrella/` (`../compose/docker-compose.yml` defines `afisha-{frontend,backend}` services)
- Core app & organizer: `vshage-monorepo`, `vshage-organizer`
- Server provisioning: `vshage-infra` (terraform-only)
