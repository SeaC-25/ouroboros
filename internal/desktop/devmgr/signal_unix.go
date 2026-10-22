//go:build !windows

package devmgr

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// prepareCommand keeps every child in its own process group so the
// orchestrator can signal the whole tree, mirroring `detached: platform !==
// 'win32'` (electron) and the default group signalling in the mjs script.
func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// signalProcess implements signalChildProcess from dev-desktop.mjs: try the
// process group first (-pid), fall back to the direct child when the group
// is already gone.
func signalProcess(cmd *exec.Cmd, sig os.Signal) error {
	if cmd.Process == nil {
		return os.ErrInvalid
	}
	psig, ok := sig.(syscall.Signal)
	if !ok {
		psig = syscall.SIGTERM
	}
	pid := cmd.Process.Pid
	if pid > 1 {
		if err := syscall.Kill(-pid, psig); err == nil {
			return nil
		}
		// Fall through: process group already gone.
	}
	if err := cmd.Process.Signal(psig); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return fmt.Errorf("signal %v to pid %d: %w", psig, pid, err)
	}
	return nil
}

// sigTerm is the graceful-phase signal.
func sigTerm() os.Signal { return syscall.SIGTERM }

// exeSuffix — no suffix on posix.
const exeSuffix = ""
