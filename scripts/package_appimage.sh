#!/usr/bin/env bash
set -euo pipefail

# Build AppImage packages for aic using the linux binaries in dist/
# Outputs to dist/aic_linux_amd64.AppImage and dist/aic_linux_arm64.AppImage

APP_NAME="aic"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="$REPO_ROOT/dist"
TOOLS_DIR="$DIST_DIR/tools"
WORK_DIR="$DIST_DIR/appimage"

mkdir -p "$TOOLS_DIR" "$WORK_DIR"

# Resolve version (optional; used for metadata only)
VERSION="${VERSION:-}"
if [ -z "${VERSION}" ] && [ -f "$REPO_ROOT/internal/version/version.go" ]; then
  VERSION=$(awk -F '"' '/const Version/ {print $2}' "$REPO_ROOT/internal/version/version.go" || true)
fi

ensure_appimagetool() {
  local tool="$TOOLS_DIR/appimagetool-x86_64.AppImage"
  if [ ! -f "$tool" ]; then
    echo "Downloading appimagetool..." >&2
    curl -fsSL -o "$tool" \
      https://github.com/AppImage/AppImageKit/releases/download/continuous/appimagetool-x86_64.AppImage
    chmod +x "$tool"
  fi
  echo "$tool"
}

make_icon_png() {
  # Write a tiny 256x256 black PNG as a placeholder icon
  # (This keeps the repo clean; only generated at packaging time.)
  local out_png="$1"
  # Base64-encoded 1x1 black PNG scaled by viewers; still acceptable placeholder.
  # iVBORw0... is a standard minimal PNG.
  cat >"$out_png.b64" <<'EOF'
iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Uo0M1kAAAAASUVORK5CYII=
EOF
  base64 --decode "$out_png.b64" > "$out_png"
  rm -f "$out_png.b64"
}

build_one() {
  local arch_go="$1" arch_ai="$2" binpath="$3"

  if [ ! -f "$binpath" ]; then
    echo "Skipping $arch_go: binary not found at $binpath" >&2
    return 0
  fi

  local appdir="$WORK_DIR/${APP_NAME}.AppDir"
  rm -rf "$appdir"
  mkdir -p "$appdir/usr/bin" "$appdir/usr/share/applications" "$appdir/usr/share/icons/hicolor/256x256/apps"

  # AppRun: exec the bundled binary, pass through args
  cat > "$appdir/AppRun" <<'EOF'
#!/bin/sh
HERE="$(dirname "$(readlink -f "$0")")"
exec "$HERE/usr/bin/aic" "$@"
EOF
  chmod +x "$appdir/AppRun"

  # Desktop file for integration (Terminal=true for CLI)
  cat > "$appdir/${APP_NAME}.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=aic
Comment=AI-assisted git commit message generator
Exec=aic
Terminal=true
Categories=Utility;Development;
Icon=aic
X-AppImage-Name=aic
X-AppImage-Version=${VERSION:-}
EOF

  # Icon
  local icon_png_root="$appdir/aic.png"
  make_icon_png "$icon_png_root"
  # Also place under conventional icon dir
  install -m 0644 "$icon_png_root" "$appdir/usr/share/icons/hicolor/256x256/apps/aic.png"

  # Binary
  install -m 0755 "$binpath" "$appdir/usr/bin/${APP_NAME}"

  # Build AppImage
  local appimagetool
  appimagetool=$(ensure_appimagetool)
  local outname="$DIST_DIR/${APP_NAME}_linux_${arch_go}.AppImage"
  # Use extract-and-run to avoid FUSE dependency on CI
  APPIMAGE_EXTRACT_AND_RUN=1 ARCH="$arch_ai" "$appimagetool" "$appdir" "$outname" >/dev/null
  echo "Built $outname"
}

# Map our dist outputs to AppImage arch names
BIN_AMD64="$DIST_DIR/ubuntu/${APP_NAME}"
BIN_ARM64="$DIST_DIR/ubuntu-arm64/${APP_NAME}"

ret=0

# amd64 → x86_64
build_one amd64 x86_64 "$BIN_AMD64" || ret=1
# arm64 → aarch64
build_one arm64 aarch64 "$BIN_ARM64" || ret=1

exit $ret

