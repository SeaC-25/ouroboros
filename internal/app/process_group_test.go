package app

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// S4 process-group guard: guardedCommand must return a usable cmd and
// killProcessGroup must be a safe no-op on every platform.
func TestGuardedCommandConstructs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	bin := "cmd"
	args := []string{"/C", "echo hi"}
	if runtime.GOOS != "windows" {
		bin = "echo"
		args = []string{"hi"}
	}
	cmd := guardedCommand(ctx, bin, args...)
	if cmd == nil {
		t.Fatal("guardedCommand returned nil")
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run guarded command: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected output")
	}
}

func TestKillProcessGroupNilSafety(t *testing.T) {
	// Must not panic on nil or process-less commands.
	killProcessGroup(nil)
	cmd := exec.Command("definitely-not-a-real-binary-xyz")
	killProcessGroup(cmd) // Process is nil until Start
}

func TestGuardedCommandTimeoutKills(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	bin := "ping"
	args := []string{"-n", "10", "127.0.0.1"}
	if runtime.GOOS != "windows" {
		bin = "sleep"
		args = []string{"10"}
	}
	cmd := guardedCommand(ctx, bin, args...)
	start := time.Now()
	_, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("command should have died with ctx (~300ms), took %v", elapsed)
	}
	killProcessGroup(cmd) // group kill must be safe post-exit
}
