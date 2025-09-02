#!/usr/bin/env bash
set -euo pipefail

# Generates a very large random diff to trigger summarization logic (>16k chars)
# and runs aic in mock mode (no API key needed) unless REAL=1 is set.
# When REAL=1 and OPENAI_API_KEY is present, it will exercise real summarization + suggestions.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$ROOT_DIR/dist/aic"
if [[ ! -x "$BIN" ]]; then
  echo "Building binary..." >&2
  "$ROOT_DIR/scripts/build.sh" >/dev/null
fi

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
  AIC_NON_INTERACTIVE=1 AIC_DEBUG_SUMMARY=1 OPENAI_API_KEY="$OPENAI_API_KEY" "$BIN" -s "Test large diff summarization" || {
    echo "Large diff summarization test FAILED" >&2; exit 1; }
else
  echo "Running mock summarization test (AIC_MOCK=1)..." >&2
  AIC_MOCK=1 AIC_NON_INTERACTIVE=1 AIC_DEBUG_SUMMARY=1 OPENAI_API_KEY="sk-mock" "$BIN" -s "Mock large diff summarization" || {
    echo "Mock large diff summarization test FAILED" >&2; exit 1; }
fi

echo "Large diff summarization script completed successfully." >&2
