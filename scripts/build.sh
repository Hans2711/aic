#!/usr/bin/env bash
set -euo pipefail

APP_NAME="aic"
ROOT_OUT="dist"
GOFLAGS=${GOFLAGS:-""}
VERSION=${VERSION:-"0.1.0"}

set -euo pipefail

LDIMPORT="github.com/diesi/aic/internal/version.Version"
LDFLAGS="-X ${LDIMPORT}=${VERSION}"

echo "Building version: ${VERSION}" >&2

build_target() {
	local goos="$1" goarch="$2" subdir="$3"
	local outdir="$ROOT_OUT/$subdir"
	mkdir -p "$outdir"
	local outfile="$outdir/$APP_NAME"
	echo "Building ${APP_NAME} for ${goos}/${goarch} -> ${outfile}" >&2
	GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
		go build $GOFLAGS -ldflags "$LDFLAGS" -o "$outfile" ./cmd/aic
	echo "  Done." >&2
}

# Targets
build_target linux amd64 ubuntu
build_target linux arm64 ubuntu-arm64
build_target darwin arm64 mac
build_target darwin amd64 mac-intel

# Checksums
echo "Generating checksums..." >&2
checksum_tool() {
	if command -v sha256sum >/dev/null 2>&1; then echo sha256sum; return; fi
	if command -v shasum >/dev/null 2>&1; then echo "shasum -a 256"; return; fi
	echo ""; return 0
}
CK_CMD=$(checksum_tool)
if [ -n "$CK_CMD" ]; then
	(
		cd "$ROOT_OUT"
		rm -f checksums.txt
		for f in $(find . -type f -name "$APP_NAME"); do
			$CK_CMD "$f" >> checksums.txt
		done
	)
	echo "Checksums written to $ROOT_OUT/checksums.txt" >&2
else
	echo "No checksum tool found (sha256sum/shasum). Skipping." >&2
fi

echo "All builds complete under $ROOT_OUT/"
