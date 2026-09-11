#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARCH="${1:-$(uname -m)}"
case "$ARCH" in
  x86_64|amd64) GOARCH=amd64; OUT=xport-linux-amd64 ;;
  aarch64|arm64) GOARCH=arm64; OUT=xport-linux-arm64 ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac
command -v go >/dev/null || { echo "Go 1.27+ is required" >&2; exit 1; }
cd "$ROOT"; mkdir -p dist
CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build -trimpath -ldflags='-s -w' -o "dist/$OUT" ./cmd/xport
printf 'Built %s\n' "$ROOT/dist/$OUT"
