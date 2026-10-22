# lib_smoke_common.sh — smoke 脚本公共辅助函数
#
# 用法：在各 smoke 脚本顶部（设好 SMOKE_ROOT / fail 之后）：
#   . "$(dirname "$0")/lib_smoke_common.sh"
#
# 提供：
#   smoke_is_windows          —— 判断是否运行在 Windows(Git Bash/MSYS2）
#   smoke_exe_suffix          —— Windows 输出 ".exe"，其余为空
#   cleanup_tree <pid>        —— 终止整个进程树（Windows 用 taskkill /T /F；
#                                POSIX 先 TERM 后 KILL)，解决 `go run` / npm
#                                包装进程被杀后真实子进程残留占端口的问题
#   wait_http <url> <label>   —— 轮询直到 URL 可访问；默认上限 120 秒，
#                                可用 OURO_SMOKE_WAIT_HTTP_TIMEOUT 覆盖（秒）;
#                                若设置了 WATCH_PID 且该进程已退出则立即失败
#   assert_port_free <port> <label>
#                              —— 启动服务前检测端口未被占用；占用则先警告
#                                再失败，避免冒烟撞上旧实例误判通过
#
# 端口约定：各 smoke 的端口均支持 OURO_*_PORT 环境变量覆盖，默认值见各脚本头注释。

smoke_is_windows() {
  [ "$(node -p 'process.platform' 2>/dev/null)" = "win32" ]
}

smoke_exe_suffix() {
  if smoke_is_windows; then
    printf '.exe'
  else
    printf ''
  fi
}

cleanup_tree() {
  pid="$1"
  [ -n "$pid" ] || return 0
  if smoke_is_windows; then
    # Git Bash 下需要 MSYS2_ARG_CONV_EXCL 防止 /PID 等被路径转换
    MSYS2_ARG_CONV_EXCL='*' taskkill.exe /PID "$pid" /T /F >/dev/null 2>&1 || true
  else
    kill "$pid" 2>/dev/null || true
    sleep 0.5
    kill -KILL "$pid" 2>/dev/null || true
  fi
}

wait_http() {
  url="$1"
  label="$2"
  timeout_s="${OURO_SMOKE_WAIT_HTTP_TIMEOUT:-120}"
  max_loops=$((timeout_s * 4))
  i=0
  while [ "$i" -lt "$max_loops" ]; do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    if [ -n "${WATCH_PID:-}" ] && ! kill -0 "$WATCH_PID" 2>/dev/null; then
      fail "$label 等待期间服务进程已退出"
    fi
    i=$((i + 1))
    sleep 0.25
  done
  fail "$label did not become ready (timeout ${timeout_s}s)"
}

assert_port_free() {
  port="$1"
  label="$2"
  # 能建立连接并得到任意 HTTP 响应即视为占用；连接被拒绝才空闲
  if curl -sS --max-time 2 -o /dev/null "http://127.0.0.1:$port/" 2>/dev/null; then
    printf 'smoke: 警告：端口 %s 已被占用，可能残留旧实例，无法安全判断结果\n' "$port" >&2
    fail "$label port $port is already in use"
  fi
}
