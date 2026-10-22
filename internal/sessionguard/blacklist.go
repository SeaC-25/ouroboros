package sessionguard

import (
	"net/url"
	"strings"
	"sync"
)

// DefaultDestructiveAuthMarkers are the logout-semantic path fragments every
// session is protected against out of the box: logging out, signing out,
// revoking tokens, destroying sessions and ending sessions. Matched
// case-insensitively as substrings of the path.
var DefaultDestructiveAuthMarkers = []string{
	"logout",
	"log_out",
	"log-out",
	"signout",
	"sign_out",
	"sign-out",
	"revoke",
	"destroy",
	"session",
	"end",
}

// Blacklist is the logout-semantic hard-stop. It decides, before a request is
// ever sent, whether a URL carries destructive-auth semantics; a hit means
// the request must not be spent from a session's cookies.
//
// It is safe for concurrent use. Extending it is copy-on-write: SetCustom
// swaps an immutable marker list, so a decision made concurrently with
// SetCustom sees either the old or the new list, never a torn one.
type Blacklist struct {
	mu     sync.RWMutex
	custom []string
}

// NewBlacklist returns a Blacklist with only the default markers.
func NewBlacklist() *Blacklist {
	return &Blacklist{}
}

// SetCustom replaces the configured extension markers. Passing an empty/nil
// slice clears the extension. Markers are lower-cased on the way in.
func (b *Blacklist) SetCustom(markers []string) {
	lowered := make([]string, 0, len(markers))
	for _, m := range markers {
		if m = strings.ToLower(strings.TrimSpace(m)); m != "" {
			lowered = append(lowered, m)
		}
	}
	b.mu.Lock()
	b.custom = lowered
	b.mu.Unlock()
}

// Custom returns a copy of the configured extension markers.
func (b *Blacklist) Custom() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, len(b.custom))
	copy(out, b.custom)
	return out
}

// IsDestructiveAuthPath reports whether url carries logout / signout / revoke
// / destroy / session-end semantics on its path. Matching is
// case-insensitive, substring-based ("path contains ⇒ intercepted") and
// consults both the default markers and the configured extension. Only the
// URL path is examined — query strings never influence the verdict.
//
// Unparseable input falls back to scanning the raw string, erring toward
// interception rather than letting a destructive call slip through.
func IsDestructiveAuthPath(u *url.URL) bool {
	return defaultBlacklist().MatchURL(u)
}

// MatchURL is the method form of IsDestructiveAuthPath for a configured
// blacklist instance.
func (b *Blacklist) MatchURL(u *url.URL) bool {
	if u == nil {
		return false
	}
	// Path-only decision: strip the query and fragment from the equation.
	candidate := u.Path
	if candidate == "" {
		candidate = u.Opaque
	}
	if candidate == "" && u.Host == "" {
		// Not a parseable absolute URL shape; scan whatever we have.
		candidate = u.String()
	}
	return b.match(candidate)
}

// MatchString parses raw as a URL and applies the same decision as MatchURL.
// A parse failure degrades to a raw substring scan over the whole input.
func (b *Blacklist) MatchString(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if u, err := url.Parse(raw); err == nil {
		return b.MatchURL(u)
	}
	return b.match(raw)
}

// match applies the marker decision to a candidate string.
func (b *Blacklist) match(candidate string) bool {
	if candidate == "" {
		return false
	}
	candidate = strings.ToLower(candidate)
	b.mu.RLock()
	custom := b.custom
	b.mu.RUnlock()
	for _, m := range DefaultDestructiveAuthMarkers {
		if strings.Contains(candidate, m) {
			return true
		}
	}
	for _, m := range custom {
		if strings.Contains(candidate, m) {
			return true
		}
	}
	return false
}

// defaultBlacklist is a shared, read-mostly instance backing the package-level
// function. It never receives custom markers, so RLock-unlock is cheap.
func defaultBlacklist() *Blacklist {
	return &Blacklist{}
}
