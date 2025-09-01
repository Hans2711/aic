#!/usr/bin/env bash
set -euo pipefail

# Basic test harness for available models. Adjust MODELS list via env or default.
: "${MODELS:=gpt-4o-mini gpt-4o gpt-5-2025-08-07}"  # add more models if desired
: "${AIC_SUGGESTIONS:=2}"          # keep it small for tests

if [[ -z "${OPENAI_API_KEY:-}" ]]; then
  echo "OPENAI_API_KEY not set" >&2
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
  # Append a unique line per model so a fresh staged diff exists each loop.
  echo "model $m change $(date +%s)" >> "$TEST_FILE"
  git add "$TEST_FILE" 2>/dev/null || true
  if ! (AIC_PROVIDER=openai AIC_MODEL="$m" AIC_SUGGESTIONS="$AIC_SUGGESTIONS" OPENAI_API_KEY="$OPENAI_API_KEY" "$BIN" <<< $'1\n n\n'); then
    echo "Model $m test failed" >&2
    ec=1
  else
    echo "Model $m test passed"
  fi
  echo
  sleep 1
done
exit $ec
