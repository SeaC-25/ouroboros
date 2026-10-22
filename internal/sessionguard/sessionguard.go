// Package sessionguard implements the GateMesh S2 session plane: the
// cross-cutting SessionLease (COW snapshots + versioned publish), the logout
// blacklist hard-stop, and the canary sentinel that freezes a session the
// moment its authentication visibly dies.
//
// The plane sits between any caller holding cookies and the HTTP client used
// to spend them. In-flight requests keep referencing the snapshot version they
// started with; publishing a new version never mutates the old one.
package sessionguard

import "errors"

// Sentinel errors returned by SessionStore and Canary. Errors.Is them.
var (
	// ErrUnknownSession is returned when a session name is not published.
	ErrUnknownSession = errors.New("sessionguard: unknown session")
	// ErrFrozen is returned when a lease snapshot is taken from a frozen
	// session; spending frozen credentials is refused at the source.
	ErrFrozen = errors.New("sessionguard: session frozen")
)

// AutoFreezeReason values explain why a Canary froze a session.
const (
	// ReasonStatusDrift: the probe endpoint stopped answering with the
	// expected status code.
	ReasonStatusDrift = "status_drift"
	// ReasonBodyDrift: the probe endpoint still answered but its body
	// signature no longer matches the learned one.
	ReasonBodyDrift = "body_drift"
)
