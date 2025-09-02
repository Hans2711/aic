#!/usr/bin/env bash
set -euo pipefail

# Package helper to create simple .zip archives for platform binaries
# Currently targets Windows exes built by scripts/build.sh

APP_NAME="aic"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="$REPO_ROOT/dist"

zip_tool() {
  if command -v zip >/dev/null 2>&1; then echo zip; return; fi
  echo ""
}

ZIP_CMD=$(zip_tool)
if [ -z "$ZIP_CMD" ]; then
  echo "zip not found; skipping zip packaging" >&2
  exit 0
fi

# Windows targets: (goos arch relpath)
for spec in \
  "windows amd64 windows/${APP_NAME}.exe" \
  "windows arm64 windows-arm64/${APP_NAME}.exe" \
; do
  set -- $spec
  goos="$1"; arch="$2"; bin_rel="$3"
  bin="$DIST_DIR/$bin_rel"
  if [ ! -f "$bin" ]; then
    echo "Skipping: $bin not found" >&2
    continue
  fi
  case "$arch" in
    amd64) zipname="${APP_NAME}_windows_amd64.zip" ;;
    arm64) zipname="${APP_NAME}_windows_arm64.zip" ;;
    *) zipname="${APP_NAME}_${goos}_${arch}.zip" ;;
  esac
  ( cd "$DIST_DIR" && $ZIP_CMD -j "$zipname" "$bin_rel" >/dev/null )
  echo "Built $DIST_DIR/$zipname"
done

exit 0
