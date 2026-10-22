package devmgr

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// RunWithRunner executes the dev-desktop orchestration against a Runner so
// tests can substitute fakes. Steps mirror runDesktopDevelopment in
// gui/scripts/dev-desktop.mjs:
//
//  1. build desktop main (node scripts/build-desktop-main.mjs)
//  2. start the vite dev-server driver and read the OURO_DEV_SERVER_URL
//     marker line from its stdout
//  3. spawn electron with VITE_DEV_SERVER_URL injected
//
// SIGINT/SIGTERM are forwarded to the active child; on shutdown the active
// child is terminated SIGTERM -> grace -> SIGKILL -> forced deadline, then
// the vite driver is terminated the same way.
func RunWithRunner(guiRoot string, opts Options, runner Runner) error {
	grace := time.Duration(gracefulShutdownMilliseconds) * time.Millisecond
	forced := time.Duration(forcedShutdownMilliseconds) * time.Millisecond

	node, err := nodeBinary()
	if err != nil {
		return err
	}
	viteDriverPath, err := materializeDriver(guiRoot, "vite-driver.mjs", viteDriverSource)
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(strings.TrimSuffix(viteDriverPath, string(os.PathSeparator)+"vite-driver.mjs")) }()

	shutdown := make(chan os.Signal, 1)
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		if sig, ok := <-sigCh; ok {
			shutdown <- sig
		}
	}()
	_ = shutdown

	ctx := context.Background()

	// Step 1: build desktop main bundle.
	build := runner.Command(ctx, node, viteBuildMainArgs(guiRoot)...)
	attachInherit(build)
	if err := build.Start(); err != nil {
		return &ChildError{Label: "Desktop main build", Err: err, Code: -1}
	}
	code, err := childResultOrShutdown(build, sigCh)
	if sig := requestedSignal(shutdown); sig != nil {
		_ = build.Kill(grace, forced)
		return &SignalExitError{Signal: signalName(sig)}
	}
	if err := assertSuccessfulChild(code, err, "Desktop main build"); err != nil {
		return err
	}

	// Step 2: start the vite dev-server driver, wait for the URL marker.
	electronPath, err := electronBinary(guiRoot)
	if err != nil {
		return err
	}
	viteArgs := []string{viteDriverPath, guiRoot}
	if opts.LogLevel != "" {
		viteArgs = append(viteArgs, opts.LogLevel)
	}
	vite := runner.Command(ctx, node, viteArgs...)
	attachInheritStderrOnly(vite)
	viteOut, err := vite.Stdout()
	if err != nil {
		return err
	}
	if err := vite.Start(); err != nil {
		return &ChildError{Label: "Vite dev server", Err: err, Code: -1}
	}
	devURL, viteErrCh, err := awaitDevServerURL(rdr{io.ReadCloser(viteOut)}, sigCh, shutdown)
	if err != nil {
		_ = vite.Kill(grace, forced)
		return err
	}
	if sig := requestedSignal(shutdown); sig != nil {
		_ = vite.Kill(grace, forced)
		return &SignalExitError{Signal: signalName(sig)}
	}
	_ = viteErrCh
	fmt.Fprintf(os.Stdout, "desktop renderer ready: %s\n", devURL)

	// Step 3: spawn electron with VITE_DEV_SERVER_URL injected.
	electron := runner.Command(ctx, electronPath, guiMainEntryPath(guiRoot))
	electron.AddEnv("VITE_DEV_SERVER_URL=" + devURL)
	attachInherit(electron)
	if err := electron.Start(); err != nil {
		_ = vite.Kill(grace, forced)
		return &ChildError{Label: "Electron development runtime", Err: err, Code: -1}
	}
	code, err = childResultOrShutdown(electron, sigCh)
	if sig := requestedSignal(shutdown); sig != nil {
		_ = electron.Kill(grace, forced)
		_ = vite.Kill(grace, forced)
		return &SignalExitError{Signal: signalName(sig)}
	}
	_ = vite.Kill(grace, forced)
	return assertSuccessfulChild(code, err, "Electron development runtime")
}

// childResultOrShutdown mirrors childResultOrShutdown in the mjs: race the
// child exit result against the first operator shutdown signal.
func childResultOrShutdown(child Command, sigCh chan os.Signal) (int, error) {
	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		code, err := child.Wait()
		done <- result{code: code, err: err}
	}()
	select {
	case r := <-done:
		return r.code, r.err
	case <-sigCh:
		return -1, nil
	}
}

// assertSuccessfulChild mirrors assertSuccessfulChild in the mjs.
func assertSuccessfulChild(code int, err error, label string) error {
	if err != nil {
		return &ChildError{Label: label, Err: err, Code: 1}
	}
	if code != 0 {
		return &ChildError{Label: label, Err: fmt.Errorf("exited with code %d", code), Code: code}
	}
	return nil
}

// requestedSignal drains the shutdown channel non-blockingly.
func requestedSignal(ch chan os.Signal) os.Signal {
	select {
	case sig := <-ch:
		return sig
	default:
		return nil
	}
}

func signalName(sig os.Signal) string {
	if sig == os.Interrupt {
		return "SIGINT"
	}
	if s, ok := sig.(syscall.Signal); ok && s == syscall.SIGTERM {
		return "SIGTERM"
	}
	return fmt.Sprintf("%v", sig)
}

// awaitDevServerURL scans the vite driver stdout for the marker line. A
// shutdown during the wait aborts with a nil URL error so the caller checks
// requestedSignal afterwards (mirrors the mjs awaiting before spawning
// electron while a shutdown promise may already be resolved).
func awaitDevServerURL(out io.Reader, sigCh chan os.Signal, shutdown chan os.Signal) (string, chan error, error) {
	lineCh := make(chan string, 8)
	errCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(out)
		scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for scanner.Scan() {
			select {
			case lineCh <- scanner.Text():
			case <-sigCh:
				errCh <- nil
				return
			}
		}
		errCh <- scanner.Err()
	}()
	for {
		select {
		case line := <-lineCh:
			if strings.HasPrefix(line, marker+" ") {
				url := strings.TrimSpace(strings.TrimPrefix(line, marker))
				if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
					return url, errCh, nil
				}
				return "", errCh, &ChildError{Label: "Vite dev server", Err: fmt.Errorf("unsupported vite development URL: %s", url), Code: 1}
			}
		case sig := <-sigCh:
			shutdown <- sig
			return "", errCh, nil
		case err := <-errCh:
			return "", nil, &EnvError{What: "read vite dev server output", Err: err}
		}
	}
}

// viteBuildMainArgs mirrors spawn(process.execPath, [scripts/build-desktop-main.mjs]).
func viteBuildMainArgs(guiRoot string) []string {
	return []string{guiRoot + string(os.PathSeparator) + "scripts" + string(os.PathSeparator) + "build-desktop-main.mjs"}
}

// guiMainEntryPath is the Electron entry the mjs passes to the electron
// binary (dist/main/main.js).
func guiMainEntryPath(guiRoot string) string {
	return guiRoot + string(os.PathSeparator) + "dist" + string(os.PathSeparator) + "main" + string(os.PathSeparator) + "main.js"
}

// attachInherit mirrors stdio:'inherit' for children whose output is passed
// through (build and electron).
func attachInherit(c Command) {
	c.SetStdout(os.Stdout)
	c.SetStderr(os.Stderr)
}

// attachInheritStderrOnly leaves stdout for the URL scanner but still
// inherits stderr for diagnostics.
func attachInheritStderrOnly(c Command) {
	c.SetStderr(os.Stderr)
}

// rdr adapts io.ReadCloser for awaitDevServerURL (kept for seam clarity).
type rdr struct {
	io.ReadCloser
}
