#!/usr/bin/env bash
# Pack skills/agentflow into dist/skill.tgz (no bin/, no caches).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${1:-$ROOT/dist}"
mkdir -p "$OUT_DIR"
STAGE="$(mktemp -d)"
cleanup() { rm -rf "$STAGE"; }
trap cleanup EXIT

SRC="$ROOT/skills/agentflow"
if [[ ! -d "$SRC" ]]; then
  echo "missing $SRC" >&2
  exit 1
fi

mkdir -p "$STAGE/agentflow"
# Copy skill tree; exclude local binaries and junk
if command -v rsync >/dev/null 2>&1; then
  rsync -a \
    --exclude 'bin/' \
    --exclude '.DS_Store' \
    --exclude '*.exe' \
    --exclude 'node_modules/' \
    --exclude '__pycache__/' \
    --exclude '*.pyc' \
    "$SRC/" "$STAGE/agentflow/"
else
  cp -R "$SRC/." "$STAGE/agentflow/"
  rm -rf "$STAGE/agentflow/bin" 2>/dev/null || true
fi

# Ensure bt_service is fresh from root
if [[ -d "$ROOT/bt_service" ]]; then
  mkdir -p "$STAGE/agentflow/bt_service"
  if command -v rsync >/dev/null 2>&1; then
    rsync -a --delete \
      --exclude '__pycache__/' \
      --exclude '*.pyc' \
      --exclude 'tests/' \
      "$ROOT/bt_service/" "$STAGE/agentflow/bt_service/"
  else
    rm -rf "$STAGE/agentflow/bt_service"
    cp -R "$ROOT/bt_service" "$STAGE/agentflow/"
    rm -rf "$STAGE/agentflow/bt_service/tests" "$STAGE/agentflow/bt_service/__pycache__" 2>/dev/null || true
    find "$STAGE/agentflow/bt_service" -type d -name '__pycache__' -exec rm -rf {} + 2>/dev/null || true
    find "$STAGE/agentflow/bt_service" -name '*.pyc' -delete 2>/dev/null || true
  fi
fi

# Ensure trees is fresh from root
if [[ -d "$ROOT/trees" ]]; then
  mkdir -p "$STAGE/agentflow/trees"
  if command -v rsync >/dev/null 2>&1; then
    rsync -a --delete \
      --exclude '__pycache__/' \
      --exclude '*.pyc' \
      "$ROOT/trees/" "$STAGE/agentflow/trees/"
  else
    rm -rf "$STAGE/agentflow/trees"
    cp -R "$ROOT/trees" "$STAGE/agentflow/"
    find "$STAGE/agentflow/trees" -type d -name '__pycache__' -exec rm -rf {} + 2>/dev/null || true
    find "$STAGE/agentflow/trees" -name '*.pyc' -delete 2>/dev/null || true
  fi
fi

if [[ -f "$ROOT/requirements.txt" ]]; then
  cp "$ROOT/requirements.txt" "$STAGE/agentflow/requirements.txt"
fi

# Sanity: verify required BT engine and tree assets
if [[ ! -d "$STAGE/agentflow/bt_service" ]]; then
  echo "ERROR: skill package missing bt_service/ — refuse pack" >&2
  exit 1
fi
if [[ ! -d "$STAGE/agentflow/trees" ]]; then
  echo "ERROR: skill package missing trees/ — refuse pack" >&2
  exit 1
fi
if [[ ! -f "$STAGE/agentflow/requirements.txt" ]]; then
  echo "ERROR: skill package missing requirements.txt — refuse pack" >&2
  exit 1
fi

# Sanity: MCP GATE must be present for releases after v0.2.1
if ! grep -q "MCP GATE" "$STAGE/agentflow/hooks/mode-lib.js"; then
  echo "ERROR: skills/agentflow/hooks/mode-lib.js missing MCP GATE — refuse pack" >&2
  exit 1
fi
if [[ ! -f "$STAGE/agentflow/VERSION" ]]; then
  echo "ERROR: skills/agentflow/VERSION missing — refuse pack" >&2
  exit 1
fi
if [[ ! -f "$STAGE/agentflow/hooks/version-check.js" ]]; then
  echo "ERROR: skills/agentflow/hooks/version-check.js missing — refuse pack" >&2
  exit 1
fi
if [[ ! -f "$STAGE/agentflow/flows/update.md" ]]; then
  echo "ERROR: skills/agentflow/flows/update.md missing — refuse pack" >&2
  exit 1
fi

OUT="$OUT_DIR/skill.tgz"
tar -czf "$OUT" -C "$STAGE" agentflow
echo "packed $OUT ($(wc -c <"$OUT") bytes)"
echo "skill VERSION=$(tr -d '\r\n' <"$STAGE/agentflow/VERSION")"
grep -n "MCP GATE" "$STAGE/agentflow/hooks/mode-lib.js" | head -1
