#!/bin/sh
set -eu

[ -f go.mod ] || printf 'module puissance4-go\n\ngo 1.21\n' > go.mod

BIN=./puissance4-go
NEED_BUILD=0

[ -f "$BIN" ] || NEED_BUILD=1
if [ "$NEED_BUILD" -eq 0 ] && find . -type f -name '*.go' -newer "$BIN" | grep -q .; then NEED_BUILD=1; fi
if [ "$NEED_BUILD" -eq 0 ] && find templates static -type f -newer "$BIN" 2>/dev/null | grep -q .; then NEED_BUILD=1; fi

if [ "$NEED_BUILD" -eq 1 ]; then
  echo "[build] Building puissance4-go..."
  GOOS=linux GOARCH=amd64 go build -o "$BIN"
fi

echo "[run] Starting on port ${SERVER_PORT:-8080}"
exec "$BIN"
