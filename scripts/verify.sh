#!/usr/bin/env bash
set -euo pipefail
# Verify checksums in dist/checksums.txt. Accepts optional path override.
FILE=${1:-dist/checksums.txt}
if [ ! -f "$FILE" ]; then
  echo "Checksum file not found: $FILE" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum --check "$FILE"
elif command -v shasum >/dev/null 2>&1; then
  shasum -a 256 --check "$FILE"
else
  echo "No sha256sum or shasum found." >&2
  exit 1
fi
