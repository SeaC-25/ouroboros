package devmgr

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Runner abstracts child-process control so tests can substitute fakes
// without launching real node/electron binaries. Obtain the production
// implementation from ExecRunner.
type Runner interface {
	// Command builds a process handle; Start has not been called yet.
	Command(ctx context.Context, name string, args ...string) Command
}

// Command is the child-process surface the orchestrator needs, kept minimal
// so tests can exercise Kill/Signal/Wait with pure-Go fakes.
type Command interface {
	// Start launches the process.
	Start() error
	// Wait blocks until the process exits, returning the exit code, or -1
	// when the process was terminated by a signal or the code is unknown.
	Wait() (int, error)
	// Stdout returns a reader streaming the child's standard output.
	Stdout() (io.ReadCloser, error)
	// AddEnv appends KEY=VALUE pairs to the child environment (mirrors the
	// env spread in the mjs electron spawn, e.g. VITE_DEV_SERVER_URL).
	AddEnv(kv ...string)
	// SetStdout/SetStderr attach sinks before Start (mirrors exec.Cmd).
	SetStdout(w io.Writer)
	SetStderr(w io.Writer)
	// Signal delivers sig (os.Interrupt or syscall.SIGTERM) applying the
	// platform policy: on Windows the direct child is targeted; on posix
	// the child's process group is signalled when the child was started
	// detached, falling back to the direct child (mirrors signalChildProcess
	// in dev-desktop.mjs).
	Signal(sig os.Signal) error
	// Kill escalates SIGTERM -> grace -> SIGKILL -> forced deadline,
	// mirroring terminateChildProcess in dev-desktop.mjs. No-op when the
	// child already exited.
	Kill(grace, forced time.Duration) error
	// Exited reports whether the process has finished.
	Exited() bool
	// PID reports the OS process id, or 0 before Start.
	PID() int
}

// ExecRunner is the production Runner backed by os/exec.
type ExecRunner struct {
	// Dir is the working directory for every child (the gui root).
	Dir string
	// EnvExtra is appended to os.Environ for every child.
	EnvExtra []string
	// Stdout/Stderr are default sinks used when the orchestrator does not
	// take over a stream (package-level, mirroring stdio:'inherit').
	Stdout io.Writer
	Stderr io.Writer
	// NewCommand lets tests of ExecRunner itself observe commands; never
	// set by production code.
	NewCommand func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// NewExecRunner returns a Runner rooted at dir.
func NewExecRunner(dir string) *ExecRunner { return &ExecRunner{Dir: dir} }

// Command implements Runner.
func (r *ExecRunner) Command(ctx context.Context, name string, args ...string) Command {
	newCmd := r.NewCommand
	if newCmd == nil {
		newCmd = exec.CommandContext
	}
	c := newCmd(ctx, name, args...)
	if r.Dir != "" {
		c.Dir = r.Dir
	}
	c.Env = append(os.Environ(), r.EnvExtra...)
	if r.Stdout != nil {
		c.Stdout = r.Stdout
	}
	if r.Stderr != nil {
		c.Stderr = r.Stderr
	}
	prepareCommand(c)
	return &execCommand{cmd: c, done: make(chan struct{})}
}

// execCommand adapts *exec.Cmd to Command.
type execCommand struct {
	cmd    *exec.Cmd
	mu     sync.Mutex
	done   chan struct{}
	code   int
	killed bool
}

func (e *execCommand) Start() error { return e.cmd.Start() }

func (e *execCommand) Wait() (int, error) {
	err := e.cmd.Wait()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.code = -1
	if e.cmd.ProcessState != nil {
		e.code = e.cmd.ProcessState.ExitCode()
	}
	select {
	case <-e.done:
	default:
		close(e.done)
	}
	if err == nil {
		return e.code, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		// Non-zero exit carries the code; the caller wants the code, not
		// the wrapper, matching assertSuccessfulChild in the mjs script.
		return exitErr.ExitCode(), nil
	}
	return e.code, err
}

func (e *execCommand) Stdout() (io.ReadCloser, error) { return e.cmd.StdoutPipe() }

func (e *execCommand) AddEnv(kv ...string) { e.cmd.Env = append(e.cmd.Env, kv...) }

func (e *execCommand) SetStdout(w io.Writer) { e.cmd.Stdout = w }
func (e *execCommand) SetStderr(w io.Writer) { e.cmd.Stderr = w }

func (e *execCommand) Exited() bool {
	select {
	case <-e.done:
		return true
	default:
		return false
	}
}

func (e *execCommand) PID() int {
	if e.cmd.Process == nil {
		return 0
	}
	return e.cmd.Process.Pid
}

// Signal implements the platform policy; see signal_windows.go /
// signal_unix.go.
func (e *execCommand) Signal(sig os.Signal) error {
	if e.Exited() {
		return nil
	}
	return signalProcess(e.cmd, sig)
}

// Kill mirrors terminateChildProcess in dev-desktop.mjs: skip already-dead
// children, SIGTERM, wait grace, then SIGKILL plus the forced deadline.
func (e *execCommand) Kill(grace, forced time.Duration) error {
	if e.Exited() {
		return nil
	}
	// SIGTERM phase (with group semantics on posix).
	_ = e.Signal(sigTerm())
	select {
	case <-e.done:
		return nil
	case <-time.After(grace):
	}
	// SIGKILL escalation.
	e.mu.Lock()
	e.killed = true
	e.mu.Unlock()
	if e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
	}
	select {
	case <-e.done:
	case <-time.After(forced):
	}
	return nil
}
