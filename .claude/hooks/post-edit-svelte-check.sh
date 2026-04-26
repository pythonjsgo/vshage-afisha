#!/usr/bin/env bash
# PostToolUse(Edit|Write|MultiEdit) hook for afisha/frontend Svelte/TS files.
# Triggers a quick `npm run check` on the frontend project after the edit and
# warns on errors WITHOUT blocking the tool call. Skips if:
#   - the edited file isn't under frontend/
#   - svelte-check is missing (frontend/node_modules not installed)
#   - the user disabled the hook via CLAUDE_DISABLE_SVELTE_CHECK_HOOK=1
set -uo pipefail

REPO_ROOT="${CLAUDE_PROJECT_DIR:-$(pwd)}"

INPUT="$(cat)"
if [ -z "$INPUT" ]; then
  exit 0
fi

FILE_PATH="$(printf '%s' "$INPUT" | jq -r '.tool_input.file_path // empty' 2>/dev/null || true)"

case "$FILE_PATH" in
  */frontend/*.svelte|*/frontend/**/*.svelte) ;;
  */frontend/*.ts|*/frontend/**/*.ts) ;;
  *) exit 0 ;;
esac

if [ "${CLAUDE_DISABLE_SVELTE_CHECK_HOOK:-0}" = "1" ]; then
  exit 0
fi

FE_DIR="$REPO_ROOT/frontend"
[ -f "$FE_DIR/package.json" ] || exit 0
[ -d "$FE_DIR/node_modules/svelte-check" ] || exit 0

cd "$FE_DIR" || exit 0
SC_OUT="$(npm run --silent check 2>&1)"
SC_EXIT=$?
if [ "$SC_EXIT" -ne 0 ]; then
  printf '⚠️  svelte-check errors after editing %s:\n%s\n' "$FILE_PATH" "$SC_OUT" >&2
  # exit 0 — non-blocking warn. Use exit 2 to BLOCK the tool call.
fi
exit 0
