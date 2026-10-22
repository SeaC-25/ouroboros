//go:build windows

package app

import "os/exec"

// applyProcessGroup: Windows has no POSIX process groups; the default
// exec.CommandContext child-kill applies. Documented limitation.
func applyProcessGroup(cmd *exec.Cmd) {}

// processGroupID is always 0 on Windows (no group kill available).
func processGroupID(cmd *exec.Cmd) int { return 0 }

// killGroup is a no-op on Windows.
func killGroup(pgid int) {}
