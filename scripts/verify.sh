#!/usr/bin/env bash
set -euo pipefail
# Verify checksums in dist/checksums.txt. Accepts optional path override.
FILE=${1:-dist/checksums.txt}
if [ ! -f "$FILE" ]; then
  echo "Checksum file not found: $FILE" >&2
  exit 1
fi

# Determine base dir for listed files (assumes FILE is dist/checksums.txt)
BASE_DIR=$(dirname "$FILE")
pushd "$BASE_DIR" >/dev/null

# Some entries have leading ./; ensure files exist
sed 's|  \./|  |' "$(basename "$FILE")" > .checksums.tmp

if command -v sha256sum >/dev/null 2>&1; then
  sha256sum --check .checksums.tmp
elif command -v shasum >/dev/null 2>&1; then
  shasum -a 256 --check .checksums.tmp
else
  echo "No sha256sum or shasum found." >&2
  popd >/dev/null
  exit 1
fi
rm -f .checksums.tmp
popd >/dev/null
