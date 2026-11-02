package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// benchTestEnv sets BENCHMARK_BASE_URL/BENCHMARK_TOKEN for one test.
func benchTestEnv(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("BENCHMARK_BASE_URL", baseURL)
	t.Setenv("BENCHMARK_TOKEN", "test-token")
}

// benchTestServer builds a Server whose workspace is a temp dir, with the
// platform client pinned to the mock platform. Tests call
// server.executeBenchAction directly (the parseAndExecuteAIActions dispatch
// is covered by the registration assertion test below).
func benchTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{workspace: t.TempDir()}
}

// benchReadAuditLog returns the raw audit journal lines.
func benchReadAuditLog(t *testing.T, server *Server) []string {
	t.Helper()
	data, err := os.ReadFile(benchLogPath(server.workspace))
	if err != nil {
		t.Fatalf("read bench audit log: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// =====================================================================
// list
// =====================================================================

func TestBenchListDistilled(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/v1/challenges" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("BENCHMARK_TOKEN") != "test-token" {
			t.Errorf("auth header missing: %q", r.Header.Get("BENCHMARK_TOKEN"))
		}
		// Official contract shape: a bare array of challenges.
		fmt.Fprint(w, `[
			{"unique_code":"e1-01","difficulty":"easy","is_completed":true,"container_status":"stopped","container_addr":[]},
			{"unique_code":"e1-02","difficulty":"easy","is_completed":false,"container_status":"stopped","container_addr":[]},
			{"unique_code":"e1-03","difficulty":"easy","is_completed":false,"container_status":"available","container_addr":["10.0.0.5:8001"]},
			{"unique_code":"e1-04","difficulty":"hard","is_completed":false,"container_status":"stopped","container_addr":[]}
		]`)
	}))
	defer mock.Close()
	benchTestEnv(t, mock.URL)
	server := benchTestServer(t)

	summary, err := server.executeBenchAction("list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// 4 题｜未解 3｜easy 未解：e1-02,e1-03｜活跃：e1-03@10.0.0.5:8001
	// The distillation now appends per-active descriptions (upgrade):
	// assert the stable prefix + the detail line presence.
	want := "4 题｜未解 3｜easy 未解：e1-02,e1-03｜活跃：e1-03@10.0.0.5:8001"
	if !strings.HasPrefix(summary, want) || !strings.Contains(summary, "活跃题目详情") {
		t.Fatalf("distilled list mismatch:\n got: %q\nwant: %q", summary, want)
	}
	// 红线：蒸馏输出不得回灌原始 JSON（无大括号/方括号结构残留）。
	if strings.ContainsAny(summary, "[]{}") {
		t.Fatalf("distilled list leaked raw JSON structure: %q", summary)
	}
}

// =====================================================================
// start
// =====================================================================

func TestBenchStartDistilled(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/openapi/v1/challenges/start" {
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("unique_code") != "e1-05" {
			t.Errorf("unique_code query missing: %q", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"unique_code":"e1-05","container_addr":["10.0.0.9:9000"]}`)
	}))
	defer mock.Close()
	benchTestEnv(t, mock.URL)
	server := benchTestServer(t)

	summary, err := server.executeBenchAction("start e1-05")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	want := "e1-05 已启动@10.0.0.9:9000"
	if summary != want {
		t.Fatalf("distilled start mismatch:\n got: %q\nwant: %q", summary, want)
	}
	if strings.ContainsAny(summary, "[]{}") {
		t.Fatalf("distilled start leaked raw JSON structure: %q", summary)
	}
}

func TestBenchStartConflict(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"code":"invalid_state","message":"max active challenges reached","detail":{}}`)
	}))
	defer mock.Close()
	benchTestEnv(t, mock.URL)
	server := benchTestServer(t)

	_, err := server.executeBenchAction("start e1-06")
	if err == nil {
		t.Fatal("expected error on 409")
	}
	if !strings.Contains(err.Error(), "409") || !strings.Contains(err.Error(), "invalid_state") {
		t.Fatalf("structured platform error expected, got: %v", err)
	}
}

// =====================================================================
// submit
// =====================================================================

func TestBenchSubmitDistilled(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/openapi/v1/challenges/submit" {
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode submit body: %v", err)
		}
		if body["unique_code"] != "e1-02" || body["flag"] != "flag{abc}" {
			t.Errorf("submit body mismatch: %v", body)
		}
		fmt.Fprint(w, `{"correct":true,"awarded":100,"cumulative_score":300,"correct_flag_count":1,"total_flag_count":3,"matched_flag_index":0}`)
	}))
	defer mock.Close()
	benchTestEnv(t, mock.URL)
	server := benchTestServer(t)

	summary, err := server.executeBenchAction("submit e1-02 flag{abc}")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	want := "correct=true awarded=100 累计=300 ——【此题已破解】"
	if !strings.HasPrefix(summary, want) {
		t.Fatalf("distilled submit mismatch:\n got: %q\nwant: %q", summary, want)
	}
	if strings.ContainsAny(summary, "[]{}") {
		t.Fatalf("distilled submit leaked raw JSON structure: %q", summary)
	}
}

func TestBenchSubmitWrongFlag(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"correct":false,"awarded":0,"cumulative_score":0,"correct_flag_count":0,"total_flag_count":3,"matched_flag_index":null}`)
	}))
	defer mock.Close()
	benchTestEnv(t, mock.URL)
	server := benchTestServer(t)

	summary, err := server.executeBenchAction("submit e1-02 wrong")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if !strings.HasPrefix(summary, "correct=false awarded=0") {
		t.Fatalf("wrong-flag distill mismatch: %q", summary)
	}
}

func TestBenchSubmitFlagCap(t *testing.T) {
	server := benchTestServer(t)
	benchTestEnv(t, "http://127.0.0.1:1") // must fail before any request
	longFlag := strings.Repeat("a", 4097)
	if _, err := server.executeBenchAction("submit e1-02 " + longFlag); err == nil {
		t.Fatal("expected flag-cap rejection")
	}
}

// =====================================================================
// close
// =====================================================================

func TestBenchCloseDistilled(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/openapi/v1/challenges/close" {
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("unique_code") != "e1-05" {
			t.Errorf("unique_code query missing: %q", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"unique_code":"e1-05","closed":true}`)
	}))
	defer mock.Close()
	benchTestEnv(t, mock.URL)
	server := benchTestServer(t)

	summary, err := server.executeBenchAction("close e1-05")
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	want := "e1-05 已关闭"
	if summary != want {
		t.Fatalf("distilled close mismatch:\n got: %q\nwant: %q", summary, want)
	}
	if strings.ContainsAny(summary, "[]{}") {
		t.Fatalf("distilled close leaked raw JSON structure: %q", summary)
	}
}

// =====================================================================
// hint
// =====================================================================

func TestBenchHintDistilledAndCapped(t *testing.T) {
	longHint := strings.Repeat("提示", 300) // 1800 bytes of UTF-8
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/v1/challenges/hint" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		payload, _ := json.Marshal(map[string]string{"unique_code": "e1-02", "hint": longHint})
		fmt.Fprint(w, string(payload))
	}))
	defer mock.Close()
	benchTestEnv(t, mock.URL)
	server := benchTestServer(t)

	summary, err := server.executeBenchAction("hint e1-02")
	if err != nil {
		t.Fatalf("hint: %v", err)
	}
	if !strings.HasSuffix(summary, "…[已截断]") {
		t.Fatalf("hint truncation marker missing: %q", summary)
	}
	// The distillate must stay under the 300-byte cap + marker, and never
	// include the full 1800-byte raw JSON payload.
	if len(summary) > 300+len("…[已截断]")+3 {
		t.Fatalf("hint exceeds byte cap: %d bytes", len(summary))
	}
	if strings.Contains(summary, `{"unique_code"`) {
		t.Fatalf("hint leaked raw JSON: %q", summary)
	}
	if !strings.HasPrefix(summary, "提示") {
		t.Fatalf("hint body mismatch: %q", summary)
	}
}

func TestBenchHintNull(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"unique_code":"e1-02","hint":null}`)
	}))
	defer mock.Close()
	benchTestEnv(t, mock.URL)
	server := benchTestServer(t)

	summary, err := server.executeBenchAction("hint e1-02")
	if err != nil {
		t.Fatalf("hint: %v", err)
	}
	if summary != "（该题无提示）" {
		t.Fatalf("null-hint distill mismatch: %q", summary)
	}
}

// =====================================================================
// config / audit / dispatch
// =====================================================================

func TestBenchMissingConfig(t *testing.T) {
	server := benchTestServer(t)
	t.Setenv("BENCHMARK_BASE_URL", "")
	t.Setenv("BENCHMARK_TOKEN", "")
	if _, err := server.executeBenchAction("list"); err == nil {
		t.Fatal("expected config error")
	} else if !strings.Contains(err.Error(), "BENCHMARK_BASE_URL") {
		t.Fatalf("config error should name the missing variable: %v", err)
	}
}

func TestBenchUnknownOp(t *testing.T) {
	server := benchTestServer(t)
	benchTestEnv(t, "http://127.0.0.1:1")
	if _, err := server.executeBenchAction("pwn e1-02"); err == nil {
		t.Fatal("expected unknown-op error")
	}
}

func TestBenchAuditLogRecordsRawResponse(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"unique_code":"e1-05","container_addr":["10.0.0.9:9000"]}`)
	}))
	defer mock.Close()
	benchTestEnv(t, mock.URL)
	server := benchTestServer(t)

	if _, err := server.executeBenchAction("start e1-05"); err != nil {
		t.Fatalf("start: %v", err)
	}
	lines := benchReadAuditLog(t, server)
	if len(lines) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(lines))
	}
	var record struct {
		Op     string `json:"op"`
		Args   string `json:"args"`
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("audit record not JSONL: %v", err)
	}
	if record.Op != "start" || record.Status != 200 {
		t.Fatalf("audit record fields wrong: %+v", record)
	}
	// The raw response (with JSON braces) must survive verbatim in the log —
	// distillation applies only to what the model sees back.
	if !strings.Contains(record.Body, `"container_addr"`) {
		t.Fatalf("audit log lost raw body: %q", record.Body)
	}
}

func TestBenchRegisteredInParseAndExecuteAIActions(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
			{"unique_code":"e1-01","difficulty":"easy","is_completed":false,"container_status":"stopped","container_addr":[]}
		]`)
	}))
	defer mock.Close()
	benchTestEnv(t, mock.URL)
	server := benchTestServer(t)

	refs, results := server.parseAndExecuteAIActions("ACTION:BENCH list")
	if len(results) != 1 {
		t.Fatalf("expected one bench result, got %d: %+v", len(results), results)
	}
	if results[0].Kind != "bench" {
		t.Fatalf("expected kind bench, got %q", results[0].Kind)
	}
	if !strings.Contains(results[0].Summary, "1 题｜未解 1") {
		t.Fatalf("dispatch did not return distilled list: %q", results[0].Summary)
	}
	if len(refs) != 0 {
		t.Fatalf("bench should not append refs, got %v", refs)
	}
}

func TestBenchSystemPromptTeachesAction(t *testing.T) {
	// The teaching line must stay in the chat system prompt; grep the source
	// text via the compiled string to avoid building a full Server.
	workspace := t.TempDir()
	server := &Server{workspace: workspace}
	// buildWorkspaceSummary is exercised elsewhere; here we assert the
	// prompt template source directly through the chat path constant.
	_ = server
	src, err := os.ReadFile(filepath.Join("ai_chat.go"))
	if err != nil {
		t.Skipf("source not available: %v", err)
	}
	if !strings.Contains(string(src), "ACTION:BENCH <op> [args]") {
		t.Fatal("system prompt no longer teaches ACTION:BENCH")
	}
}
