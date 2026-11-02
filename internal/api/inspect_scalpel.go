package api

// inspect_scalpel.go — ACTION:INSPECT surgical primitive (v28.1).
//
// The 600B distilled summary necessarily hides response details; models used
// to READ the full observations.jsonl (20KB+) to recover them. INSPECT gives
// a bounded slice of ONE observation on demand:
//
//	ACTION:INSPECT <obs-id> body:bytes=500..2000
//	ACTION:INSPECT <obs-id> body:regex=<pattern>   (matches + ±80B context, max 5)
//	ACTION:INSPECT <obs-id> headers                (full headers/cookies/redirects)
//
// Output hard-capped at 1.5KB. The tool stays zero-verdict: WHERE to look
// and WHAT pattern to match are the model's decisions.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const inspectMaxOutput = 1500

// executeInspectAction parses "<obs-id> <selector...>" and renders the slice.
func (s *Server) executeInspectAction(args string) (string, error) {
	parts := strings.SplitN(strings.TrimSpace(args), " ", 2)
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", fmt.Errorf("格式: ACTION:INSPECT <obs-id> <selector>（selector: body:bytes=A..B / body:regex=<正则> / headers）")
	}
	obsID := strings.TrimSpace(parts[0])
	selectors := ""
	if len(parts) == 2 {
		selectors = strings.TrimSpace(parts[1])
	}
	if selectors == "" {
		return "", fmt.Errorf("缺少 selector：body:bytes=A..B / body:regex=<正则> / headers（可空格组合）")
	}

	record, err := s.loadObservationRecord(obsID)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	for _, sel := range strings.Fields(selectors) {
		switch {
		case sel == "headers":
			out.WriteString(renderInspectHeaders(record))
		case strings.HasPrefix(sel, "body:bytes="):
			span := strings.TrimPrefix(sel, "body:bytes=")
			lo, hi, ok := parseByteSpan(span)
			if !ok {
				return "", fmt.Errorf("bytes 区间格式: body:bytes=500..2000，收到 %q", span)
			}
			out.WriteString(renderInspectBytes(record.BodyPreview, lo, hi))
		case strings.HasPrefix(sel, "body:regex="):
			pattern := strings.TrimPrefix(sel, "body:regex=")
			pattern = strings.Trim(pattern, "\"'")
			out.WriteString(renderInspectRegex(record.BodyPreview, pattern))
		default:
			return "", fmt.Errorf("未知 selector %q（支持 body:bytes= / body:regex= / headers）", sel)
		}
		out.WriteString("\n")
	}
	result := strings.TrimSpace(out.String())
	if len(result) > inspectMaxOutput {
		result = result[:inspectMaxOutput] + "\n[已截断，可换区间/更窄正则继续 INSPECT]"
	}
	return result, nil
}

// inspectObservation is the ObserveResult subset INSPECT operates on.
type inspectObservation struct {
	URL             string              `json:"url"`
	Method          string              `json:"method"`
	Status          int                 `json:"status"`
	ResponseHeaders map[string][]string `json:"response_headers"`
	SetCookies      []string            `json:"set_cookies,omitempty"`
	Location        string              `json:"location,omitempty"`
	RedirectChain   []string            `json:"redirect_chain,omitempty"`
	BodyPreview     string              `json:"body_preview,omitempty"`
	BodySize        int                 `json:"body_size"`
}

// loadObservationRecord scans observations.jsonl for the obs-id.
func (s *Server) loadObservationRecord(obsID string) (*inspectObservation, error) {
	path := filepath.Join(s.workspace, "runtime", "observations.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取观测日志失败: %v", err)
	}
	var latest *inspectObservation
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var envelope struct {
			ID      string `json:"id"`
			Payload string `json:"payload"`
		}
		if json.Unmarshal([]byte(line), &envelope) != nil {
			continue
		}
		if envelope.ID != obsID {
			continue
		}
		var obs inspectObservation
		if json.Unmarshal([]byte(envelope.Payload), &obs) == nil {
			latest = &obs // keep the last occurrence
		}
	}
	if latest == nil {
		return nil, fmt.Errorf("观测 %s 不存在（用 ACTION:COLLECT 或黑板事实确认 ID）", obsID)
	}
	return latest, nil
}

// renderInspectHeaders renders the full untruncated header surface.
func renderInspectHeaders(obs *inspectObservation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s -> %d\n", obs.Method, obs.URL, obs.Status)
	keys := make([]string, 0, len(obs.ResponseHeaders))
	for k := range obs.ResponseHeaders {
		keys = append(keys, k)
	}
	sortStrings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "%s: %s\n", k, strings.Join(obs.ResponseHeaders[k], "; "))
	}
	if len(obs.SetCookies) > 0 {
		b.WriteString("Set-Cookie:\n")
		for _, c := range obs.SetCookies {
			b.WriteString("  " + c + "\n")
		}
	}
	if obs.Location != "" {
		b.WriteString("Location: " + obs.Location + "\n")
	}
	if len(obs.RedirectChain) > 0 {
		b.WriteString("RedirectChain: " + strings.Join(obs.RedirectChain, " -> ") + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderInspectBytes extracts a byte window from the preview.
func renderInspectBytes(preview string, lo, hi int) string {
	if preview == "" {
		return "body: 预览为空（BodySize=" + strconv.Itoa(hi*0) + "）"
	}
	runes := []byte(preview)
	if lo < 0 {
		lo = 0
	}
	if hi > len(runes) {
		hi = len(runes)
	}
	if lo >= hi {
		return fmt.Sprintf("body: 区间 [%d..%d) 超出预览长度 %d", lo, hi, len(runes))
	}
	return fmt.Sprintf("body[%d..%d): %s", lo, hi, string(runes[lo:hi]))
}

// renderInspectRegex extracts matches with ±80 bytes of context (max 5).
func renderInspectRegex(preview, pattern string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "regex 编译失败: " + err.Error()
	}
	matches := re.FindAllStringIndex(preview, 5)
	if len(matches) == 0 {
		return "regex 无匹配（预览长度 " + strconv.Itoa(len(preview)) + "）"
	}
	var b strings.Builder
	for i, m := range matches {
		lo := m[0] - 80
		if lo < 0 {
			lo = 0
		}
		hi := m[1] + 80
		if hi > len(preview) {
			hi = len(preview)
		}
		fmt.Fprintf(&b, "match%d @%d: ...%s...\n", i+1, m[0], preview[lo:hi])
	}
	return strings.TrimRight(b.String(), "\n")
}

// parseByteSpan parses "500..2000".
func parseByteSpan(span string) (int, int, bool) {
	dots := strings.Index(span, "..")
	if dots < 0 {
		return 0, 0, false
	}
	lo, err1 := strconv.Atoi(strings.TrimSpace(span[:dots]))
	hi, err2 := strconv.Atoi(strings.TrimSpace(span[dots+2:]))
	if err1 != nil || err2 != nil || lo < 0 || hi < lo {
		return 0, 0, false
	}
	return lo, hi, true
}

// sortStrings is a tiny insertion sort (avoids importing sort for 3 uses).
func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

// deniedJournalRead returns the journal name when the path targets a system
// journal the model must not pull wholesale (redirect to INSPECT/QUERY).
func deniedJournalRead(relPath string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(relPath), "\\", "/")
	lowered := strings.ToLower(normalized)
	if !strings.HasSuffix(lowered, ".jsonl") {
		return ""
	}
	for _, journal := range []string{
		"observations.jsonl", "blackboard.jsonl", "subloops.jsonl",
		"ai-conversation.jsonl", "bench-log.jsonl", "goals.jsonl",
	} {
		if strings.HasSuffix(lowered, journal) {
			return journal
		}
	}
	return "系统 jsonl"
}
