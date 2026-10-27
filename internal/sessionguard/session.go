package sessionguard

import (
	"sync"
	"time"
)

// Lease is an immutable, versioned snapshot of one authenticated session's
// cookies. Callers take a Lease and read it for the whole duration of a
// request; publishing a new version never touches a previously returned
// Lease (copy-on-write), so in-flight requests stay consistent even while the
// store replaces the session state underneath them.
//
// A Lease must be treated as read-only: Cookies is an internal map exposed for
// iteration via ForEach only. Construct Leases through NewLease / SessionStore,
// never by literal.
type Lease struct {
	cookies map[string]string

	version   int64
	createdAt time.Time

	// frozen marks a snapshot taken from a session that has since been
	// frozen; spending it is refused. Frozen-ness is decided at snapshot
	// time, not retroactively on old references.
	frozen bool
}

// NewLease builds an independent snapshot from the given cookies. The input
// map is copied; later mutation of the caller's map cannot affect the lease.
// A nil/empty map yields an empty (but valid) lease.
func NewLease(cookies map[string]string) *Lease {
	cp := make(map[string]string, len(cookies))
	for k, v := range cookies {
		cp[k] = v
	}
	return &Lease{cookies: cp, createdAt: time.Now().UTC()}
}

// Version returns the monotonically increasing version of this snapshot.
// Version 0 is the initial unpublished state; the store's first Publish
// produces version 1.
func (l *Lease) Version() int64 { return l.version }

// Frozen reports whether this snapshot came from a frozen session.
func (l *Lease) Frozen() bool { return l.frozen }

// Cookie returns the value for name and whether it exists.
func (l *Lease) Cookie(name string) (string, bool) {
	v, ok := l.cookies[name]
	return v, ok
}

// Len returns the number of cookies in the snapshot.
func (l *Lease) Len() int { return len(l.cookies) }

// ForEach calls fn for every cookie in the snapshot. This is the only
// supported way to read the full set; fn must not retain the key/value
// strings beyond the call.
func (l *Lease) ForEach(fn func(name, value string)) {
	for k, v := range l.cookies {
		fn(k, v)
	}
}

// clone produces a deep copy of the lease for the next published version.
func (l *Lease) clone(version int64, frozen bool) *Lease {
	cp := make(map[string]string, len(l.cookies))
	for k, v := range l.cookies {
		cp[k] = v
	}
	return &Lease{cookies: cp, version: version, createdAt: time.Now().UTC(), frozen: frozen}
}

// SessionStore manages multiple named sessions, each holding the current
// Lease version plus its freeze state. All methods are safe for concurrent
// use. Publish never mutates a previously returned Lease.
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]*sessionState
	next     int64 // next version to hand out, shared across all sessions
}

type sessionState struct {
	current *Lease // the published, authoritative version
	frozen  bool
	reason  string // freeze reason, "" when live
}

// NewSessionStore returns an empty store.
func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]*sessionState), next: 1}
}

// Publish stores cookies as the new version of the named session and returns
// the fresh snapshot. Version numbers increase monotonically per call across
// the whole store (1, 2, 3, …), so every published snapshot is uniquely
// identifiable. Re-publishing over a frozen session is refused with ErrFrozen;
// unfreeze (or Delete + Publish) first.
func (s *SessionStore) Publish(name string, cookies map[string]string) (*Lease, error) {
	if name == "" {
		return nil, ErrUnknownSession
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.sessions[name]; ok && st.frozen {
		return nil, ErrFrozen
	}
	lease := NewLease(cookies)
	lease.version = s.next
	s.next++
	lease.frozen = false
	s.sessions[name] = &sessionState{current: lease}
	return lease, nil
}

// Lease returns the current published snapshot for the named session. A
// frozen session returns a frozen-marked lease (not an error): callers may
// still inspect it, but MustLease-style spenders see Frozen() and refuse.
// Unknown sessions return ErrUnknownSession.
func (s *SessionStore) Lease(name string) (*Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.sessions[name]
	if !ok {
		return nil, ErrUnknownSession
	}
	if st.frozen {
		// Hand out a frozen clone so callers can never reach the
		// authoritative version, and so freezing does not retroactively
		// mutate previously returned snapshots (COW discipline).
		return st.current.clone(st.current.version, true), nil
	}
	return st.current, nil
}

// Freeze marks the named session frozen with a reason. Subsequent Lease()
// calls return frozen-marked snapshots and Publish is refused until Unfreeze.
// Unknown sessions return ErrUnknownSession. Idempotent: re-freezing updates
// the reason and still returns nil.
func (s *SessionStore) Freeze(name, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.sessions[name]
	if !ok {
		return ErrUnknownSession
	}
	st.frozen = true
	st.reason = reason
	return nil
}

// Unfreeze lifts the freeze so Publish may resume. The reason is cleared.
func (s *SessionStore) Unfreeze(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.sessions[name]
	if !ok {
		return ErrUnknownSession
	}
	st.frozen = false
	st.reason = ""
	return nil
}

// Frozen reports the session's freeze state and reason.
func (s *SessionStore) Frozen(name string) (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.sessions[name]
	if !ok {
		return false, ""
	}
	return st.frozen, st.reason
}

// Delete removes the session entirely; the previously published Lease objects
// held by callers remain valid (COW). Unknown sessions return ErrUnknownSession.
func (s *SessionStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[name]; !ok {
		return ErrUnknownSession
	}
	delete(s.sessions, name)
	return nil
}

// Versions returns every version number ever handed out by this store —
// useful for assertions that publishing never rewinds.
func (s *SessionStore) Versions() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int64, 0, s.next-1)
	for v := int64(1); v < s.next; v++ {
		out = append(out, v)
	}
	return out
}
