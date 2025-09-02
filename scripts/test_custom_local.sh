#!/usr/bin/env bash
set -euo pipefail

# Basic test harness for a local/custom OpenAI-compatible server (e.g., LM Studio).
# Configure endpoints via env or rely on defaults.
:
: "${AIC_SUGGESTIONS:=2}"
:

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
echo "Using aic at: $(command -v aic)" >&2

TEST_FILE=".aic_model_test.txt"
cleanup() {
  git reset -q HEAD -- "$TEST_FILE" 2>/dev/null || true
  rm -f "$TEST_FILE"
}
trap cleanup EXIT

# Seed file so first iteration has content
echo "AIC test seed" > "$TEST_FILE"
git add "$TEST_FILE" 2>/dev/null || true

# Default local endpoints (override via env before calling)
: "${CUSTOM_BASE_URL:=http://127.0.0.1:1234}"
: "${CUSTOM_CHAT_COMPLETIONS_PATH:=/v1/chat/completions}"

echo "Testing CUSTOM provider against $CUSTOM_BASE_URL$CUSTOM_CHAT_COMPLETIONS_PATH"

echo "model custom change $(date +%s)" >> "$TEST_FILE"
git add "$TEST_FILE" 2>/dev/null || true

# AIC_MODEL can be omitted or set to "auto" to pick the first model from /v1/models
: "${AIC_MODEL:=auto}"

env -i PATH="$PATH" \
  AIC_PROVIDER=custom \
  AIC_MODEL="$AIC_MODEL" \
  AIC_SUGGESTIONS="$AIC_SUGGESTIONS" \
  CUSTOM_BASE_URL="$CUSTOM_BASE_URL" \
  CUSTOM_CHAT_COMPLETIONS_PATH="$CUSTOM_CHAT_COMPLETIONS_PATH" \
  aic <<< $'1\n n\n'

echo "Custom provider test completed."
