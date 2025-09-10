#!/usr/bin/env bash
set -euo pipefail

# Symlink the VS Code extension from this repo into ~/.vscode/extensions/aic
# Usage: scripts/link_vscode_extension.sh [--source <path>] [--dest-dir <path>] [--name <link-name>]

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Try to locate repo root; fall back to parent of scripts/
if REPO_ROOT=$(cd "$SCRIPT_DIR" && git rev-parse --show-toplevel 2>/dev/null); then
  :
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
fi

SOURCE_DEFAULT="$REPO_ROOT/plugins/vscode"
DEST_DIR_DEFAULT="$HOME/.vscode/extensions"
NAME_DEFAULT="aic"

SOURCE="$SOURCE_DEFAULT"
DEST_DIR="$DEST_DIR_DEFAULT"
NAME="$NAME_DEFAULT"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --source)
      SOURCE="$2"; shift 2;;
    --dest-dir)
      DEST_DIR="$2"; shift 2;;
    --name)
      NAME="$2"; shift 2;;
    -h|--help)
      echo "Usage: $0 [--source <path>] [--dest-dir <path>] [--name <link-name>]";
      echo "Default source: $SOURCE_DEFAULT";
      echo "Default dest-dir: $DEST_DIR_DEFAULT";
      echo "Default name: $NAME_DEFAULT";
      exit 0;;
    *)
      echo "Unknown argument: $1" >&2; exit 1;;
  esac
done

if [[ ! -d "$SOURCE" ]]; then
  echo "Source extension directory not found: $SOURCE" >&2
  exit 1
fi

mkdir -p "$DEST_DIR"
DEST_PATH="$DEST_DIR/$NAME"

# If already linked to the same source, nothing to do
if [[ -L "$DEST_PATH" ]]; then
  TARGET_RESOLVED="$(readlink "$DEST_PATH" || true)"
  # Resolve relative link target to absolute for comparison
  if [[ ! "$TARGET_RESOLVED" = /* ]]; then
    TARGET_RESOLVED="$(cd "$DEST_DIR" && cd "$(dirname "$TARGET_RESOLVED")" && pwd)/$(basename "$TARGET_RESOLVED")"
  fi
  SOURCE_RESOLVED="$(cd "$SOURCE" && pwd)"
  if [[ "$TARGET_RESOLVED" == "$SOURCE_RESOLVED" ]]; then
    echo "Already linked: $DEST_PATH -> $SOURCE_RESOLVED"
    exit 0
  fi
fi

if [[ -e "$DEST_PATH" || -L "$DEST_PATH" ]]; then
  TS="$(date +%s)"
  BAK="$DEST_PATH.bak.$TS"
  echo "Backing up existing path: $DEST_PATH -> $BAK"
  mv "$DEST_PATH" "$BAK"
fi

ln -s "$SOURCE" "$DEST_PATH"
echo "Created symlink: $DEST_PATH -> $SOURCE"
echo "VS Code: Reload Window to pick up the extension (Ctrl/Cmd+Shift+P → Reload Window)."

