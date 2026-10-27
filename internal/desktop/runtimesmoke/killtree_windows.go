//go:build windows

package runtimesmoke

import (
	"os/exec"
)

// prepareCommand is a no-op on Windows; tree termination goes through
// taskkill (devmgr Windows pattern).
func prepareCommand(cmd *exec.Cmd) {}
