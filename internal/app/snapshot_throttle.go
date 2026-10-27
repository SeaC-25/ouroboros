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
// Snapshots are for display; a 250ms coalescing window is imperceptible and
// the stage-end rebuild always flushes the final state.
type snapshotThrottle struct {
	mu       sync.Mutex
	lastEmit time.Time
}

// allow reports whether a rebuild should run now. Events within the window
// are skipped (the next allowed rebuild carries them).
func (t *snapshotThrottle) allow(now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if now.Sub(t.lastEmit) >= 250*time.Millisecond {
		t.lastEmit = now
		return true
	}
	return false
}

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
	throttle := &snapshotThrottle{}
	options.snapshotThrottle = throttle
	options.OnSnapshot = func(snapshot eventlog.ReplaySnapshot) error {
		if !throttle.allow(time.Now()) {
			return nil
		}
		return inner(snapshot)
	}
	return options
}
