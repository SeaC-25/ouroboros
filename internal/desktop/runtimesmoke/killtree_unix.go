//go:build !windows

package runtimesmoke

import (
	"os/exec"
	"syscall"
)

// prepareCommand keeps the child in the caller's process group so terminal
// signals reach it, exactly like Node's spawnSync.
func prepareCommand(cmd *exec.Cmd) {}

// KillTree force-kills cmd's process. Deeper tree cleanup is delegated to the
// smoke shell script's own EXIT/INT/TERM trap, mirroring the original wrapper
// (devmgr Unix pattern: signal then SIGKILL).
func KillTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	_ = cmd.Process.Kill()
}
