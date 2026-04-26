---
name: deploy-prod
description: Deploy afisha-frontend or afisha-backend to s1 (PROD). ALWAYS asks the user for explicit confirmation first. Requires VSHAGE_PROD_OK=yes; this skill enforces that gate. Use only after a successful DEV run + smoke test.
allowed-tools: Bash(../scripts/deploy-prod.sh:*) Bash(curl:*) Bash(ssh:*)
argument-hint: <frontend|backend> <tag>
---

# /deploy-prod — push afisha tag to s1 (PROD — interactive)

Arguments:
- `$1` — `frontend` or `backend`
- `$2` — tag already on ghcr.io and verified on s2

## Pre-flight (MANDATORY — do not skip)

Confirm with the user:

```
About to deploy afisha-{frontend|backend} tag <X> to PROD.
- Image on ghcr.io: yes/no?
- Verified on s2 (https://afisha.vshage.app already serving the new tag)?
- User-impacting risk: <one line>?
- Proceed (y/N)?
```

No yes from user in this turn → **stop**. PROD never deploys autonomously.

## Run (only after explicit yes)

```!
SVC="$0"
TAG="$1"
case "$SVC" in
  frontend) SERVICE=afisha-frontend ;;
  backend)  SERVICE=afisha-backend ;;
  *) echo "first arg must be 'frontend' or 'backend'"; exit 1 ;;
esac
VSHAGE_PROD_OK=yes ../scripts/deploy-prod.sh "$SERVICE" "$TAG"
```

## After deploy

```!
case "$0" in
  frontend) curl -fsS https://afisha.vshage.app/ -o /dev/null -w 'HTTP %{http_code} in %{time_total}s\n' ;;
  backend)  curl -fsS https://afisha.vshage.app/api/events | jq 'length' ;;
esac
```

If a health check fails, the user impact is real. Tell the user immediately —
do not silently retry. Roll back via the same skill with the previous tag.

## Hard rules

- Never deploy a tag that wasn't first run on s2 successfully
- Never deploy during the user's stated freeze windows
- Never combine PROD deploy with other risky actions in the same turn
