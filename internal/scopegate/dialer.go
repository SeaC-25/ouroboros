package scopegate

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// GuardedDialer is a *net.Dialer wrapper enforcing a ScopePolicy inside the
// DialContext hook: every resolved address is checked against the CIDR
// allowlist and the request host against the domain allowlist before any
// TCP traffic is emitted.
type GuardedDialer struct {
	policy *ScopePolicy
	base   *net.Dialer
}

// NewGuardedDialerStruct returns a GuardedDialer whose GuardHTTPClient
// produces scope-guarded HTTP clients (callers that only need the raw
// DialContext can use NewGuardedDialer instead).
func NewGuardedDialerStruct(policy *ScopePolicy, base *net.Dialer) *GuardedDialer {
	if base == nil {
		base = &net.Dialer{}
	}
	return &GuardedDialer{policy: policy, base: base}
}

// Validate reports whether the wrapped policy is usable (non-nil; the
// loader LoadScopePolicyFile already validated its shape).
func (g *GuardedDialer) Validate() error {
	if g == nil || g.policy == nil {
		return fmt.Errorf("guarded dialer: nil policy")
	}
	return nil
}

// NewGuardedDialer wraps base (a fresh default dialer when nil) with policy
// enforcement. The returned function plugs directly into
// http.Transport.DialContext.
func NewGuardedDialer(policy *ScopePolicy, base *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if base == nil {
		base = &net.Dialer{}
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if err := policy.checkDial(ctx, base, network, addr); err != nil {
			return nil, err
		}
		return base.DialContext(ctx, network, addr)
	}
}

// GuardHTTPClient returns an *http.Client whose transport dials exclusively
// through policy-guarded dialing. When next is an *http.Transport a copy is
// taken so its settings (TLS config, timeouts) are inherited without
// mutating the original; proxy support is stripped because a proxy would
// bypass the scope check. Redirects stay enabled: every hop dials through
// the same guarded transport, so a cross-domain redirect is re-checked at
// the new host.
func (g *GuardedDialer) GuardHTTPClient(next ...http.RoundTripper) *http.Client {
	var transport *http.Transport
	if len(next) > 0 {
		if t, ok := next[0].(*http.Transport); ok && t != nil {
			transport = t.Clone()
		}
	}
	if transport == nil {
		transport = &http.Transport{}
	}
	transport.Proxy = nil
	transport.DialContext = NewGuardedDialer(g.policy, g.base)
	return &http.Client{Transport: transport}
}

// checkDial applies domain, forbidden-path and CIDR rules to one dial
// attempt, in that order. When base carries a Resolver the host is expanded
// to all resolved addresses before dialing, so a multi-record host
// partially outside the CIDR list refuses before a single packet leaves.
// IP-literal targets skip DNS and are validated directly.
func (p *ScopePolicy) checkDial(ctx context.Context, base *net.Dialer, network, addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return &ScopeError{Host: addr, Rule: "address"}
	}
	if host == "" {
		return &ScopeError{Host: addr, Rule: "empty host"}
	}
	for _, re := range p.ForbiddenPaths {
		if re.MatchString(host) {
			return &ScopeError{Host: host, Rule: "forbidden path pattern: " + re.String()}
		}
	}
	if !isIPLiteral(host) && !p.DomainAllowed(host) {
		return &ScopeError{Host: host, Rule: "domain allowlist"}
	}
	ips, err := p.resolve(ctx, base, host)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if !p.IPAllowed(ip) {
			return &ScopeError{IP: ip, Host: host, Rule: "cidr allowlist"}
		}
	}
	return nil
}

// resolve turns the dial address into candidate IPs. IP literals skip DNS.
// Dialers carrying a Resolver have each name expanded to all addresses so a
// multi-A-record host partially outside the CIDR list is caught. Unknown
// hosts refuse with a *ScopeError; transient resolver failures return a
// plain error.
func (p *ScopePolicy) resolve(ctx context.Context, base *net.Dialer, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	resolver := base.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return nil, &ScopeError{Host: host, Rule: "dns: unknown host"}
		}
		return nil, fmt.Errorf("scopegate: resolve %q: %w", host, err)
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, net.IP(a.AsSlice()))
	}
	return ips, nil
}

func isIPLiteral(host string) bool {
	return net.ParseIP(strings.Trim(host, "[]")) != nil
}
