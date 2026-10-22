//go:build windows

package runtimesmoke

import (
	"os/exec"
	"strconv"
)

// prepareCommand is a no-op on Windows; tree termination goes through
// taskkill (devmgr Windows pattern).
func prepareCommand(cmd *exec.Cmd) {}

// KillTree terminates the whole tree rooted at cmd's process via taskkill,
// matching the /T /F semantics used elsewhere in the repo.
func KillTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = exec.Command("taskkill.exe", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F").Run()
}
