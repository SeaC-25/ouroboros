package app

import (
	"context"
	"os/exec"
)

// S4 resilience spine: process-group kill for external tool binaries
// (GateMesh architecture, doc/gatemesh-architecture-v1.md §S4).
//
// Problem: when a managed binary (fastsec, nmap …) times out, Go's
// exec.CommandContext kills only the direct child. Orphaned grandchildren
// keep burning network bandwidth and file handles.
//
// Fix: on Unix, launch the tool in its own process group (Setpgid) and kill
// the whole negative group on timeout. Windows has no POSIX process groups;
// the default child-kill applies there (documented limitation).
//
// Platform-specific pieces live in process_group_unix.go / _windows.go.

// guardedCommand wraps exec.CommandContext with process-group semantics.
func guardedCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	applyProcessGroup(cmd)
	return cmd
}

// killProcessGroup terminates the command's whole process group (Unix).
// Safe no-op on Windows. Call after ctx expiry if the process lingers.
func killProcessGroup(cmd *exec.Cmd) {
	if pgid := processGroupID(cmd); pgid > 0 {
		killGroup(pgid)
	}
}
