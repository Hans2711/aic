#!/usr/bin/env bash
set -euo pipefail

# Installer: ensures repo exists at ~/aic, then links /usr/local/bin/aic -> <repo>/dist/<platform>/aic
# If /usr/local/bin isn’t writable, installs to ~/.local/bin (or ~/bin) without sudo.

APP_NAME="aic"
REPO_URL="https://github.com/Hans2711/aic.git"
REPO_DIR="${HOME}/aic"
PREFIX_SYSTEM="/usr/local/bin"
USER_BIN="${HOME}/.local/bin"
[ -d "${HOME}/bin" ] && USER_BIN="${HOME}/bin"

ensure_repo() {
  if [ -d "$REPO_DIR/.git" ]; then
    echo "Found repo at $REPO_DIR"
  else
    echo "Cloning $REPO_URL into $REPO_DIR ..."
    git clone --depth 1 "$REPO_URL" "$REPO_DIR"
  fi
}

detect_platform_dir() {
  local uname_s uname_m
  uname_s=$(uname -s 2>/dev/null || echo unknown)
  uname_m=$(uname -m 2>/dev/null || echo unknown)
  case "$uname_s" in
    Linux)
      case "$uname_m" in
        x86_64) echo "ubuntu" ;;
        aarch64|arm64) echo "ubuntu-arm64" ;;
        *) echo "" ;;
      esac
      ;;
    Darwin)
      case "$uname_m" in
        arm64) echo "mac" ;;
        x86_64) echo "mac-intel" ;;
        *) echo "" ;;
      esac
      ;;
    *) echo "" ;;
  esac
}

link_bin() {
  local src="$1" dst="$2"
  if [ -L "$dst" ] || [ -f "$dst" ]; then
    rm -f "$dst"
  fi
  ln -s "$src" "$dst"
  echo "Symlink: $dst -> $src"
}

ensure_repo

DIST_DIR="${REPO_DIR}/dist"
if [ ! -d "$DIST_DIR" ]; then
  echo "dist directory not found in $REPO_DIR. Please update the repo or build locally (scripts/build.sh)." >&2
  exit 1
fi

platform_dir=$(detect_platform_dir)
if [ -z "$platform_dir" ]; then
  echo "Unsupported OS/arch: $(uname -s) $(uname -m)" >&2
  exit 1
fi

BIN_PATH="${DIST_DIR}/${platform_dir}/${APP_NAME}"
if [ ! -f "$BIN_PATH" ]; then
  echo "Binary not found at $BIN_PATH. If you built locally, run scripts/build.sh; otherwise ensure you pulled prebuilt artifacts." >&2
  exit 1
fi

target_link="${PREFIX_SYSTEM}/${APP_NAME}"
if [ -w "$PREFIX_SYSTEM" ]; then
  abs_bin="$BIN_PATH"
  link_bin "$abs_bin" "$target_link"
  echo "Installed symlink for ${APP_NAME} (platform: ${platform_dir}) -> $BIN_PATH"
else
  echo "No write perms to $PREFIX_SYSTEM; installing to user bin." >&2
  mkdir -p "$USER_BIN"
  abs_bin="$BIN_PATH"
  dest_path="$USER_BIN/$APP_NAME"
  # If destination already resolves to the same file, report and exit cleanly
  if [ -e "$dest_path" ]; then
    dest_real=$(readlink -f "$dest_path" 2>/dev/null || echo "")
    src_real=$(readlink -f "$abs_bin" 2>/dev/null || echo "")
    if [ -n "$dest_real" ] && [ "$dest_real" = "$src_real" ]; then
      echo "Already installed: $dest_path -> $abs_bin"
      echo "(Consider adding $USER_BIN to PATH if not present.)"
      echo "Run: aic --version"
      exit 0
    fi
  fi

  # Prefer a symlink; overwrite existing file/symlink atomically
  if ln -sfn "$abs_bin" "$dest_path" 2>/dev/null; then
    echo "Symlink: $dest_path -> $abs_bin"
  else
    # As a fallback, place a plain copy
    cp -f "$abs_bin" "$dest_path"
    chmod 0755 "$dest_path"
    echo "Installed user copy: $dest_path"
  fi
  echo "(Consider adding $USER_BIN to PATH if not present.)"
fi

echo "Run: aic --version"
