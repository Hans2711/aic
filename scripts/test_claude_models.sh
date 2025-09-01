#!/usr/bin/env bash
set -euo pipefail

# Basic test harness for Claude models. Adjust MODELS list via env or default.
: "${MODELS:=claude-3-sonnet-20240229 claude-3-opus-20240229}"
: "${AIC_SUGGESTIONS:=2}"

if [[ -z "${CLAUDE_API_KEY:-}" ]]; then
  echo "CLAUDE_API_KEY not set" >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$ROOT_DIR/dist/aic"
if [[ ! -x "$BIN" ]]; then
  echo "Binary not built. Building..." >&2
  "$ROOT_DIR/scripts/build.sh"
fi

ec=0
TEST_FILE=".aic_model_test.txt"
cleanup() {
  git reset -q HEAD -- "$TEST_FILE" 2>/dev/null || true
  rm -f "$TEST_FILE"
}
trap cleanup EXIT

# Seed file so first iteration has content
echo "AIC test seed" > "$TEST_FILE"
git add "$TEST_FILE" 2>/dev/null || true

for m in $MODELS; do
  echo "Testing model: $m"
  echo "model $m change $(date +%s)" >> "$TEST_FILE"
  git add "$TEST_FILE" 2>/dev/null || true
  if ! (AIC_PROVIDER=claude AIC_MODEL="$m" AIC_SUGGESTIONS="$AIC_SUGGESTIONS" CLAUDE_API_KEY="$CLAUDE_API_KEY" "$BIN" <<< $'1\n n\n'); then
    echo "Model $m test failed" >&2
    ec=1
  else
    echo "Model $m test passed"
  fi
  echo
  sleep 1

done
exit $ec
