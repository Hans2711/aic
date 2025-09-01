#!/usr/bin/env bash
set -euo pipefail

APP_NAME="aic"
OUT_DIR="dist"
mkdir -p "$OUT_DIR"
GOFLAGS=${GOFLAGS:-""}

echo "Building $APP_NAME..."
go build $GOFLAGS -o "$OUT_DIR/$APP_NAME" ./cmd/aic

echo "Done. Binary at $OUT_DIR/$APP_NAME"
