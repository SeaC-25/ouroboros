#!/bin/sh
# README: smoke 脚本公共前置逻辑 —— 防止冒烟验收跑到陈旧的 bin/ouro。
#
# 用法（在各 smoke 脚本解析完参数、首次使用二进制之前 source）:
#     . "$ROOT/scripts/smoke_fresh_bin.sh"
#     ensure_fresh_ouro_bin "$OURO_XXX_SMOKE_BIN"   # 或 "$ROOT/bin/ouro"
#
# 行为:
#   * 目标二进制缺失（Windows 下自动补测 `<bin>.exe`),
#     或 $ROOT/cmd / $ROOT/internal 下任一 Go 源文件、或 go.mod/go.sum
#     比二进制新，则先执行 `go build -o "$bin" ./cmd/ouro`。
#   * OURO_SMOKE_SKIP_BUILD=1 可显式跳过自动构建（默认必须构建）。
#   * GOPROXY 沿用调用脚本已设置的值（未设置时回退默认镜像）。

ensure_fresh_ouro_bin() {
	_ouro_fresh_bin="$1"
	if [ "${OURO_SMOKE_SKIP_BUILD:-0}" = "1" ]; then
		printf 'smoke-fresh-bin: skip build (OURO_SMOKE_SKIP_BUILD=1)\n'
		return 0
	fi

	# Windows: 可执行文件带 .exe 后缀。
	_ouro_fresh_check="$_ouro_fresh_bin"
	if [ ! -e "$_ouro_fresh_check" ] && [ -e "$_ouro_fresh_check.exe" ]; then
		_ouro_fresh_check="$_ouro_fresh_check.exe"
	fi

	_ouro_fresh_rebuild=1
	if [ -f "$_ouro_fresh_check" ] \
		&& [ ! "$ROOT/go.mod" -nt "$_ouro_fresh_check" ] \
		&& { [ ! -f "$ROOT/go.sum" ] || [ ! "$ROOT/go.sum" -nt "$_ouro_fresh_check" ]; } \
		&& [ -z "$(find "$ROOT/cmd" "$ROOT/internal" -type f -name '*.go' -newer "$_ouro_fresh_check" -print -quit 2>/dev/null)" ]; then
		_ouro_fresh_rebuild=0
	fi
	if [ "$_ouro_fresh_rebuild" -eq 0 ]; then
		return 0
	fi

	command -v go >/dev/null 2>&1 || {
		printf 'smoke-fresh-bin: go not found on PATH, cannot rebuild %s\n' "$_ouro_fresh_bin" >&2
		return 1
	}
	mkdir -p "$(dirname "$_ouro_fresh_bin")"
	printf 'smoke-fresh-bin: rebuilding %s (missing or stale sources)\n' "$_ouro_fresh_bin"
	(cd "$ROOT" && env GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" go build -o "$_ouro_fresh_bin" ./cmd/ouro)
}
