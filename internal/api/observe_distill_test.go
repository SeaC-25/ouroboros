package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/nativetools"
)

// TestDistillObservationForAICapsAndKeys 构造含 3KB body_preview、基线 diff、
// 表单/链接/JSON 键齐全的 ObserveResult，验证蒸馏输出 ≤600 字节且保留
// status/diff/forms/links/keys 关键字段，body 只取前 120 字节。
func TestDistillObservationForAICapsAndKeys(t *testing.T) {
	preview := strings.Repeat("HEAD", 30) + strings.Repeat("A", 2952) + "TAILMARKER"
	obs := nativetools.ObserveResult{
		URL:         "http://target.local/login",
		Method:      "POST",
		ContentType: "text/html",
		Status:      403,
		StatusText:  "Forbidden",
		BodySize:    4300,
		BodyPreview: preview,
		ElapsedMS:   85.4,
		Server:      "nginx/1.24.0 (Ubuntu)",
		XPoweredBy:  "PHP/8.2.1",
		Forms: []nativetools.ObserveForm{
			{Action: "/login", Method: "post", Fields: []string{"user", "pass"}},
		},
		Links:    make([]string, 12),
		JSONKeys: []string{"token", "user"},
		Diff: &nativetools.ObserveDiff{
			StatusChanged: true,
			ControlStatus: 200,
			SizeDelta:     256,
			ControlSize:   4044,
			BodyChanged:   true,
			HeaderAdded:   []string{"Set-Cookie"},
			HeaderRemoved: []string{"X-Powered-By"},
		},
	}
	raw, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("marshal observation: %v", err)
	}

	got := distillObservationForAI(string(raw))

	if len(got) > maxDistilledObservation {
		t.Fatalf("distilled summary exceeds %d bytes: %d", maxDistilledObservation, len(got))
	}
	for _, want := range []string{
		"status=403",
		"t=85ms",
		"len=4.2KB",
		"server=nginx", // first token only, no version noise
		"diff:",
		"status 200→403",
		"+Set-Cookie",
		"-X-Powered-By",
		"forms:/login(user,pass)",
		"links:12",
		"keys:token,user",
		strings.Repeat("HEAD", 30), // first 120 bytes of body_preview
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("distilled summary missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "TAILMARKER") || strings.Contains(got, strings.Repeat("A", 200)) {
		t.Fatalf("body_preview not capped to 120 bytes:\n%s", got)
	}
}

// TestDistillObservationForAIFirstSightFingerprint 无 diff（无基线）时展示
// 首指纹（server/xpb），不出现 diff 段。
func TestDistillObservationForAIFirstSightFingerprint(t *testing.T) {
	obs := nativetools.ObserveResult{
		URL:       "http://target.local/",
		Status:    200,
		ElapsedMS: 42,
		BodySize:  853,
		Server:    "nginx",
	}
	raw, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("marshal observation: %v", err)
	}
	got := distillObservationForAI(string(raw))
	if !strings.Contains(got, "status=200") || !strings.Contains(got, "len=853B") || !strings.Contains(got, "server=nginx") {
		t.Fatalf("missing fingerprint fields:\n%s", got)
	}
	if strings.Contains(got, "diff") {
		t.Fatalf("no-baseline observation must not contain diff section:\n%s", got)
	}
}

// TestDistillObservationForAIFailOpen 非法 JSON 不阻断，退回原文前 400 字节。
func TestDistillObservationForAIFailOpen(t *testing.T) {
	raw := "<html>" + strings.Repeat("x", 1000) + "</html>"
	got := distillObservationForAI(raw)
	if !strings.HasPrefix(got, "<html>") {
		t.Fatalf("fail-open must keep raw prefix:\n%s", got)
	}
	if len(got) > 400+len("\n...[已截断]") {
		t.Fatalf("fail-open fallback too long: %d", len(got))
	}
	if distillObservationForAI("") != "" {
		t.Fatalf("empty input must yield empty output")
	}
}
