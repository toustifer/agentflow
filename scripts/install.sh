#!/usr/bin/env bash
# Download-first install of agentflow skill + MCP binary (no Go, no git clone).
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/toustifer/agentflow/master/scripts/install.sh | bash
#   VERSION=v0.2.1 bash install.sh
#   bash install.sh --write-config   # also merge ~/.claude.json mcp entry (backup first)
set -euo pipefail

REPO="${REPO:-toustifer/agentflow}"
VERSION="${VERSION:-v0.2.1}"
BASE="${BASE:-https://github.com/${REPO}/releases/download/${VERSION}}"
DEST="${DEST:-$HOME/.claude/skills/agentflow}"
WRITE_CONFIG=0
for arg in "$@"; do
  case "$arg" in
    --write-config) WRITE_CONFIG=1 ;;
    --version=*) VERSION="${arg#--version=}"; BASE="https://github.com/${REPO}/releases/download/${VERSION}" ;;
  esac
done

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$os" in
  darwin) os=darwin ;;
  linux) os=linux ;;
  msys*|mingw*|cygwin*) os=windows ;;
  *) echo "Unsupported OS: $os" >&2; exit 1 ;;
esac
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "Unsupported arch: $arch" >&2; exit 1 ;;
esac

if [[ "$os" == "windows" ]]; then
  BIN_NAME="agentflow-windows-amd64.exe"
  LOCAL_BIN_NAME="agentflow.exe"
elif [[ "$os" == "darwin" && "$arch" == "arm64" ]]; then
  BIN_NAME="agentflow-darwin-arm64"
  LOCAL_BIN_NAME="agentflow"
elif [[ "$os" == "darwin" ]]; then
  BIN_NAME="agentflow-darwin-amd64"
  LOCAL_BIN_NAME="agentflow"
else
  if [[ "$arch" != "amd64" ]]; then
    echo "Linux release currently ships amd64 only; got $arch" >&2
    exit 1
  fi
  BIN_NAME="agentflow-linux-amd64"
  LOCAL_BIN_NAME="agentflow"
fi

TMP="$(mktemp -d)"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

echo "==> agentflow install $VERSION"
echo "    skill+bin -> $DEST"
echo "    binary    -> $BIN_NAME"

echo "==> download skill.tgz"
curl -fsSL "$BASE/skill.tgz" -o "$TMP/skill.tgz"

echo "==> download $BIN_NAME"
curl -fsSL "$BASE/$BIN_NAME" -o "$TMP/$BIN_NAME"
chmod +x "$TMP/$BIN_NAME"

if [[ -d "$DEST" ]]; then
  BAK="${DEST}.bak-$(date +%Y%m%d%H%M%S)"
  echo "==> backup existing skill -> $BAK"
  cp -R "$DEST" "$BAK"
fi

echo "==> extract skill"
mkdir -p "$DEST"
# skill.tgz top-level dir is agentflow/
tar -xzf "$TMP/skill.tgz" -C "$TMP"
rsync -a --delete \
  --exclude 'bin/' \
  "$TMP/agentflow/" "$DEST/" 2>/dev/null \
  || {
    # fallback without rsync
    find "$DEST" -mindepth 1 -maxdepth 1 ! -name bin -exec rm -rf {} +
    cp -R "$TMP/agentflow/." "$DEST/"
  }

mkdir -p "$DEST/bin"
cp "$TMP/$BIN_NAME" "$DEST/bin/$LOCAL_BIN_NAME"
chmod +x "$DEST/bin/$LOCAL_BIN_NAME"

if ! grep -q "MCP GATE" "$DEST/hooks/mode-lib.js"; then
  echo "ERROR: installed skill missing MCP GATE" >&2
  exit 1
fi

ABS_BIN="$DEST/bin/$LOCAL_BIN_NAME"
# Prefer realpath when available
if command -v realpath >/dev/null 2>&1; then
  ABS_BIN="$(realpath "$ABS_BIN")"
elif command -v python3 >/dev/null 2>&1; then
  ABS_BIN="$(python3 -c 'import os,sys; print(os.path.realpath(sys.argv[1]))' "$ABS_BIN")"
fi

ABS_INJECT="$DEST/hooks/mode-inject.js"
ABS_STATUS="$DEST/hooks/statusline.js"
if command -v realpath >/dev/null 2>&1; then
  ABS_INJECT="$(realpath "$ABS_INJECT")"
  ABS_STATUS="$(realpath "$ABS_STATUS")"
fi

echo
echo "==> installed"
echo "    binary: $ABS_BIN"
echo "    skill:  $DEST"
grep -n "MCP GATE" "$DEST/hooks/mode-lib.js" | head -1

if [[ "$WRITE_CONFIG" -eq 1 ]]; then
  CLAUDE_JSON="${CLAUDE_JSON:-$HOME/.claude.json}"
  echo "==> --write-config: merge agentflow into $CLAUDE_JSON"
  if [[ ! -f "$CLAUDE_JSON" ]]; then
    echo '{}' >"$CLAUDE_JSON"
  fi
  cp "$CLAUDE_JSON" "${CLAUDE_JSON}.bak-$(date +%Y%m%d%H%M%S)"
  python3 - "$CLAUDE_JSON" "$ABS_BIN" <<'PY'
import json, sys
path, binary = sys.argv[1], sys.argv[2]
with open(path, "r", encoding="utf-8") as f:
    data = json.load(f)
servers = data.setdefault("mcpServers", {})
servers["agentflow"] = {
    "command": binary,
    "args": ["stdio"],
    "type": "stdio",
}
with open(path, "w", encoding="utf-8") as f:
    json.dump(data, f, indent=2)
    f.write("\n")
print("wrote mcpServers.agentflow ->", binary)
PY
else
  echo
  echo "==> add to ~/.claude.json (or project mcp) — copy:"
  cat <<EOF
{
  "mcpServers": {
    "agentflow": {
      "command": "${ABS_BIN}",
      "args": ["stdio"],
      "type": "stdio"
    }
  }
}
EOF
fi

echo
echo "==> sticky hooks — merge into ~/.claude/settings.json (do not wipe other hooks):"
cat <<EOF
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "node ${ABS_INJECT}",
            "timeout": 5
          }
        ]
      }
    ]
  },
  "statusLine": {
    "type": "command",
    "command": "node ${ABS_STATUS}",
    "refreshInterval": 5
  }
}
EOF

echo
echo "==> next"
echo "    1. Merge MCP + hooks configs if not --write-config"
echo "    2. Fully quit and restart Claude Code"
echo "    3. /mcp -> agentflow not failed"
echo "    4. Session must call mcp__agentflow__flow_ping (CLI Connected is not enough)"
echo "    5. statusline may show MCP:cfg|missing|broken"
echo "Done."
