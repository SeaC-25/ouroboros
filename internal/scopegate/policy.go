// Package scopegate implements the GateMesh S3 compliance gate: the Scope
// door that physically blocks out-of-bounds traffic and labels every action
// by its side-effect class.
//
// Scope enforcement: ScopePolicy declares the target envelope (allowed
// CIDRs, allowed domains with subdomain suffix matching, forbidden path
// patterns). NewGuardedDialer wraps any *net.Dialer into the
// http.Transport DialContext hook and verifies, after DNS resolution but
// before the TCP handshake, that every resolved IP is inside an allowed
// CIDR and that the request host is on the domain allowlist. Blocked
// connections never touch the wire; they fail with ErrOutOfScope wrapped in
// a *ScopeError carrying the offending IP, host and rule.
//
// Mutability classification: every tool action is labelled L0 Passive,
// L1 ReadOnly or L2 StateMutating. L1 spends token-bucket rate-limit tokens
// (default 10 req/s); L2 consumes a per-endpoint attempt budget (default 15)
// and is filtered against high-impact path semantics
// (delete/drop/reset/password/config/shutdown). Denials return a
// *MutabilityError naming tool, endpoint and rule.
//
// The gate is a library: it never wires itself into the agent mainline.
package scopegate

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
)

// Package-wide sentinel errors. Errors.Is them; richer detail lives in the
// *ScopeError / *MutabilityError types returned alongside.
var (
	// ErrOutOfScope is returned by the guarded dialer when a connection
	// target is outside the declared scope envelope.
	ErrOutOfScope = errors.New("scopegate: target outside declared scope")
	// ErrRateLimited is returned when an L1 action exceeds the token-bucket
	// rate.
	ErrRateLimited = errors.New("scopegate: L1 action rate limited")
	// ErrAttemptsExhausted is returned when an L2 action exceeds its
	// per-endpoint attempt budget.
	ErrAttemptsExhausted = errors.New("scopegate: L2 attempt budget exhausted")
	// ErrForbiddenPath is returned when a path matches a high-impact
	// semantic pattern refused for L2 actions.
	ErrForbiddenPath = errors.New("scopegate: path matches forbidden semantic")
	// ErrNotAdmitted is returned when a tool classifies below the level a
	// caller requested.
	ErrNotAdmitted = errors.New("scopegate: tool level below requested")
)

// ScopePolicy is the declared target envelope. A policy is immutable after
// load; GuardedDialer reads it without locking.
type ScopePolicy struct {
	// CIDRs lists allowed network ranges. An empty list disables IP
	// enforcement (every IP passes) so policies may declare domains only.
	CIDRs []*net.IPNet
	// Domains lists allowed domains. A bare entry ("example.com") matches
	// the exact host only; a dot-prefixed entry (".example.com") matches
	// the apex and any subdomain. An empty list disables domain
	// enforcement.
	Domains []string
	// ForbiddenPaths are regexes matched against the request path; a match
	// refuses the action. Empty disables path filtering.
	ForbiddenPaths []*regexp.Regexp
}

type scopePolicyJSON struct {
	CIDRs          []string `json:"cidrs"`
	Domains        []string `json:"domains"`
	ForbiddenPaths []string `json:"forbidden_paths"`
}

// LoadScopePolicy parses a policy document. cidrs entries are CIDR blocks or
// bare IPs; domains are bare hostnames (exact) or dot-prefixed
// (apex-plus-subdomains) forms; forbidden_paths are Go regexes. Empty
// fields disable the corresponding check, making the policy permissive on
// that axis — a policy that whitelists nothing at all loads but guards
// nothing, which callers should treat as a configuration bug.
func LoadScopePolicy(data []byte) (*ScopePolicy, error) {
	var doc scopePolicyJSON
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("scopegate: parse policy JSON: %w", err)
	}
	return buildScopePolicy(doc)
}

// LoadScopePolicyFile reads and parses a policy document from path.
func LoadScopePolicyFile(path string) (*ScopePolicy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("scopegate: read policy file: %w", err)
	}
	return LoadScopePolicy(data)
}

func buildScopePolicy(doc scopePolicyJSON) (*ScopePolicy, error) {
	p := &ScopePolicy{}
	seenCIDR := map[string]bool{}
	for _, raw := range doc.CIDRs {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if _, ipnet, err := net.ParseCIDR(entry); err == nil {
			key := ipnet.String()
			if seenCIDR[key] {
				continue
			}
			seenCIDR[key] = true
			p.CIDRs = append(p.CIDRs, ipnet)
			continue
		}
		ip := net.ParseIP(entry)
		if ip == nil {
			return nil, fmt.Errorf("scopegate: cidrs[%q]: not a valid CIDR block or IP", entry)
		}
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		ipnet := &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}
		key := ipnet.String()
		if seenCIDR[key] {
			continue
		}
		seenCIDR[key] = true
		p.CIDRs = append(p.CIDRs, ipnet)
	}
	seenDomain := map[string]bool{}
	for _, raw := range doc.Domains {
		d := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
		if d == "" {
			continue
		}
		if strings.ContainsAny(d, " \t") {
			return nil, fmt.Errorf("scopegate: domains[%q]: whitespace inside hostname", d)
		}
		if !strings.Contains(d, ".") && !strings.HasPrefix(d, ".") {
			return nil, fmt.Errorf("scopegate: domains[%q]: not a fully-qualified domain name", d)
		}
		if seenDomain[d] {
			continue
		}
		seenDomain[d] = true
		p.Domains = append(p.Domains, d)
	}
	for _, raw := range doc.ForbiddenPaths {
		pattern := strings.TrimSpace(raw)
		if pattern == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("scopegate: forbidden_paths[%q]: %w", pattern, err)
		}
		p.ForbiddenPaths = append(p.ForbiddenPaths, re)
	}
	return p, nil
}

// IPAllowed reports whether ip falls inside any allowed CIDR. With no CIDRs
// declared every IP is allowed.
func (p *ScopePolicy) IPAllowed(ip net.IP) bool {
	if len(p.CIDRs) == 0 {
		return true
	}
	for _, c := range p.CIDRs {
		if c.Contains(ip) {
			return true
		}
	}
	return false
}

// DomainAllowed reports whether host satisfies the domain whitelist. An
// empty host (IP-literal dial targets) passes through to CIDR enforcement;
// with no Domains declared every host is allowed. Matching is
// case-insensitive and tolerates a trailing dot.
func (p *ScopePolicy) DomainAllowed(host string) bool {
	if host == "" {
		return true
	}
	if len(p.Domains) == 0 {
		return true
	}
	h := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for _, d := range p.Domains {
		if d == h {
			return true
		}
		if strings.HasPrefix(d, ".") {
			// ".example.com": apex itself plus any subdomain.
			apex := strings.TrimPrefix(d, ".")
			if h == apex || strings.HasSuffix(h, d) {
				return true
			}
		}
	}
	return false
}

// PathAllowed reports whether path avoids every forbidden pattern. With no
// patterns declared every path is allowed.
func (p *ScopePolicy) PathAllowed(path string) bool {
	for _, re := range p.ForbiddenPaths {
		if re.MatchString(path) {
			return false
		}
	}
	return true
}

// ScopeError describes one refused connection. It names the offending IP,
// host and the rule that fired; at most one of IP/Host is the cause, the
// other is context. errors.Is(err, ErrOutOfScope) holds for every
// ScopeError.
type ScopeError struct {
	IP   net.IP
	Host string
	Rule string
}

// Error implements error.
func (e *ScopeError) Error() string {
	var b strings.Builder
	b.WriteString("scopegate: target outside declared scope")
	if e.Rule != "" {
		fmt.Fprintf(&b, " (%s)", e.Rule)
	}
	if e.IP != nil {
		fmt.Fprintf(&b, ": ip=%s", e.IP)
	}
	if e.Host != "" {
		fmt.Fprintf(&b, " host=%q", e.Host)
	}
	return b.String()
}

// Unwrap exposes the sentinel for errors.Is.
func (e *ScopeError) Unwrap() error { return ErrOutOfScope }
