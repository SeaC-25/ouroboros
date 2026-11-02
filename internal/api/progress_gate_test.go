package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestProgressGatePromptContainsForcedPivotDirectives checks the injected
// pivot instruction carries the three mandatory re-orientation directives.
func TestProgressGatePromptContainsForcedPivotDirectives(t *testing.T) {
	prompt := progressGatePrompt(6)
	for _, want := range []string{"【Progress 门】", "连续 6 轮", "已排除手法", "正交象限", "前置信息"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("progressGatePrompt(6) missing %q, got %q", want, prompt)
		}
	}
}

// TestDoneRejectionPromptText checks the DONE rejection instruction text.
func TestDoneRejectionPromptText(t *testing.T) {
	prompt := doneRejectionPrompt()
	for _, want := range []string{"终止被驳回", "缺什么前置条件", "2 个侦察动作", "再输出 DONE"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("doneRejectionPrompt() missing %q, got %q", want, prompt)
		}
	}
}

// TestHasNewFlagFactSince exercises the DONE-rejection precondition on a
// minimal Server over a temp workspace blackboard journal: only a
// flag-shaped fact at or after the loop start counts as terminal progress.
func TestHasNewFlagFactSince(t *testing.T) {
	workspace := t.TempDir()
	server := &Server{workspace: workspace}
	if server.hasNewFlagFactSince(1000) {
		t.Fatalf("empty blackboard must report no flag fact")
	}
	runtimeDir := filepath.Join(workspace, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	// Old non-flag fact: never terminal, whatever the window.
	if err := os.WriteFile(filepath.Join(runtimeDir, "blackboard.jsonl"), []byte(
		`{"id":"f1","statement":"旧结论","createdAt":500}`+"\n"), 0o644); err != nil {
		t.Fatalf("write blackboard: %v", err)
	}
	if server.hasNewFlagFactSince(400) {
		t.Fatalf("non-flag fact must never count as terminal progress")
	}
	// Old flag fact: predates the loop start -> not new.
	if err := os.WriteFile(filepath.Join(runtimeDir, "blackboard.jsonl"), []byte(
		`{"id":"f2","statement":"flag{old}","createdAt":500}`+"\n"), 0o644); err != nil {
		t.Fatalf("rewrite blackboard: %v", err)
	}
	if server.hasNewFlagFactSince(1000) {
		t.Fatalf("flag fact before loop start must not count")
	}
	// New flag fact at/after loop start: terminal progress.
	if !server.hasNewFlagFactSince(500) {
		t.Fatalf("flag fact at loop start must count as terminal progress")
	}
}

// TestSubLoopGateStatePersistsThroughJournal covers the journal round-trip
// of PivotCount and DoneRejected: persist -> load -> same values.
func TestSubLoopGateStatePersistsThroughJournal(t *testing.T) {
	workspace := t.TempDir()
	server := &Server{workspace: workspace}

	state := &subLoopState{
		Name:         "gate-loop",
		Goal:         "验证持久化",
		Status:       "done",
		Turns:        12,
		PivotCount:   2,
		DoneRejected: true,
		StartedAt:    time.Unix(1700000000, 0),
		EndedAt:      time.Unix(1700000600, 0),
	}
	server.persistSubLoop(state)

	// Live state wins over the journal, so drop it before loading.
	globalSubLoops.mu.Lock()
	delete(globalSubLoops.loops, "gate-loop")
	globalSubLoops.mu.Unlock()
	server.loadPersistedSubLoops()

	globalSubLoops.mu.Lock()
	loaded := globalSubLoops.loops["gate-loop"]
	globalSubLoops.mu.Unlock()
	if loaded == nil {
		t.Fatalf("expected gate-loop loaded from journal")
	}
	if loaded.PivotCount != 2 || !loaded.DoneRejected {
		t.Fatalf("gate state lost: PivotCount=%d DoneRejected=%v", loaded.PivotCount, loaded.DoneRejected)
	}
}

// TestSubLoopStatePivotCounterSemantics exercises the gate condition
// expressions directly: pivot cap at 2 and one-time DONE rejection.
func TestSubLoopStatePivotCounterSemantics(t *testing.T) {
	// Pivot cap: injection fires only while PivotCount < 2.
	shouldInject := func(state *subLoopState, stall int) bool { return stall >= 5 && state.PivotCount < 2 }
	state := &subLoopState{}
	for range 2 {
		if !shouldInject(state, 5) {
			t.Fatalf("expected injection at stall=5 pivot %d", state.PivotCount)
		}
		state.PivotCount++
	}
	if shouldInject(state, 5) || shouldInject(state, 99) {
		t.Fatalf("injection must stop once PivotCount reaches 2")
	}

	// DONE rejection: fires exactly once while turns remain and no new
	// flag fact landed; after DoneRejected flips it never blocks again.
	hasFlag := false
	turn := 7
	shouldReject := func(state *subLoopState) bool {
		return !state.DoneRejected && turn < subLoopTurnLimit && !hasFlag
	}
	rejected := &subLoopState{}
	if !shouldReject(rejected) {
		t.Fatalf("first premature DONE must be rejected")
	}
	rejected.DoneRejected = true
	if shouldReject(rejected) {
		t.Fatalf("DONE must pass once DoneRejected is set")
	}
	// New flag fact on the board -> never rejected.
	hasFlag = true
	fresh := &subLoopState{}
	if shouldReject(fresh) {
		t.Fatalf("DONE with flag on blackboard must not be rejected")
	}
	// Turns exhausted -> never rejected.
	turn = subLoopTurnLimit
	another := &subLoopState{}
	if shouldReject(another) {
		t.Fatalf("DONE at turn limit must not be rejected")
	}
}
