package app

import (
	"sync"
	"time"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

// snapshotThrottle coalesces expensive snapshot rebuilds during a run.
// The TUI registers OnSnapshot; without coalescing every appended event
// triggers a full BuildReplaySnapshot (sort + JSON-decode of every event
// payload), which is O(n²) over the run — seconds of pure CPU on long runs.
// Snapshots are for display; a 250ms coalescing window is imperceptible.
//
// Skipped snapshots are not lost: the newest skipped snapshot stays pending
// and a window-boundary timer flushes it, so the terminal state is always
// delivered even when the run ends inside the coalescing window.
type snapshotThrottle struct {
	mu       sync.Mutex
	window   time.Duration
	lastEmit time.Time
	pending  eventlog.ReplaySnapshot
	timer    *time.Timer
	emit     func(eventlog.ReplaySnapshot) error
}

const snapshotThrottleWindow = 250 * time.Millisecond

// WrapSnapshotThrottle installs the coalescing window on a copy of options.
// Callers that already carry a throttle (re-entrant run paths) keep theirs.
// Display consumers opt in; RunWorkspaceWithOptions itself never wraps —
// OnSnapshot is a per-event contract and some callers use it as a
// control-flow signal.
func WrapSnapshotThrottle(options RunOptions) RunOptions {
	if options.OnSnapshot == nil || options.snapshotThrottle != nil {
		return options
	}
	inner := options.OnSnapshot
	throttle := &snapshotThrottle{window: snapshotThrottleWindow, emit: inner}
	options.snapshotThrottle = throttle
	options.OnSnapshot = func(snapshot eventlog.ReplaySnapshot) error {
		return throttle.offer(snapshot)
	}
	return options
}

// FlushSnapshotThrottle delivers any snapshot still pending inside the
// coalescing window. Run owners call it after the run completes so the
// terminal state reaches display consumers even when the run ends inside
// the window.
func FlushSnapshotThrottle(options RunOptions) {
	if options.snapshotThrottle == nil {
		return
	}
	_ = options.snapshotThrottle.flush()
}

// offer either emits immediately (window elapsed) or defers the newest
// snapshot to a single window-boundary flush.
func (t *snapshotThrottle) offer(snapshot eventlog.ReplaySnapshot) error {
	t.mu.Lock()
	now := time.Now()
	if now.Sub(t.lastEmit) >= t.window {
		t.lastEmit = now
		t.pending = eventlog.ReplaySnapshot{}
		if t.timer != nil {
			t.timer.Stop()
			t.timer = nil
		}
		emit := t.emit
		t.mu.Unlock()
		return emit(snapshot)
	}
	t.pending = snapshot
	if t.timer == nil {
		delay := t.window - now.Sub(t.lastEmit)
		t.timer = time.AfterFunc(delay, func() { _ = t.flush() })
	}
	t.mu.Unlock()
	return nil
}

// flush delivers the newest deferred snapshot once the window elapses.
func (t *snapshotThrottle) flush() error {
	t.mu.Lock()
	pending := t.pending
	if pending.RunID == "" && pending.Status.Model == "" {
		t.timer = nil
		t.mu.Unlock()
		return nil
	}
	t.pending = eventlog.ReplaySnapshot{}
	t.lastEmit = time.Now()
	t.timer = nil
	emit := t.emit
	t.mu.Unlock()
	return emit(pending)
}
