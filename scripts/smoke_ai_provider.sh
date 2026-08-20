#!/bin/sh
set -eu

TRUSTED_PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
PATH="$TRUSTED_PATH"
export PATH

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
TMPDIR_BASE="${TMPDIR:-/tmp}"
OURO_AI_PROVIDER_SMOKE_BIN="${OURO_AI_PROVIDER_SMOKE_BIN:-$ROOT/bin/ouro}"
OURO_AI_PROVIDER_SMOKE_API_KEY="smoke-secret-value"
export OURO_AI_PROVIDER_SMOKE_API_KEY
SERVER_PID=""

if [ -z "${OURO_AI_PROVIDER_SMOKE_LOG_DIR:-}" ]; then
	OURO_AI_PROVIDER_SMOKE_LOG_DIR="$(mktemp -d "$TMPDIR_BASE/ouro-ai-provider-smoke-logs-XXXXXX")"
fi

cleanup() {
	if [ -n "$SERVER_PID" ]; then
		kill "$SERVER_PID" 2>/dev/null || true
		wait "$SERVER_PID" 2>/dev/null || true
	fi
}
trap cleanup EXIT HUP INT TERM

fail() {
	printf 'ai-provider-smoke: failed: %s\n' "$1" >&2
	printf 'ai-provider-smoke: logs %s\n' "$OURO_AI_PROVIDER_SMOKE_LOG_DIR" >&2
	exit 1
}

assert_absolute_path() {
	label="$1"
	path="$2"
	case "$path" in
		/*) ;;
		*) fail "$label path must be absolute: $path" ;;
	esac
}

assert_not_symlink_dir() {
	label="$1"
	path="$2"
	[ ! -L "$path" ] || fail "$label directory must not be a symlink: $path"
}

assert_not_symlink_file() {
	label="$1"
	path="$2"
	[ ! -L "$path" ] || fail "$label file must not be a symlink: $path"
}

assert_no_symlink_path_components() {
	label="$1"
	path="$2"
	remaining="${path#/}"
	current=""
	while [ -n "$remaining" ]; do
		component="${remaining%%/*}"
		if [ "$remaining" = "$component" ]; then
			remaining=""
		else
			remaining="${remaining#*/}"
		fi
		[ -n "$component" ] || continue
		current="$current/$component"
		[ ! -L "$current" ] || fail "$label path component must not be a symlink: $current"
		[ -e "$current" ] || break
	done
}

assert_non_empty_file() {
	label="$1"
	path="$2"
	[ ! -L "$path" ] || fail "$label must not be a symlink: $path"
	[ -s "$path" ] || fail "$label was not written or is empty: $path"
}

assert_log_contains() {
	log_path="$1"
	needle="$2"
	message="$3"
	grep -F -q "$needle" "$log_path" || fail "$message"
}

assert_log_not_contains() {
	log_path="$1"
	needle="$2"
	message="$3"
	if grep -F -q "$needle" "$log_path"; then
		fail "$message"
	fi
}

write_fake_provider_server() {
	server_go="$1"
	cat > "$server_go" <<'GOEOF'
package main

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: fake-openai-provider <base-url-file>")
	}
	expectedBearer := os.Getenv("OURO_AI_PROVIDER_SMOKE_API_KEY")
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models/gpt-smoke", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r, expectedBearer) {
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, map[string]any{"id": "gpt-smoke", "object": "model"})
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r, expectedBearer) {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		writeJSON(w, map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "semantic_action: enumerate_paths"}},
			},
			"usage": map[string]any{"prompt_tokens": 7, "completion_tokens": 5, "total_tokens": 12},
		})
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	baseURL := "http://" + listener.Addr().String() + "/v1"
	if err := os.WriteFile(os.Args[1], []byte(baseURL+"\n"), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.Serve(listener, mux))
}

func authorized(w http.ResponseWriter, r *http.Request, expectedBearer string) bool {
	if expectedBearer == "" || r.Header.Get("Authorization") != "Bearer "+expectedBearer {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
GOEOF
}

start_fake_provider() {
	if [ "${OURO_AI_PROVIDER_SMOKE_SKIP_FAKE_SERVER:-}" = "1" ]; then
		OURO_AI_PROVIDER_SMOKE_BASE_URL="${OURO_AI_PROVIDER_SMOKE_BASE_URL:-http://127.0.0.1:1/v1}"
		return
	fi
	server_dir="$(mktemp -d "$TMPDIR_BASE/ouro-ai-provider-smoke-server-XXXXXX")"
	server_go="$server_dir/fake-openai-provider.go"
	server_url_file="$server_dir/base-url.txt"
	write_fake_provider_server "$server_go"
	go run "$server_go" "$server_url_file" > "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/fake-provider.txt" 2>&1 &
	SERVER_PID="$!"
	i=0
	while [ "$i" -lt 100 ]; do
		if [ -s "$server_url_file" ]; then
			OURO_AI_PROVIDER_SMOKE_BASE_URL="$(cat "$server_url_file")"
			return
		fi
		kill -0 "$SERVER_PID" 2>/dev/null || fail "fake provider exited before publishing base URL"
		i=$((i + 1))
		sleep 0.1
	done
	fail "fake provider did not publish base URL"
}

run_ouro_logged() {
	label="$1"
	log_path="$2"
	shift 2
	if ! "$OURO_AI_PROVIDER_SMOKE_BIN" "$@" > "$log_path" 2>&1; then
		fail "$label failed; see $log_path"
	fi
}

assert_absolute_path "AI provider smoke binary" "$OURO_AI_PROVIDER_SMOKE_BIN"
assert_no_symlink_path_components "AI provider smoke binary" "$OURO_AI_PROVIDER_SMOKE_BIN"
assert_not_symlink_file "AI provider smoke binary" "$OURO_AI_PROVIDER_SMOKE_BIN"
[ -x "$OURO_AI_PROVIDER_SMOKE_BIN" ] || fail "AI provider smoke binary is not executable: $OURO_AI_PROVIDER_SMOKE_BIN"

assert_absolute_path "AI provider smoke log dir" "$OURO_AI_PROVIDER_SMOKE_LOG_DIR"
assert_no_symlink_path_components "AI provider smoke log dir" "$OURO_AI_PROVIDER_SMOKE_LOG_DIR"
assert_not_symlink_dir "AI provider smoke log dir" "$OURO_AI_PROVIDER_SMOKE_LOG_DIR"
mkdir -p "$OURO_AI_PROVIDER_SMOKE_LOG_DIR"
assert_no_symlink_path_components "AI provider smoke log dir" "$OURO_AI_PROVIDER_SMOKE_LOG_DIR"
assert_not_symlink_dir "AI provider smoke log dir" "$OURO_AI_PROVIDER_SMOKE_LOG_DIR"

workspace_provided=0
if [ -z "${WORKSPACE:-}" ]; then
	SMOKE_ROOT="$(mktemp -d "$TMPDIR_BASE/ouro-ai-provider-smoke-XXXXXX")"
	WORKSPACE="$SMOKE_ROOT/workspace"
else
	workspace_provided=1
fi
assert_absolute_path "workspace" "$WORKSPACE"
assert_no_symlink_path_components "workspace" "$WORKSPACE"
assert_not_symlink_dir "workspace" "$WORKSPACE"

if [ -e "$WORKSPACE" ] && [ ! -d "$WORKSPACE" ]; then
	fail "workspace path exists and is not a directory"
fi

if [ "$workspace_provided" -eq 1 ] && [ -d "$WORKSPACE" ]; then
	existing_entry="$(find "$WORKSPACE" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null || true)"
	if [ -n "$existing_entry" ] && [ "${OURO_AI_PROVIDER_SMOKE_ALLOW_EXISTING_WORKSPACE:-}" != "1" ]; then
		fail "refusing existing non-empty workspace; set OURO_AI_PROVIDER_SMOKE_ALLOW_EXISTING_WORKSPACE=1 only for a safe smoke workspace"
	fi
fi

cd "$ROOT"

printf 'ai-provider-smoke: workspace %s\n' "$WORKSPACE"
printf 'ai-provider-smoke: logs %s\n' "$OURO_AI_PROVIDER_SMOKE_LOG_DIR"
printf 'binary: %s\n' "$OURO_AI_PROVIDER_SMOKE_BIN"

run_ouro_logged "init" "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/init.txt" init "$WORKSPACE"
start_fake_provider
printf 'fake-provider: %s\n' "$OURO_AI_PROVIDER_SMOKE_BASE_URL"

run_ouro_logged "provider configure" "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/configure-provider.txt" provider configure-openai-compatible "$WORKSPACE" --name smoke-openai --base-url "$OURO_AI_PROVIDER_SMOKE_BASE_URL" --model gpt-smoke --api-key-env OURO_AI_PROVIDER_SMOKE_API_KEY --capability chat --capability tool_planning --routing-strategy enabled_order_failover --enable
assert_log_contains "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/configure-provider.txt" "provider-configure: status=written provider=smoke-openai" "provider configure output missing status"
assert_log_contains "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/configure-provider.txt" "api_key_value_stored=false" "provider configure did not report metadata-only key handling"
assert_log_not_contains "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/configure-provider.txt" "$OURO_AI_PROVIDER_SMOKE_API_KEY" "provider configure output leaked API key value"
assert_non_empty_file "providers.yaml" "$WORKSPACE/providers.yaml"
assert_log_not_contains "$WORKSPACE/providers.yaml" "$OURO_AI_PROVIDER_SMOKE_API_KEY" "providers.yaml leaked API key value"
printf 'provider-configure: ok\n'

run_ouro_logged "doctor" "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/doctor.txt" doctor "$WORKSPACE"
grep -q '^workspace: ready$' "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/doctor.txt" || fail "doctor did not report ready"
printf 'doctor: ready\n'

run_ouro_logged "provider generate" "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/provider-generate.txt" provider generate "$WORKSPACE" --prompt "Return one reviewed semantic action marker." --max-tokens 64 --record-usage
assert_log_contains "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/provider-generate.txt" "provider-generate: status=completed" "provider generate output missing completion status"
assert_log_contains "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/provider-generate.txt" "provider=smoke-openai" "provider generate did not use configured provider"
assert_log_contains "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/provider-generate.txt" "text=semantic_action: enumerate_paths" "provider generate output missing fake action marker"
assert_log_not_contains "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/provider-generate.txt" "$OURO_AI_PROVIDER_SMOKE_API_KEY" "provider generate output leaked API key value"
USAGE_RECORD="$(awk -F= '/^usage_record=/ { print substr($0, 14); exit }' "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/provider-generate.txt")"
[ -n "$USAGE_RECORD" ] || fail "provider generate did not print usage_record"
assert_non_empty_file "provider usage record" "$USAGE_RECORD"
assert_log_not_contains "$USAGE_RECORD" "$OURO_AI_PROVIDER_SMOKE_API_KEY" "provider usage record leaked API key value"
printf 'provider-generate: ok\n'

run_ouro_logged "chat prompt" "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/chat-prompt.txt" chat "$WORKSPACE" --prompt "Return one reviewed semantic action marker."
assert_log_contains "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/chat-prompt.txt" "chat: status=completed provider=smoke-openai model=gpt-smoke tokens=12 stored=false" "chat prompt output missing completion status"
assert_log_contains "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/chat-prompt.txt" "chat-text: semantic_action: enumerate_paths" "chat prompt output missing fake action marker"
assert_log_contains "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/chat-prompt.txt" "chat-preview: status=created action=enumerate_paths marker=semantic_action: enumerate_paths artifact_created=true stored=false" "chat prompt output missing preview creation summary"
assert_log_not_contains "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/chat-prompt.txt" "$OURO_AI_PROVIDER_SMOKE_API_KEY" "chat prompt output leaked API key value"
PREVIEW_COUNT="$(find "$WORKSPACE/runtime/model-tool-plan-previews" -type f -name '*.json' 2>/dev/null | wc -l | tr -d ' ')"
[ "$PREVIEW_COUNT" = "1" ] || fail "chat prompt did not create exactly one model tool-plan preview artifact"
if [ -e "$WORKSPACE/replay/events.db" ]; then
	fail "chat prompt must not create replay event log"
fi
printf 'chat-prompt: preview=created stored=false\n'

run_ouro_logged "provider usage summary" "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/provider-usage-summary.txt" provider usage "$WORKSPACE" --summary
assert_log_contains "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/provider-usage-summary.txt" "provider-usage-summary:" "provider usage summary missing header"
assert_log_contains "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/provider-usage-summary.txt" "records=1" "provider usage summary did not include one record"
assert_log_contains "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/provider-usage-summary.txt" "providers=smoke-openai" "provider usage summary missing provider"
assert_log_not_contains "$OURO_AI_PROVIDER_SMOKE_LOG_DIR/provider-usage-summary.txt" "$OURO_AI_PROVIDER_SMOKE_API_KEY" "provider usage summary leaked API key value"
printf 'usage-summary: records=1\n'

printf 'ai-provider-smoke: ok\n'
