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
    "$SRC/" "$STAGE/agentflow/"
else
  cp -R "$SRC/." "$STAGE/agentflow/"
  rm -rf "$STAGE/agentflow/bin" 2>/dev/null || true
fi

# Sanity: MCP GATE must be present for releases after v0.2.1
if ! grep -q "MCP GATE" "$STAGE/agentflow/hooks/mode-lib.js"; then
  echo "ERROR: skills/agentflow/hooks/mode-lib.js missing MCP GATE — refuse pack" >&2
  exit 1
fi

OUT="$OUT_DIR/skill.tgz"
tar -czf "$OUT" -C "$STAGE" agentflow
echo "packed $OUT ($(wc -c <"$OUT") bytes)"
grep -n "MCP GATE" "$STAGE/agentflow/hooks/mode-lib.js" | head -1
