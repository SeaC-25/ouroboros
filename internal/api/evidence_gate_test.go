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

// writeScopeAllowAll writes a scope.yaml that passes the observe gate for
// the given test server URL (target_policy disabled = operator opt-out).
// Infrastructure entries stay enforced, but a bare httptest server is not
// one.
func writeScopeAllowAll(t *testing.T, workspace string) {
	t.Helper()
	scope := "version: 1\n" +
		"scope:\n" +
		"  target_policy: disabled\n" +
		"  targets: []\n" +
		"  exclusions: []\n" +
		"  infrastructure: []\n"
	if err := os.WriteFile(filepath.Join(workspace, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope.yaml: %v", err)
	}
}

// appendObservationJournal writes one raw-observation record into
// runtime/observations.jsonl whose payload is an ObserveResult JSON for the
// given URL — the shape ACTION:OBSERVE produces.
func appendObservationJournal(t *testing.T, workspace string, obsID string, targetURL string) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"url":    targetURL,
		"method": "GET",
		"status": 200,
	})
	if err != nil {
		t.Fatalf("marshal observe payload: %v", err)
	}
	record, err := json.Marshal(map[string]any{
		"id":        obsID,
		"kind":      "http_observe",
		"target":    targetURL,
		"payload":   string(payload),
		"source":    "ai-chat",
		"createdAt": 1000,
	})
	if err != nil {
		t.Fatalf("marshal observation record: %v", err)
	}
	runtimeDir := filepath.Join(workspace, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	f, err := os.OpenFile(filepath.Join(runtimeDir, "observations.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open observations journal: %v", err)
	}
	defer f.Close()
	if _, err := f.Write(append(record, '\n')); err != nil {
		t.Fatalf("append observation: %v", err)
	}
}

// appendTestFact writes a BLACKBOARD-shaped fact line into
// runtime/blackboard.jsonl with the given evidence level and back-link.
func appendTestFact(t *testing.T, workspace string, factID string, statement string, evidenceLevel int, obsIDs []string) {
	t.Helper()
	record, err := json.Marshal(map[string]any{
		"id":                  factID,
		"kind":                "ai",
		"statement":           statement,
		"source":              "ai-agent",
		"source_observations": obsIDs,
		"evidence_level":      evidenceLevel,
		"createdAt":           1000,
	})
	if err != nil {
		t.Fatalf("marshal fact: %v", err)
	}
	runtimeDir := filepath.Join(workspace, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	f, err := os.OpenFile(filepath.Join(runtimeDir, "blackboard.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open blackboard journal: %v", err)
	}
	defer f.Close()
	if _, err := f.Write(append(record, '\n')); err != nil {
		t.Fatalf("append fact: %v", err)
	}
}

// readJournalFacts loads every fact from the workspace blackboard journal.
func readJournalFacts(t *testing.T, workspace string) []BlackboardFact {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(workspace, "runtime", "blackboard.jsonl"))
	if err != nil {
		t.Fatalf("read blackboard journal: %v", err)
	}
	var facts []BlackboardFact
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var fact BlackboardFact
		if json.Unmarshal([]byte(line), &fact) != nil {
			t.Fatalf("corrupt blackboard line: %q", line)
		}
		facts = append(facts, fact)
	}
	return facts
}

// TestVerifyPromotesBacklinkedFact covers the full gate chain: OBSERVE-style
// observation lands, BLACKBOARD writes a tentative fact citing it, VERIFY
// replays the observation, the signal reproduces, and the fact is promoted
// to E3 in the journal.
func TestVerifyPromotesBacklinkedFact(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("replay must default to GET, got %s", r.Method)
		}
		fmt.Fprint(w, "LOGIN portal: credentials admin:secret123 accepted")
	}))
	defer mock.Close()

	workspace := t.TempDir()
	writeScopeAllowAll(t, workspace)
	server := &Server{workspace: workspace}
	appendObservationJournal(t, workspace, "obs-1000-abcd", mock.URL)
	appendTestFact(t, workspace, "fact-1", "凭据 admin:secret123 可用 [obs-1000-abcd]", 2, []string{"obs-1000-abcd"})

	summary, err := server.executeVerifyAction("obs-1000-abcd", "credentials admin:secret123 accepted")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !strings.Contains(summary, "验证通过") || !strings.Contains(summary, "1 条结论晋升 E3") {
		t.Fatalf("verify summary mismatch: %q", summary)
	}
	facts := readJournalFacts(t, workspace)
	if len(facts) != 1 || facts[0].ID != "fact-1" || facts[0].EvidenceLevel != 3 {
		t.Fatalf("expected fact-1 promoted to E3, got %+v", facts)
	}
}

// TestVerifyMissedSignalKeepsTentative covers the failure branch: the replay
// succeeds but the expected signal is absent — the fact must stay at E2.
func TestVerifyMissedSignalKeepsTentative(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ACCESS DENIED for user admin")
	}))
	defer mock.Close()

	workspace := t.TempDir()
	writeScopeAllowAll(t, workspace)
	server := &Server{workspace: workspace}
	appendObservationJournal(t, workspace, "obs-2000-abcd", mock.URL)
	appendTestFact(t, workspace, "fact-2", "凭据 admin:secret123 可用 [obs-2000-abcd]", 2, []string{"obs-2000-abcd"})

	summary, err := server.executeVerifyAction("obs-2000-abcd", "credentials admin:secret123 accepted")
	if err != nil {
		t.Fatalf("verify must not error on a missed signal: %v", err)
	}
	if !strings.Contains(summary, "验证失败：信号未复现") {
		t.Fatalf("verify summary mismatch: %q", summary)
	}
	facts := readJournalFacts(t, workspace)
	if len(facts) != 1 || facts[0].EvidenceLevel != 2 {
		t.Fatalf("expected fact-2 to stay tentative E2, got %+v", facts)
	}
}

// TestLegacyFactWithoutEvidenceFieldLoads covers journal compatibility: a
// blackboard.jsonl line without evidence_level loads fine and decodes as 0.
func TestLegacyFactWithoutEvidenceFieldLoads(t *testing.T) {
	workspace := t.TempDir()
	runtimeDir := filepath.Join(workspace, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	legacy := `{"id":"f1","statement":"旧结论","createdAt":500}` + "\n"
	if err := os.WriteFile(filepath.Join(runtimeDir, "blackboard.jsonl"), []byte(legacy), 0o644); err != nil {
		t.Fatalf("write blackboard: %v", err)
	}
	server := &Server{workspace: workspace}
	facts := server.loadBlackboardFacts()
	if len(facts) != 1 {
		t.Fatalf("expected one legacy fact, got %d", len(facts))
	}
	if facts[0].EvidenceLevel != 0 {
		t.Fatalf("legacy fact must default to evidence level 0, got %d", facts[0].EvidenceLevel)
	}
	if facts[0].ID != "f1" || facts[0].Statement != "旧结论" {
		t.Fatalf("legacy fact fields must survive the round-trip: %+v", facts[0])
	}
}

// TestVerifyUnknownObservIDErrors covers the structured-error branch: a
// VERIFY against an observation ID absent from the journal reports an error
// instead of panicking.
func TestVerifyUnknownObservIDErrors(t *testing.T) {
	workspace := t.TempDir()
	writeScopeAllowAll(t, workspace)
	server := &Server{workspace: workspace}

	// Journal exists but holds no matching record: structured not-found error.
	appendObservationJournal(t, workspace, "obs-0000-other", "http://10.255.255.1/unused")
	_, err := server.executeVerifyAction("obs-9999-beef", "any signal")
	if err == nil {
		t.Fatalf("expected structured error for unknown observation id")
	}
	if !strings.Contains(err.Error(), "obs-9999-beef 不存在") {
		t.Fatalf("unexpected error text: %v", err)
	}
}

// TestAppendBlackboardFactDefaultsTentative covers the E2 default: a fact
// written through the chat BLACKBOARD path carries evidence_level=2 in the
// journal.
func TestAppendBlackboardFactDefaultsTentative(t *testing.T) {
	workspace := t.TempDir()
	server := &Server{workspace: workspace}

	fact, err := server.appendBlackboardFact("ai", "目标存在 SQL 注入 [obs-3000-abcd]")
	if err != nil {
		t.Fatalf("append fact: %v", err)
	}
	if fact.EvidenceLevel != 2 {
		t.Fatalf("BLACKBOARD fact must start tentative (2), got %d", fact.EvidenceLevel)
	}
	journaled := readJournalFacts(t, workspace)
	if len(journaled) != 1 || journaled[0].EvidenceLevel != 2 {
		t.Fatalf("journal must record evidence_level=2, got %+v", journaled)
	}
}

// TestVerifyActionParseAndExecute covers the ACTION:VERIFY line parsing
// end-to-end through parseAndExecuteAIActions, including a signal that
// contains spaces.
func TestVerifyActionParseAndExecute(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "welcome, admin panel token=XYZ")
	}))
	defer mock.Close()

	workspace := t.TempDir()
	writeScopeAllowAll(t, workspace)
	server := &Server{workspace: workspace}
	appendObservationJournal(t, workspace, "obs-4000-abcd", mock.URL)
	appendTestFact(t, workspace, "fact-4", "admin panel 可达 [obs-4000-abcd]", 2, []string{"obs-4000-abcd"})

	refs, results := server.parseAndExecuteAIActions("ACTION:VERIFY obs-4000-abcd token=XYZ")
	if len(results) != 1 || results[0].Kind != "verify" {
		t.Fatalf("expected one verify result, got %+v", results)
	}
	if !strings.Contains(results[0].Summary, "验证通过") {
		t.Fatalf("expected promotion summary, got %q", results[0].Summary)
	}
	_ = refs
	facts := readJournalFacts(t, workspace)
	if len(facts) != 1 || facts[0].EvidenceLevel != 3 {
		t.Fatalf("expected fact-4 promoted to E3, got %+v", facts)
	}
}
