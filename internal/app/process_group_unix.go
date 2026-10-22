//go:build unix

package app

import (
	"os/exec"
	"syscall"
)

// applyProcessGroup starts the command in its own process group so a timeout
// can kill the entire tree (see process_group.go).
func applyProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// processGroupID returns the PGID of the command (its own PID, since we set
// Setpgid at launch), or 0 when unavailable.
func processGroupID(cmd *exec.Cmd) int {
	if cmd == nil || cmd.Process == nil {
		return 0
	}
	// With Setpgid=true the child becomes its own group leader: PGID == PID.
	return cmd.Process.Pid
}

// killGroup sends SIGKILL to the whole (negative) process group.
func killGroup(pgid int) {
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}
