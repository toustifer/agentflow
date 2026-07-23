#!/usr/bin/env bash
# Cross-compile release binaries into dist/ + pack skill.tgz
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
OUT_DIR="${OUT_DIR:-$ROOT/dist}"
mkdir -p "$OUT_DIR"

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}"

build_one() {
  local goos="$1" goarch="$2" outname="$3"
  echo ">> building $outname (GOOS=$goos GOARCH=$goarch)"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "$LDFLAGS" \
    -o "$OUT_DIR/$outname" ./cmd/agentflow/
  ls -la "$OUT_DIR/$outname"
}

build_one windows amd64 agentflow-windows-amd64.exe
build_one linux amd64 agentflow-linux-amd64
build_one darwin amd64 agentflow-darwin-amd64
build_one darwin arm64 agentflow-darwin-arm64

bash "$ROOT/scripts/pack-skill.sh" "$OUT_DIR"

echo
echo "Release artifacts in $OUT_DIR:"
ls -la "$OUT_DIR"/agentflow-* "$OUT_DIR"/skill.tgz 2>/dev/null || ls -la "$OUT_DIR"
