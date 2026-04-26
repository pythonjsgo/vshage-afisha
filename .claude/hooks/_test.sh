#!/usr/bin/env bash
# Smoke tests for the afisha .claude/hooks scripts.
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$HERE/../.." && pwd)"
fails=0

run_case() {
  local name="$1" hook="$2" payload="$3" expected_exit="$4"
  local out exit_code
  out="$(CLAUDE_PROJECT_DIR="$REPO_ROOT" CLAUDE_DISABLE_SVELTE_CHECK_HOOK=1 \
    bash "$HERE/$hook" <<<"$payload" 2>&1)"
  exit_code=$?
  if [ "$exit_code" -ne "$expected_exit" ]; then
    printf '✗ %s — exit %d, expected %d\n  output: %s\n' \
      "$name" "$exit_code" "$expected_exit" "$out"
    fails=$((fails+1))
  else
    printf '✓ %s\n' "$name"
  fi
}

run_case "post-edit-svelte-check skips backend Go" "post-edit-svelte-check.sh" \
  '{"tool_input":{"file_path":"backend/main.go"}}' 0
run_case "post-edit-svelte-check skips when disabled (svelte file)" "post-edit-svelte-check.sh" \
  '{"tool_input":{"file_path":"frontend/src/lib/components/EventCard.svelte"}}' 0
run_case "post-edit-svelte-check skips when disabled (ts file)" "post-edit-svelte-check.sh" \
  '{"tool_input":{"file_path":"frontend/src/lib/types.ts"}}' 0
run_case "post-edit-svelte-check handles empty stdin" "post-edit-svelte-check.sh" '' 0
run_case "post-edit-svelte-check handles bad json" "post-edit-svelte-check.sh" 'not-json{' 0

if [ "$fails" -gt 0 ]; then
  printf '\n%d test(s) failed.\n' "$fails"
  exit 1
fi
printf '\nall hook tests passed.\n'
