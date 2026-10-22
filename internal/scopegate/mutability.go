package scopegate

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// MutabilityLevel labels the side-effect class of a tool action.
type MutabilityLevel int

// Side-effect tiers, least to most intrusive.
const (
	// L0Passive: observation only, no traffic aimed at the target beyond
	// ambient infrastructure queries (whois, passive DNS, local file
	// analysis).
	L0Passive MutabilityLevel = iota
	// L1ReadOnly: active but non-mutating target traffic (GET probes,
	// port scans, directory fuzzing).
	L1ReadOnly
	// L2StateMutating: traffic that may change target state (POST/PUT
	// payloads, login attempts, file upload, config change).
	L2StateMutating
)

// String renders the level for logs and error messages.
func (m MutabilityLevel) String() string {
	switch m {
	case L0Passive:
		return "L0"
	case L1ReadOnly:
		return "L1"
	case L2StateMutating:
		return "L2"
	default:
		return fmt.Sprintf("MutabilityLevel(%d)", int(m))
	}
}

// MutabilityError describes one refused tool action. Exactly one of
// ErrRateLimited / ErrAttemptsExhausted / ErrForbiddenPath / ErrNotAdmitted
// unwraps from it.
type MutabilityError struct {
	Tool     string
	Endpoint string
	Level    MutabilityLevel
	Rule     string
	Err      error
}

// Error implements error.
func (e *MutabilityError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "scopegate: %s action tool=%q endpoint=%q refused (%s)", e.Level, e.Tool, e.Endpoint, e.Rule)
	if e.Err != nil {
		fmt.Fprintf(&b, ": %v", e.Err)
	}
	return b.String()
}

// Unwrap exposes the cause for errors.Is / errors.As.
func (e *MutabilityError) Unwrap() error { return e.Err }

// RateLimiter is a token bucket for L1 read-only traffic.
type RateLimiter struct {
	mu       sync.Mutex
	tokens   float64
	max      float64
	refill   float64 // tokens per second
	lastFill time.Time
}

// NewRateLimiter returns a bucket holding max tokens, refilling at perSec
// tokens per second, starting full.
func NewRateLimiter(max int, perSec float64) *RateLimiter {
	return &RateLimiter{tokens: float64(max), max: float64(max), refill: perSec, lastFill: time.Now()}
}

// Allow spends one token, refilling elapsed time first. It returns false
// without consuming anything when the bucket is empty.
func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	r.tokens += now.Sub(r.lastFill).Seconds() * r.refill
	if r.tokens > r.max {
		r.tokens = r.max
	}
	r.lastFill = now
	if r.tokens < 1 {
		return false
	}
	r.tokens--
	return true
}

// EndpointTries is the L2StateMutating budget: how many attempts one
// endpoint may absorb. Exceeding the cap refuses further attempts.
type EndpointTries struct {
	mu   sync.Mutex
	used map[string]int
	cap  int
}

// NewEndpointTries builds a budget of cap attempts per endpoint key.
func NewEndpointTries(capacity int) *EndpointTries {
	return &EndpointTries{used: make(map[string]int), cap: capacity}
}

// Spend reserves one attempt for endpoint. When the endpoint still has
// budget it consumes one and returns true; at or past the cap it returns
// false without counting.
func (e *EndpointTries) Spend(endpoint string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.used[endpoint] >= e.cap {
		return false
	}
	e.used[endpoint]++
	return true
}

// Used reports how many attempts endpoint has consumed.
func (e *EndpointTries) Used(endpoint string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.used[endpoint]
}

// Budget defaults: L1 token bucket 10 req/s, L2 per-endpoint attempts 15.
const (
	DefaultL1PerSec      = 10.0
	DefaultL1Burst       = 10
	DefaultL2MaxAttempts = 15
)

// HighImpactPathPattern matches request paths carrying destructive or
// credential semantics. L2 actions whose endpoint path hits it are refused
// outright, before any budget is spent.
var HighImpactPathPattern = regexp.MustCompile(`(?i)(delete|drop|reset|password|config|shutdown)`)

// MutabilityConfig tunes the side-effect gate. Zero-value fields fall back
// to the defaults documented on each field.
type MutabilityConfig struct {
	// L1PerSec caps L1 read-only traffic; default 10 requests/second.
	L1PerSec float64
	// L1Burst is the token-bucket depth; default 10.
	L1Burst int
	// L2MaxAttempts caps state-mutating attempts per endpoint; default 15.
	L2MaxAttempts int
	// Registry overrides the built-in tool classification table. Keys are
	// tool names (case-insensitive), values their level.
	Registry map[string]MutabilityLevel
}

// Mutability is the side-effect gate. Classify feeds it tool names; Admit
// turns a tool plus endpoint into an allowance or a *MutabilityError.
type Mutability struct {
	registry map[string]MutabilityLevel
	cfg      MutabilityConfig
	l1       *RateLimiter
	l2       *EndpointTries
}

// NewMutability builds a gate from cfg, applying defaults for zero fields
// and installing the built-in tool registry under any overrides.
func NewMutability(cfg MutabilityConfig) *Mutability {
	if cfg.L1PerSec <= 0 {
		cfg.L1PerSec = DefaultL1PerSec
	}
	if cfg.L1Burst <= 0 {
		cfg.L1Burst = DefaultL1Burst
	}
	if cfg.L2MaxAttempts <= 0 {
		cfg.L2MaxAttempts = DefaultL2MaxAttempts
	}
	reg := make(map[string]MutabilityLevel, len(defaultRegistry)+len(cfg.Registry))
	for k, v := range defaultRegistry {
		reg[strings.ToLower(k)] = v
	}
	for k, v := range cfg.Registry {
		reg[strings.ToLower(k)] = v
	}
	return &Mutability{
		registry: reg,
		cfg:      cfg,
		l1:       NewRateLimiter(cfg.L1Burst, cfg.L1PerSec),
		l2:       NewEndpointTries(cfg.L2MaxAttempts),
	}
}

// defaultRegistry classifies the shipped Ouroboros nativetools. Tools absent
// from every registry classify as L1: throttled, never silently allowed to
// mutate.
var defaultRegistry = map[string]MutabilityLevel{
	// Active read-only target traffic.
	"http_probe":          L1ReadOnly,
	"dns_probe":           L1ReadOnly,
	"dnsrecon":            L1ReadOnly,
	"port_scan":           L1ReadOnly,
	"service_fingerprint": L1ReadOnly,
	"path_fuzz":           L1ReadOnly,
	"subdomain_discovery": L1ReadOnly,
	"archive_discovery":   L1ReadOnly,
	"amass_discovery":     L1ReadOnly,
	"parameter_discovery": L1ReadOnly,
	"crawl":               L1ReadOnly,
	"tls_assessment":      L1ReadOnly,
	"testssl":             L1ReadOnly,
	"sslscan":             L1ReadOnly,
	// State-mutating or auth-affecting traffic.
	"xss_scan":            L2StateMutating,
	"injection_detection": L2StateMutating,
	"auth_test":           L2StateMutating,
	"template_scan":       L2StateMutating,
	// Passive / local analysis.
	"static_code_scan":  L0Passive,
	"static_code_mobsf": L0Passive,
	"apk_jadx":          L0Passive,
	"apk_decompile":     L0Passive,
	"apk_mobsf":         L0Passive,
	"firmware_analyze":  L0Passive,
	"secret_scan":       L0Passive,
	"trufflehog":        L0Passive,
	"metadata":          L0Passive,
	"exiftool":          L0Passive,
	"gitleaks":          L0Passive,
	"binwalk":           L0Passive,
	"artifact_ops":      L0Passive,
	"axml":              L0Passive,
	"semgrep_scan":      L0Passive,
	"env":               L0Passive,
}

// Classify resolves a tool name to its side-effect level. Unknown tools are
// treated as L1: they get throttled but never silently allowed to mutate.
func (m *Mutability) Classify(tool string) MutabilityLevel {
	if lvl, ok := m.registry[strings.ToLower(tool)]; ok {
		return lvl
	}
	return L1ReadOnly
}

// Admit decides whether one action may proceed. It returns the tool's
// registered level; a level below requested refuses with ErrNotAdmitted.
// L1 spends a rate-limiter token; L2 is first filtered against high-impact
// path semantics (no budget spent on a refusal), then consumes a
// per-endpoint attempt. Endpoint must be a stable per-target identity (the
// canonical URL with query stripped works well); each distinct endpoint
// gets its own L2 budget.
func (m *Mutability) Admit(tool, endpoint string, requested MutabilityLevel) (MutabilityLevel, error) {
	lvl := m.Classify(tool)
	if lvl < requested {
		return lvl, &MutabilityError{
			Tool: tool, Endpoint: endpoint, Level: lvl,
			Rule: fmt.Sprintf("tool level %s below requested %s", lvl, requested),
			Err:  ErrNotAdmitted,
		}
	}
	switch lvl {
	case L0Passive:
		return lvl, nil
	case L1ReadOnly:
		if !m.l1.Allow() {
			return lvl, &MutabilityError{
				Tool: tool, Endpoint: endpoint, Level: lvl,
				Rule: fmt.Sprintf("L1 token bucket empty (limit %.0f req/s)", m.cfg.L1PerSec),
				Err:  ErrRateLimited,
			}
		}
		return lvl, nil
	default: // L2StateMutating
		if path := endpointPath(endpoint); HighImpactPathPattern.MatchString(path) {
			return lvl, &MutabilityError{
				Tool: tool, Endpoint: endpoint, Level: lvl,
				Rule: fmt.Sprintf("high-impact path %q matches forbidden semantics", path),
				Err:  ErrForbiddenPath,
			}
		}
		if !m.l2.Spend(endpoint) {
			return lvl, &MutabilityError{
				Tool: tool, Endpoint: endpoint, Level: lvl,
				Rule: fmt.Sprintf("endpoint attempt budget exhausted (cap %d)", m.cfg.L2MaxAttempts),
				Err:  ErrAttemptsExhausted,
			}
		}
		return lvl, nil
	}
}

// endpointPath extracts the URL path from an endpoint key, tolerating bare
// paths and full URLs.
func endpointPath(endpoint string) string {
	if s, ok := strings.CutPrefix(endpoint, "http://"); ok {
		endpoint = s
	} else if s, ok := strings.CutPrefix(endpoint, "https://"); ok {
		endpoint = s
	}
	if i := strings.IndexByte(endpoint, '/'); i >= 0 {
		endpoint = endpoint[i:]
	} else {
		endpoint = "/"
	}
	if i := strings.IndexAny(endpoint, "?#"); i >= 0 {
		endpoint = endpoint[:i]
	}
	return endpoint
}
