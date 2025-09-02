#!/usr/bin/env bash
set -euo pipefail

# Install the aic Git hook (prepare-commit-msg) into the current repo's .git/hooks

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOOK_SRC="$REPO_ROOT/scripts/git-hooks/prepare-commit-msg"
HOOK_DST="$(git rev-parse --git-path hooks/prepare-commit-msg 2>/dev/null || true)"

if [[ -z "${HOOK_DST}" ]]; then
  echo "Not inside a Git repository. Run from within a repo." >&2
  exit 1
fi

mkdir -p "$(dirname "$HOOK_DST")"

# Prefer symlink for easy updates; fallback to copy on failure
if ln -sf "$HOOK_SRC" "$HOOK_DST" 2>/dev/null; then
  :
else
  cp "$HOOK_SRC" "$HOOK_DST"
fi
chmod +x "$HOOK_DST"

echo "Installed aic prepare-commit-msg hook at: $HOOK_DST"
echo "Tip: ensure 'aic' is on your PATH so the hook can run."

