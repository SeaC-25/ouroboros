package desktopcontract

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Dynamic contract drivers, materialised to a temp dir and executed with
// node so they can import the exact same JS modules as the original
// check-desktop-contract.mjs and run the exact same assertions.
//
//go:embed drivers/memory.mjs drivers/backend_exit.mjs drivers/runtime_pid.mjs drivers/installer.mjs
var driverFS embed.FS

const (
	dynamicMemoryContractDriver      = "drivers/memory.mjs"
	dynamicBackendExitContractDriver = "drivers/backend_exit.mjs"
	dynamicRuntimePIDContractDriver  = "drivers/runtime_pid.mjs"
	dynamicInstallerContractDriver   = "drivers/installer.mjs"
)

// runNodeContract materialises the named embedded driver into a fresh temp
// directory and runs it as `node <driver> <args...>`. A non-zero node exit
// is a contract failure (the driver already printed the original mjs
// messages to stderr); a spawn failure (node missing, unwritable temp dir)
// is an environment error.
func (r *runner) runNodeContract(driver string, args ...string) {
	source, err := driverFS.ReadFile(driver)
	if err != nil {
		fmt.Fprintf(r.opts.Stderr, "desktop contract driver missing: %s: %v\n", driver, err)
		r.envErr = true
		return
	}
	dir, err := os.MkdirTemp("", "ouro-desktop-contract-")
	if err != nil {
		fmt.Fprintf(r.opts.Stderr, "desktop contract driver temp dir: %v\n", err)
		r.envErr = true
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()

	path := filepath.Join(dir, filepath.Base(driver))
	if err := os.WriteFile(path, source, 0o644); err != nil {
		fmt.Fprintf(r.opts.Stderr, "desktop contract driver %s: %v\n", path, err)
		r.envErr = true
		return
	}

	nodeArgs := append([]string{path}, args...)
	cmd := exec.Command(r.opts.Node, nodeArgs...)
	cmd.Stdout = r.opts.Stderr
	cmd.Stderr = r.opts.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			fmt.Fprintf(r.opts.Stderr, "dynamic desktop contract failed (%s): node exited with code %d\n", driver, exitErr.ExitCode())
			r.failed = true
			return
		}
		fmt.Fprintf(r.opts.Stderr, "dynamic desktop contract could not run (%s): %v\n", driver, err)
		r.envErr = true
	}
}
