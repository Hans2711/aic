#!/usr/bin/env bash
set -euo pipefail

# Simple install script: symlink /usr/local/bin/aic -> <repo>/dist/<platform>/aic
# If not root-writable, creates user-level copy/symlink in ~/.local/bin (or ~/bin).

APP_NAME="aic"
DIST_DIR="dist"
PREFIX_SYSTEM="/usr/local/bin"
USER_BIN="${HOME}/.local/bin"
[ -d "${HOME}/bin" ] && USER_BIN="${HOME}/bin"

if [ ! -d "$DIST_DIR" ]; then
  echo "dist directory not found. Run scripts/build.sh first." >&2
  exit 1
fi

uname_s=$(uname -s 2>/dev/null || echo unknown)
uname_m=$(uname -m 2>/dev/null || echo unknown)
platform_dir=""
case "$uname_s" in
  Linux)
    case "$uname_m" in
      x86_64) platform_dir="ubuntu" ;;
      aarch64|arm64) platform_dir="ubuntu-arm64" ;;
      *) echo "Unsupported Linux arch: $uname_m" >&2; exit 1 ;;
    esac
    ;;
  Darwin)
    case "$uname_m" in
      arm64) platform_dir="mac" ;;
      x86_64) platform_dir="mac-intel" ;;
      *) echo "Unsupported macOS arch: $uname_m" >&2; exit 1 ;;
    esac
    ;;
  *)
    echo "Unsupported OS: $uname_s" >&2
    exit 1
    ;;
 esac

BIN_PATH="${DIST_DIR}/${platform_dir}/${APP_NAME}"
if [ ! -f "$BIN_PATH" ]; then
  echo "Binary not found at $BIN_PATH. Run scripts/build.sh." >&2
  exit 1
fi

target_link="${PREFIX_SYSTEM}/${APP_NAME}"
needs_sudo=0
if [ ! -w "$PREFIX_SYSTEM" ]; then needs_sudo=1; fi

link_bin() {
  local src="$1" dst="$2"
  if [ -L "$dst" ] || [ -f "$dst" ]; then
    rm -f "$dst"
  fi
  ln -s "$src" "$dst"
  echo "Symlink: $dst -> $src"
}

if [ $needs_sudo -eq 0 ]; then
  # Use absolute path for symlink target so it works outside repo dir.
  abs_bin="$(cd "$(dirname "$BIN_PATH")" && pwd)/$APP_NAME"
  link_bin "$abs_bin" "$target_link"
else
  echo "No write perms to $PREFIX_SYSTEM; installing to user bin." >&2
  mkdir -p "$USER_BIN"
  abs_bin="$(cd "$(dirname "$BIN_PATH")" && pwd)/$APP_NAME"
  # Prefer symlink if possible
  if ln -s "$abs_bin" "$USER_BIN/$APP_NAME" 2>/dev/null; then
    echo "Symlink: $USER_BIN/$APP_NAME -> $abs_bin"
  else
    cp "$abs_bin" "$USER_BIN/$APP_NAME"
    chmod 0755 "$USER_BIN/$APP_NAME"
    echo "Installed user copy: $USER_BIN/$APP_NAME"
  fi
  echo "(Consider adding $USER_BIN to PATH if not present.)"
  exit 0
fi

echo "Installed symlink for ${APP_NAME} (platform: ${platform_dir}) -> $BIN_PATH"
echo "Run: aic --version"
