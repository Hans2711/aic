#!/usr/bin/env bash
set -euo pipefail

APP_NAME="aic"
# Resolve repo root (script lives in scripts/)
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ROOT_OUT="${REPO_ROOT}/dist"
GOFLAGS=${GOFLAGS:-""}

# Ensure we have a writable module cache even if the system GOPATH is read-only
GO_LOCAL_MODCACHE="${GO_LOCAL_MODCACHE:-$ROOT_OUT/.gomodcache}"
GO_LOCAL_GOPATH="${GO_LOCAL_GOPATH:-$ROOT_OUT/.gopath}"
mkdir -p "$GO_LOCAL_MODCACHE" "$GO_LOCAL_GOPATH"
export GOMODCACHE="$GO_LOCAL_MODCACHE"
export GOPATH="$GO_LOCAL_GOPATH"

set -euo pipefail

# Pre-fetch modules into our local cache to avoid repeated network calls per target
echo "Downloading modules into local cache ($GOMODCACHE)" >&2
go mod download all

build_target() {
	local goos="$1" goarch="$2" subdir="$3"
	local outdir="$ROOT_OUT/$subdir"
	mkdir -p "$outdir"
	local outfile="$outdir/$APP_NAME"
	echo "Building ${APP_NAME} for ${goos}/${goarch} -> ${outfile}" >&2
    GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
        go build $GOFLAGS -o "$outfile" ./cmd/aic
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
	# Provide legacy single-path alias for tooling expecting dist/aic
	if [ -f ubuntu/aic ] && [ ! -e aic ]; then ln -s ubuntu/aic aic; fi
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
