package app

import (
	"runtime"
	"testing"
)

// skipIfWindows marks tests that rely on POSIX shell probe execution or other
// Unix-only behavior as Linux-only. They pass on Linux/CI and are skipped on
// Windows rather than falsifying their assertions.
func skipIfWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test relies on POSIX shell probe execution and is Linux-only")
	}
}
