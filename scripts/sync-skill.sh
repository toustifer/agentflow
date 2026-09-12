#!/usr/bin/env bash
# Synchronize bt_service, trees, and requirements.txt into skills/agentflow/
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST="$ROOT/skills/agentflow"

echo "==> syncing bt_service and trees to $DEST"
mkdir -p "$DEST/bt_service" "$DEST/trees"

if command -v rsync >/dev/null 2>&1; then
  rsync -a --delete \
    --exclude '__pycache__/' \
    --exclude '*.pyc' \
    --exclude 'tests/' \
    "$ROOT/bt_service/" "$DEST/bt_service/"
  rsync -a --delete \
    --exclude '__pycache__/' \
    --exclude '*.pyc' \
    "$ROOT/trees/" "$DEST/trees/"
else
  rm -rf "$DEST/bt_service" "$DEST/trees"
  cp -R "$ROOT/bt_service" "$DEST/"
  rm -rf "$DEST/bt_service/tests" "$DEST/bt_service/__pycache__" 2>/dev/null || true
  find "$DEST/bt_service" -type d -name '__pycache__' -exec rm -rf {} + 2>/dev/null || true
  find "$DEST/bt_service" -name '*.pyc' -delete 2>/dev/null || true

  cp -R "$ROOT/trees" "$DEST/"
  find "$DEST/trees" -type d -name '__pycache__' -exec rm -rf {} + 2>/dev/null || true
  find "$DEST/trees" -name '*.pyc' -delete 2>/dev/null || true
fi

cp "$ROOT/requirements.txt" "$DEST/requirements.txt"
echo "==> sync complete"
