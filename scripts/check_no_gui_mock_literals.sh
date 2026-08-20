#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PATTERN='SQL Injection|Port 80|192\.168\.1\.|10\.0\.0\.1|toolsReady[[:space:]]*[:=][[:space:]]*(42|37|29)|mock数据|fake fixture'

if grep -RInE "$PATTERN" "$ROOT/internal/api" "$ROOT/gui/src/app" 2>/dev/null | grep -v '_test\.go'; then
  echo "GUI/API mock literals remain" >&2
  exit 1
fi
