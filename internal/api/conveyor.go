package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ============================================================================
// ACTION:BENCH — deterministic benchmark-platform conveyor (zero LLM).
//
// The model emits `ACTION:BENCH <op> [args]`; the handler calls the TSec
// Benchmark platform API directly over net/http, distills the response into
// a compact summary (NEVER raw JSON — feeding the raw body back into the
// model defeats the context budget this conveyor exists to save) and
// appends the untouched raw response to runtime/bench-log.jsonl for audit;
// the model can ACTION:READ that file when the distillate is not enough.
//
// Scope gate: BENCH targets the benchmark platform API itself, not a range
// target, so it deliberately does NOT pass through authorizeObserveTarget —
// that gate classifies scope.yaml range/infrastructure URLs and would reject
// the platform host. The only model-controlled injection surface is the
// string arguments, capped below (flag ≤ 4096 bytes) before being sent.
// ============================================================================

// benchFlagCap mirrors the official contract's flag length limit (1~4096).
const benchFlagCap = 4096

// benchPlatformClient is an instance hook for tests (httptest swaps in a
// client pinned to the test server); production uses the default 30s
// timeout client per contract.
var benchPlatformClient = &http.Client{Timeout: 30 * time.Second}

// benchAPIConfig resolves platform credentials from the environment.
// Unset variables fail fast with an actionable error: the model must not
// burn turns probing a platform it cannot reach.
func benchAPIConfig() (string, string, error) {
	baseURL := strings.TrimSpace(osGetenv("BENCHMARK_BASE_URL"))
	if baseURL == "" {
		return "", "", fmt.Errorf("未设置 BENCHMARK_BASE_URL 环境变量（平台 API 地址），无法执行 ACTION:BENCH；请先在部署环境中配置")
	}
	token := strings.TrimSpace(osGetenv("BENCHMARK_TOKEN"))
	if token == "" {
		return "", "", fmt.Errorf("未设置 BENCHMARK_TOKEN 环境变量（平台认证头），无法执行 ACTION:BENCH；请先在部署环境中配置")
	}
	return baseURL, token, nil
}

// benchLogPath returns the audit journal path inside the workspace.
func benchLogPath(workspace string) string {
	return filepath.Join(workspace, "runtime", "bench-log.jsonl")
}

// appendBenchLog appends one audit record (op/args/status/raw body) to
// runtime/bench-log.jsonl. The raw platform response lands here verbatim —
// this is the only place the full JSON survives; the model-visible summary
// is always the distilled form.
func (s *Server) appendBenchLog(op string, args string, status int, body string) {
	record := map[string]any{
		"ts":     time.Now().UTC().Format(time.RFC3339Nano),
		"op":     op,
		"args":   args,
		"status": status,
		"body":   body,
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		log.Printf("bench audit log marshal failed: %v", err)
		return
	}
	encoded = append(encoded, '\n')
	path := benchLogPath(s.workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("bench audit log mkdir failed: %v", err)
		return
	}
	// O_APPEND single write; concurrent BENCH actions must not interleave
	// partial JSONL lines (same invariant as the observations journal).
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("bench audit log open failed: %v", err)
		return
	}
	defer file.Close()
	if _, err := file.Write(encoded); err != nil {
		log.Printf("bench audit log write failed: %v", err)
	}
}

// benchRequest issues one platform API call and returns (status, body).
// Auth follows the official contract: header "BENCHMARK_TOKEN: <token>" on
// every request (NOT an Authorization header — the platform does not read
// one). Raw JSON bodies are sent with application/json.
func benchRequest(ctx context.Context, baseURL string, token string, method string, path string, body []byte, query url.Values) (int, string, error) {
	target := strings.TrimSuffix(baseURL, "/") + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return 0, "", fmt.Errorf("构造平台请求失败: %w", err)
	}
	req.Header.Set("BENCHMARK_TOKEN", token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := benchPlatformClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("平台请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, "", fmt.Errorf("读取平台响应失败: %w", err)
	}
	return resp.StatusCode, string(raw), nil
}

// benchExcerpt byte-caps an unusable body for error text. The raw JSON is
// already on the audit journal; the error only needs enough to classify.
func benchExcerpt(body string) string {
	excerpt := body
	if len(excerpt) > 200 {
		excerpt = excerpt[:200] + "…"
	}
	return fmt.Sprintf("原始响应见 runtime/bench-log.jsonl（前缀: %q）", excerpt)
}

// benchPlatformError renders a non-2xx platform business error
// {"code": ..., "message": ...} compactly, per the official contract's
// error table (task_not_found / invalid_state / duplicate / ...).
func benchPlatformError(status int, body string) error {
	var parsed struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err == nil && parsed.Code != "" {
		return fmt.Errorf("平台返回 HTTP %d %s: %s（详见 runtime/bench-log.jsonl）", status, parsed.Code, parsed.Message)
	}
	return fmt.Errorf("平台返回 HTTP %d（详见 runtime/bench-log.jsonl）", status)
}

// benchDistillList renders the list op as one compact line:
// "N 题｜未解 X｜easy 未解：c1,c2｜活跃：code@addr"
// Schema follows the official contract: the response is an ARRAY of
// challenges (unique_code/difficulty/is_completed/container_status/
// container_addr) — there is no wrapping object.
func benchDistillList(payload []byte) (string, error) {
	var challenges []struct {
		UniqueCode      string   `json:"unique_code"`
		Difficulty      string   `json:"difficulty"`
		IsCompleted     bool     `json:"is_completed"`
		ContainerStatus string   `json:"container_status"`
		ContainerAddr   []string `json:"container_addr"`
		Description     string   `json:"description"`
		FlagCount       int      `json:"flag_count"`
		TotalScore      int      `json:"total_score"`
	}
	if err := json.Unmarshal(payload, &challenges); err != nil {
		return "", fmt.Errorf("平台列表响应解析失败: %v; %s", err, benchExcerpt(string(payload)))
	}
	unsolved := 0
	var easyUnsolved []string
	var actives []string
	var activeDescs []string
	for _, ch := range challenges {
		if !ch.IsCompleted {
			unsolved++
			if strings.EqualFold(ch.Difficulty, "easy") {
				easyUnsolved = append(easyUnsolved, ch.UniqueCode)
			}
		}
		if ch.ContainerStatus == "available" && len(ch.ContainerAddr) > 0 {
			actives = append(actives, ch.UniqueCode+"@"+strings.Join(ch.ContainerAddr, "|"))
			desc := ch.Description
			if len(desc) > 90 {
				desc = desc[:90] + "…"
			}
			activeDescs = append(activeDescs, fmt.Sprintf("  %s（%s，%d分%d旗）：%s", ch.UniqueCode, ch.Difficulty, ch.TotalScore, ch.FlagCount, desc))
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d 题｜未解 %d", len(challenges), unsolved)
	if len(easyUnsolved) == 0 {
		b.WriteString("｜easy 未解：无")
	} else {
		joined := strings.Join(easyUnsolved, ",")
		if len(joined) > 120 {
			joined = joined[:120] + "…"
		}
		fmt.Fprintf(&b, "｜easy 未解：%s", joined)
	}
	if len(actives) > 0 {
		joined := strings.Join(actives, ",")
		if len(joined) > 120 {
			joined = joined[:120] + "…"
		}
		fmt.Fprintf(&b, "｜活跃：%s", joined)
		b.WriteString("\n活跃题目详情：")
		for _, d := range activeDescs {
			b.WriteString("\n" + d)
		}
	}
	return b.String(), nil
}

// benchDistillStart renders the start op: "code 已启动@addr".
// The start response carries unique_code + container_addr.
func benchDistillStart(payload []byte) (string, error) {
	var parsed struct {
		UniqueCode    string   `json:"unique_code"`
		ContainerAddr []string `json:"container_addr"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return "", fmt.Errorf("平台启动响应解析失败: %v; %s", err, benchExcerpt(string(payload)))
	}
	addr := strings.Join(parsed.ContainerAddr, "|")
	return fmt.Sprintf("%s 已启动@%s", parsed.UniqueCode, addr), nil
}

// benchDistillSubmit renders the submit op:
// "correct=true/false awarded=N 累计=N"
// Schema per contract: correct / awarded / cumulative_score.
func benchDistillSubmit(payload []byte) (string, error) {
	var parsed struct {
		Correct         bool `json:"correct"`
		Awarded         int  `json:"awarded"`
		CumulativeScore int  `json:"cumulative_score"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return "", fmt.Errorf("平台提交响应解析失败: %v; %s", err, benchExcerpt(string(payload)))
	}
	if parsed.Correct {
		// Terminal directive on success: without it the loop kept hunting
		// flags on an already-solved challenge and burned duplicate (always
		// wrong) submissions. d-02/f1-04 both showed correct-then-2-wrongs.
		return fmt.Sprintf("correct=true awarded=%d 累计=%d ——【此题已破解】立即收工：执行 ACTION:BENCH close 收尾，然后单独一行 DONE 结束循环。不要再寻找或提交任何其他 flag。", parsed.Awarded, parsed.CumulativeScore), nil
	}
	return fmt.Sprintf("correct=false awarded=%d —— 提交错误：这个 flag 值不对（或此题已满分）。若此题已破解请立即 close+DONE；否则换方向重新取证，禁止重交同值。", parsed.Awarded), nil
}

// benchDistillClose renders the close op: "code 已关闭" (closed=true),
// or an explicit "closed=false" when the platform refuses.
func benchDistillClose(payload []byte) (string, error) {
	var parsed struct {
		UniqueCode string `json:"unique_code"`
		Closed     bool   `json:"closed"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return "", fmt.Errorf("平台关闭响应解析失败: %v; %s", err, benchExcerpt(string(payload)))
	}
	if !parsed.Closed {
		return fmt.Sprintf("%s 未关闭（closed=false）", parsed.UniqueCode), nil
	}
	return fmt.Sprintf("%s 已关闭", parsed.UniqueCode), nil
}

// benchDistillHint renders the hint op: the hint text, byte-capped at 300
// with an explicit truncation marker. The platform may return hint:null.
func benchDistillHint(payload []byte) (string, error) {
	var parsed struct {
		Hint *string `json:"hint"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return "", fmt.Errorf("平台提示响应解析失败: %v; %s", err, benchExcerpt(string(payload)))
	}
	if parsed.Hint == nil {
		return "（该题无提示）", nil
	}
	hint := strings.TrimSpace(*parsed.Hint)
	if hint == "" {
		return "（该题无提示）", nil
	}
	const capBytes = 300
	if len(hint) > capBytes {
		// Byte-slice on a UTF-8 boundary to avoid splitting a rune mid-sequence.
		cut := capBytes
		for cut > 0 && !utf8ValidSuffix([]byte(hint[cut-1:cut])) {
			cut--
		}
		hint = hint[:cut] + "…[已截断]"
	}
	return hint, nil
}

// utf8ValidSuffix reports whether b begins a continuation-free rune start
// (i.e. is NOT a continuation byte 0b10xxxxxx).
func utf8ValidSuffix(b []byte) bool {
	return len(b) > 0 && b[0]&0xC0 != 0x80
}

// executeBenchAction is the ACTION:BENCH entry point: parses
// "<op> [args]", validates args, calls the platform, distills, and logs.
// Returned error text is the model-visible failure summary (structured,
// never raw JSON).
func (s *Server) executeBenchAction(spec string) (string, error) {
	fields := strings.Fields(strings.TrimSpace(spec))
	if len(fields) == 0 {
		return "", fmt.Errorf("ACTION:BENCH 需要 <op> 参数（list/start/submit/close/hint）")
	}
	op := fields[0]

	baseURL, token, cfgErr := benchAPIConfig()
	if cfgErr != nil {
		return "", cfgErr
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// arg returns the n-th argument after the op ("" when absent).
	arg := func(n int) string {
		if n < len(fields) {
			return fields[n]
		}
		return ""
	}

	switch op {
	case "list":
		status, body, err := benchRequest(ctx, baseURL, token, http.MethodGet, "/openapi/v1/challenges", nil, nil)
		if err != nil {
			return "", err
		}
		s.appendBenchLog(op, "", status, body)
		if status != http.StatusOK {
			return "", benchPlatformError(status, body)
		}
		return benchDistillList([]byte(body))

	case "start":
		code := arg(1)
		if code == "" {
			return "", fmt.Errorf("ACTION:BENCH start 需要 <unique_code> 参数（如 ACTION:BENCH start e1-05）")
		}
		status, body, err := benchRequest(ctx, baseURL, token, http.MethodPost,
			"/openapi/v1/challenges/start", nil, url.Values{"unique_code": {code}})
		if err != nil {
			return "", err
		}
		s.appendBenchLog(op, code, status, body)
		if status != http.StatusOK && status != http.StatusCreated {
			return "", benchPlatformError(status, body)
		}
		return benchDistillStart([]byte(body))

	case "submit":
		code := arg(1)
		flag := arg(2)
		if code == "" || flag == "" {
			return "", fmt.Errorf("ACTION:BENCH submit 需要 <unique_code> <flag> 参数")
		}
		// Injection cap: the flag is model-authored text about to be sent
		// to the platform; the contract allows 1~4096, so anything larger
		// is refused without a network round-trip.
		if len(flag) > benchFlagCap {
			return "", fmt.Errorf("flag 参数超过 %d 字节上限，拒绝提交（契约允许 1~4096）", benchFlagCap)
		}
		payload, err := json.Marshal(map[string]string{"unique_code": code, "flag": flag})
		if err != nil {
			return "", fmt.Errorf("构造提交请求失败: %w", err)
		}
		status, body, reqErr := benchRequest(ctx, baseURL, token, http.MethodPost,
			"/openapi/v1/challenges/submit", payload, nil)
		if reqErr != nil {
			return "", reqErr
		}
		s.appendBenchLog(op, code+" <flag>", status, body)
		if status != http.StatusOK {
			return "", benchPlatformError(status, body)
		}
		// Record rejections for the circuit breaker (correct=false).
		var verdict struct {
			Correct bool `json:"correct"`
		}
		if json.Unmarshal([]byte(body), &verdict) == nil && !verdict.Correct {
			s.rejectedFlagsMu.Lock()
			if s.rejectedFlags == nil {
				s.rejectedFlags = map[string]bool{}
			}
			s.rejectedFlags[flag] = true
			s.rejectedFlagsMu.Unlock()
		}
		return benchDistillSubmit([]byte(body))

	case "close":
		code := arg(1)
		if code == "" {
			return "", fmt.Errorf("ACTION:BENCH close 需要 <unique_code> 参数")
		}
		status, body, err := benchRequest(ctx, baseURL, token, http.MethodPost,
			"/openapi/v1/challenges/close", nil, url.Values{"unique_code": {code}})
		if err != nil {
			return "", err
		}
		s.appendBenchLog(op, code, status, body)
		if status != http.StatusOK && status != http.StatusCreated {
			return "", benchPlatformError(status, body)
		}
		return benchDistillClose([]byte(body))

	case "hint":
		code := arg(1)
		if code == "" {
			return "", fmt.Errorf("ACTION:BENCH hint 需要 <unique_code> 参数")
		}
		status, body, err := benchRequest(ctx, baseURL, token, http.MethodGet,
			"/openapi/v1/challenges/hint", nil, url.Values{"unique_code": {code}})
		if err != nil {
			return "", err
		}
		s.appendBenchLog(op, code, status, body)
		if status != http.StatusOK {
			return "", benchPlatformError(status, body)
		}
		return benchDistillHint([]byte(body))

	default:
		return "", fmt.Errorf("未知 ACTION:BENCH op: %q（支持 list/start/submit/close/hint）", op)
	}
}

// flagBackedByObservation reports whether the flag appears verbatim in the
// raw observation journal (evidence-first submission policy).
func (s *Server) flagBackedByObservation(flag string) bool {
	if s == nil || strings.TrimSpace(flag) == "" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(s.workspace, "runtime", "observations.jsonl"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), flag)
}

// flagKnownRejected reports whether the platform already rejected this flag.
func (s *Server) flagKnownRejected(flag string) bool {
	if s == nil {
		return false
	}
	s.rejectedFlagsMu.Lock()
	defer s.rejectedFlagsMu.Unlock()
	if s.rejectedFlags == nil {
		s.rejectedFlags = map[string]bool{}
	}
	return s.rejectedFlags[flag]
}
