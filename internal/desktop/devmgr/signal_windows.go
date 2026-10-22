//go:build windows

package devmgr

import (
	"os"
	"os/exec"
	"syscall"
)

// prepareCommand is a no-op on Windows: mirrors dev-desktop.mjs running the
// electron child non-detached on win32, and the build/vite children plain.
func prepareCommand(cmd *exec.Cmd) {}

// signalWindowsReason documents that Windows has no POSIX kill semantics for
// os.Interrupt; Signal maps everything onto Terminate below.
const signalWindowsReason = "windows: no POSIX signals for console children"

// signalProcess implements the Windows half of signalChildProcess: the
// process-group branch is skipped entirely (process.platform === 'win32'
// in the mjs), so we only ever target the direct child. Go cannot deliver
// POSIX signals to a child console process, so any request becomes
// Process.Kill — which is what the mjs effectively achieves too, because
// Node delivers SIGTERM to a win32 child via TerminateProcess.
func signalProcess(cmd *exec.Cmd, _ os.Signal) error {
	if cmd.Process == nil {
		return os.ErrInvalid
	}
	return cmd.Process.Kill()
}

// sigTerm returns the pseudo SIGTERM used for the graceful phase. Identity
// only matters for fake-command tests; signalProcess ignores it on Windows.
func sigTerm() os.Signal { return syscall.SIGTERM }

// exeSuffix is the Windows executable suffix used by the node/electron
// lookup in paths.go.
const exeSuffix = ".exe"
