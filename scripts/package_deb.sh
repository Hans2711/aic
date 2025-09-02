#!/usr/bin/env bash
set -euo pipefail

# Build simple .deb packages for aic from dist linux binaries.
# Outputs to dist/deb/aic_<version>_<arch>.deb

APP_NAME="aic"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="$REPO_ROOT/dist"
DEB_OUT_DIR="$DIST_DIR/deb"

mkdir -p "$DEB_OUT_DIR"
# Remove any previously generated .deb files to avoid uploading stale ones
rm -f "$DEB_OUT_DIR"/*.deb 2>/dev/null || true

# Resolve version: prefer $VERSION env, otherwise read from internal/version/version.go
VERSION="${VERSION:-}"
if [ -z "${VERSION}" ]; then
  if [ -f "$REPO_ROOT/internal/version/version.go" ]; then
    VERSION=$(awk -F '"' '/const Version/ {print $2}' "$REPO_ROOT/internal/version/version.go")
  fi
fi
if [ -z "${VERSION}" ]; then
  echo "ERROR: Could not determine version (set VERSION env or ensure internal/version/version.go exists)." >&2
  exit 1
fi

package_one() {
  local arch="$1" binpath="$2"
  if [ ! -f "$binpath" ]; then
    echo "Binary not found for $arch at $binpath" >&2
    return 1
  fi
  local pkgroot="$DEB_OUT_DIR/${APP_NAME}_${VERSION}_${arch}"
  rm -rf "$pkgroot"
  mkdir -p "$pkgroot/DEBIAN" "$pkgroot/usr/local/bin"

  install -m 0755 "$binpath" "$pkgroot/usr/local/bin/${APP_NAME}"

  cat > "$pkgroot/DEBIAN/control" <<EOF
Package: ${APP_NAME}
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: ${arch}
Maintainer: aic maintainers <noreply@example.com>
Description: AI-assisted git commit message generator
EOF

  # Create versionless output filenames so each build overwrites
  local outfile="$DEB_OUT_DIR/${APP_NAME}_${arch}.deb"
  dpkg-deb --build "$pkgroot" "$outfile" >/dev/null
  echo "Built $outfile"
  # Clean build root for this package to keep dist/deb tidy
  rm -rf "$pkgroot"
}

# Map our dist layout to Debian arch names
BIN_AMD64="$DIST_DIR/ubuntu/${APP_NAME}"
BIN_ARM64="$DIST_DIR/ubuntu-arm64/${APP_NAME}"

ret=0
if [ -f "$BIN_AMD64" ]; then
  package_one amd64 "$BIN_AMD64" || ret=1
else
  echo "Skipping amd64: $BIN_AMD64 not found" >&2
fi

if [ -f "$BIN_ARM64" ]; then
  package_one arm64 "$BIN_ARM64" || ret=1
else
  echo "Skipping arm64: $BIN_ARM64 not found" >&2
fi

exit $ret
