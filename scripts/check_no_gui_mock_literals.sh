#!/bin/sh
# scripts/check_no_gui_mock_literals.sh
#
# 防 mock 守门:扫描 internal/api 与 gui/src/app,若发现遗留的 mock
# 字面量(硬编码内网地址、工具计数、中文 mock 文案等)则报错退出 1。
#
# 说明:
# - 目标目录必须存在;缺失时直接报错退出非零,避免 grep 静默吞错导致
#   “零命中=通过”的假阳性。
# - `_test.go` 文件豁免:单元测试允许使用与 PATTERN 相同的字面量作为
#   断言样本(它们本就用于验证真实输出,不是生产代码里的 mock)。
set -eu

case "$0" in
	*/*) ROOT_DIR="${0%/*}" ;;
	*) ROOT_DIR="." ;;
esac
ROOT="$(CDPATH= cd -- "$ROOT_DIR/.." && pwd)"
PATTERN='SQL Injection|Port 80|192\.168\.1\.|10\.0\.0\.1|toolsReady[[:space:]]*[:=][[:space:]]*(42|37|29)|mock数据|fake fixture'

CHECK_DIRS="$ROOT/internal/api $ROOT/gui/src/app"

for dir in $CHECK_DIRS; do
  if [ ! -d "$dir" ]; then
    echo "required directory missing: $dir" >&2
    exit 2
  fi
done

# 注意:不得给 grep 加 2>/dev/null —— 吞掉错误会让“读不到目录”表现成
# “零命中”,守门就静默失效了。
if grep -RInE "$PATTERN" $CHECK_DIRS | grep -v '_test\.go'; then
  echo "GUI/API mock literals remain" >&2
  exit 1
fi
