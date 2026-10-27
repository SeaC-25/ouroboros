#!/usr/bin/env sh
set -eu

case "$0" in
	*/*) ROOT_DIR="${0%/*}" ;;
	*) ROOT_DIR="." ;;
esac
ROOT="$(CDPATH= cd -- "$ROOT_DIR/.." && pwd)"
BIN_DIR="${BIN_DIR:-"$ROOT/bin"}"
BIN="${BIN:-"$BIN_DIR/ouro"}"
GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

mkdir -p "$BIN_DIR"
cd "$ROOT"

GOPROXY="$GOPROXY" go build -o "$BIN" ./cmd/ouro
printf 'built %s\n' "$BIN"
