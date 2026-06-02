#!/usr/bin/env sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
BIN_DIR="${BIN_DIR:-"$ROOT/bin"}"
BIN="${BIN:-"$BIN_DIR/ouro"}"
GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

mkdir -p "$BIN_DIR"
cd "$ROOT"

GOPROXY="$GOPROXY" go build -o "$BIN" ./cmd/ouro
printf 'built %s\n' "$BIN"
