#!/usr/bin/env bash
set -euo pipefail

# Generates a very large random diff to trigger summarization logic (>16k chars)
# and runs aic in mock mode (no API key needed) unless REAL=1 is set.
# When REAL=1 and OPENAI_API_KEY is present, it will exercise real summarization + suggestions.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Prefer repo-local build for host platform, then ensure `aic` exists
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
  echo "Building binary..." >&2
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

# Create / modify a large dummy file
LARGE_FILE="large_dummy_test.txt"
# If file exists remove to ensure clean staged diff
rm -f "$LARGE_FILE"

# Target size ~50 KB ( > 16k )
TARGET_KB=50
LINES=$(( TARGET_KB * 64 )) # approx 50KB assuming ~16 chars avg per line; we randomize lengths.

# Generate random-ish content deterministically for repeatability
python3 - <<'PY'
import random, string, os
random.seed(42)
lines = []
N = int(os.environ.get('LINES','3200'))
for i in range(N):
    ln_len = random.randint(20,90)
    txt = ''.join(random.choice(string.ascii_letters+string.digits+" _-") for _ in range(ln_len))
    lines.append(f"line_{i:05d}: {txt}")
open('large_dummy_test.txt','w').write('\n'.join(lines)+"\n")
PY

git add "$LARGE_FILE"

# Run aic; we only need to confirm it does not error and (in real mode) triggers summary.
# Use NON_INTERACTIVE=1 to auto-select first suggestion, skip commit.

if [[ "${REAL:-}" == "1" ]]; then
  if [[ -z "${OPENAI_API_KEY:-}" ]]; then
    echo "REAL=1 but OPENAI_API_KEY not set" >&2; exit 1
  fi
  echo "Running real summarization test (this will call API)..." >&2
  AIC_NON_INTERACTIVE=1 AIC_DEBUG=1 OPENAI_API_KEY="$OPENAI_API_KEY" aic -s "Test large diff summarization" || {
    echo "Large diff summarization test FAILED" >&2; exit 1; }
else
  echo "Running mock summarization test (AIC_MOCK=1)..." >&2
  AIC_NON_INTERACTIVE=1 AIC_DEBUG=1 aic -s "Mock large diff summarization" || {
    echo "Mock large diff summarization test FAILED" >&2; exit 1; }
fi

echo "Large diff summarization script completed successfully." >&2
