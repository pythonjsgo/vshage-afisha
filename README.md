# vshage-afisha

Public event board for Вшаге — https://afisha.vshage.app

## Stack
- Frontend: SvelteKit 2 + Svelte 5 (adapter-node)
- Backend: Go 1.23 + chi + pgx/v5
- Infra: Docker on s1, shared Postgres/Redis with core-api

## Quickstart
```bash
# Backend
cd backend && go run ./cmd/server
# Frontend (separate terminal)
cd frontend && npm install && npm run dev
```

See `docs/runbook.md` for deploy.
