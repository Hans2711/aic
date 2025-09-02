#!/usr/bin/env bash
set -euo pipefail

# Basic test harness for Gemini models. Adjust MODELS list via env or default.
: "${MODELS:=gemini-2.5-flash gemini-2.5-pro}"
: "${AIC_SUGGESTIONS:=2}"

if [[ -z "${GEMINI_API_KEY:-}" ]]; then
  echo "GEMINI_API_KEY not set" >&2
  exit 1
fi

# Prefer repo-local build for host platform, then ensure `aic` exists
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
HOST_DIR=""
case "$OS" in
  darwin)
    if [[ "$ARCH" == "arm64" ]]; then HOST_DIR="mac"; else HOST_DIR="mac-intel"; fi ;;
  linux)
    if [[ "$ARCH" == "aarch64" || "$ARCH" == "arm64" ]]; then HOST_DIR="ubuntu-arm64"; else HOST_DIR="ubuntu"; fi ;;
esac
if [[ -n "$HOST_DIR" && -x "$ROOT_DIR/dist/$HOST_DIR/aic" ]]; then
  PATH="$ROOT_DIR/dist/$HOST_DIR:$PATH"
fi
if ! command -v aic >/dev/null 2>&1; then
  echo "Building local aic binary..." >&2
  bash "$ROOT_DIR/scripts/build.sh" >/dev/null
  if [[ -n "$HOST_DIR" && -x "$ROOT_DIR/dist/$HOST_DIR/aic" ]]; then
    PATH="$ROOT_DIR/dist/$HOST_DIR:$PATH"
  fi
fi
if ! command -v aic >/dev/null 2>&1; then
  echo "aic binary not found on PATH. Install or build it and ensure it's available (e.g., run scripts/install.sh)." >&2
  exit 1
fi

# Sanity-check that this aic mentions Gemini support
if ! aic --help 2>/dev/null | grep -qi "GEMINI_API_KEY"; then
  echo "The 'aic' on PATH may be outdated and not support Gemini. Ensure you are using the freshly built binary for your platform in dist/." >&2
  exit 1
fi
echo "Using aic at: $(command -v aic)" >&2

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
  if ! (env -i PATH="$PATH" AIC_PROVIDER=gemini AIC_DEBUG=1 AIC_MODEL="$m" AIC_SUGGESTIONS="$AIC_SUGGESTIONS" GEMINI_API_KEY="$GEMINI_API_KEY" aic <<< $'1\n n\n'); then
    echo "Model $m test failed" >&2
    ec=1
  else
    echo "Model $m test passed"
  fi
  echo
  sleep 1

done
exit $ec
