---
name: afisha-deploy
description: Manually deploy afisha-frontend or afisha-backend to s2 (DEV) when CI isn't an option. CI normally handles dev pushes via the cascaded deploy-dev.yml workflow. Use this for hotfix or when you've built an image locally.
allowed-tools: Bash(../scripts/deploy-dev.sh:*) Bash(../scripts/health.sh:*) Bash(curl:*) Bash(ssh:*)
argument-hint: <frontend|backend> <tag>
---

# /afisha-deploy — bump a service tag on s2

Arguments:
- `$1` — `frontend` or `backend`
- `$2` — image tag already on `ghcr.io/pythonjsgo/afisha-{frontend,backend}:<tag>`

| Arg      | Service          | Env var on s2          | Healthcheck |
|----------|------------------|------------------------|-------------|
| frontend | afisha-frontend  | `AFISHA_FRONTEND_TAG`  | `https://afisha.vshage.app/` (HTTP 200) |
| backend  | afisha-backend   | `AFISHA_BACKEND_TAG`   | `https://afisha.vshage.app/api/events` (JSON) |

## Run

```!
SVC="$0"
TAG="$1"
case "$SVC" in
  frontend) SERVICE=afisha-frontend ;;
  backend)  SERVICE=afisha-backend ;;
  *) echo "first arg must be 'frontend' or 'backend'"; exit 1 ;;
esac
../scripts/deploy-dev.sh "$SERVICE" "$TAG"
```

## After deploy

```!
case "$0" in
  frontend) curl -fsS "https://afisha.vshage.app/" -o /dev/null -w 'HTTP %{http_code} in %{time_total}s\n' ;;
  backend)  curl -fsS "https://afisha.vshage.app/api/events" | jq 'length' ;;
esac
```

If the check fails, **don't pretend it's done** — name the failing endpoint
and propose a next step (logs via `docker compose logs --tail=200`).

## When NOT to use this

- Normal dev pushes: just `git push origin dev` — `backend-ci` + `frontend-ci`
  build, then `deploy-dev.yml` cascades automatically.
- For PROD use the `deploy-prod` skill, never this one.
