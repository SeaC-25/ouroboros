package sessionguard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CanaryConfig tunes the sentinel that watches one session.
type CanaryConfig struct {
	// ProbeURL is the login-only-visible endpoint the canary HEADs. Required.
	ProbeURL string
	// ExpectedStatus is the status code a live session must see. Required;
	// anything else is treated as status drift.
	ExpectedStatus int
	// ExpectedBodySignature is a substring the live body must contain. When
	// empty, body-signature checking is disabled (status-only mode).
	ExpectedBodySignature string
	// EveryN is the number of stateful requests between probes. Zero or
	// negative selects the default of 20.
	EveryN int
	// Interval is the wall-clock interval between probes regardless of
	// request count. Zero or negative selects the default of 3 minutes.
	Interval time.Duration
	// Client overrides the HTTP client used for probes (tests). A nil client
	// uses a dedicated client with no cookie jar, default timeouts and no
	// redirects followed.
	Client *http.Client
	// ProbeMethod overrides the probe HTTP method; default and zero value is
	// HEAD. Set to GET when the probe needs a body signature. The body is
	// only signature-checked for GET.
	ProbeMethod string
	// OnFreeze, when non-nil, is invoked exactly once when the canary freezes
	// the session. Called on the probing goroutine; must not block.
	OnFreeze func(session, reason string)
}

const (
	defaultEveryN   = 20
	defaultInterval = 3 * time.Minute
	probeTimeout    = 10 * time.Second
)

// Canary is the sentinel watching one session's authentication health. It is
// triggered either by request counting (every EveryN stateful requests) or by
// the wall clock (every Interval), whichever comes first, and sends a
// lightweight HEAD to the configured login-only-visible probe endpoint.
//
// A probe verdict:
//   - status code differs from ExpectedStatus → freeze (ReasonStatusDrift)
//   - body signature differs (when configured) → freeze (ReasonBodyDrift)
//   - transport-level failure → wrapped error returned, no freeze
//
// Freeze is one-shot: after a freeze the canary stops probing and refuses
// further Tick/TickDue/Probe calls. Unfreeze restores operation.
type Canary struct {
	name   string
	cfg    CanaryConfig
	client *http.Client

	mu           sync.Mutex
	statefulN    int
	lastProbe    time.Time
	frozen       bool
	freezeReason string
}

// NewCanary validates cfg and returns a canary watching session name.
func NewCanary(name string, cfg CanaryConfig) (*Canary, error) {
	if name == "" {
		return nil, errors.New("sessionguard: canary session name required")
	}
	if cfg.ProbeURL == "" {
		return nil, errors.New("sessionguard: canary probe URL required")
	}
	if cfg.ExpectedStatus == 0 {
		return nil, errors.New("sessionguard: canary expected status required")
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{
			Timeout: probeTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Never follow redirects: a logout endpoint would
				// otherwise be followed straight to the login page and
				// masquerade as a live session.
				return http.ErrUseLastResponse
			},
		}
	}
	return &Canary{
		name:      name,
		cfg:       cfg,
		client:    client,
		lastProbe: time.Now(),
	}, nil
}

// Tick records one stateful request. When the count reaches EveryN it resets
// the counter and returns true, telling the caller to run Probe now.
func (c *Canary) Tick() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.frozen {
		return false
	}
	c.statefulN++
	if c.statefulN >= c.everyN() {
		c.statefulN = 0
		return true
	}
	return false
}

// TickDue reports whether the wall-clock interval has elapsed since the last
// probe and, if so, resets the clock (the caller should run Probe now). It
// never freezes on its own; Probe delivers the verdict.
func (c *Canary) TickDue() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.frozen {
		return false
	}
	if time.Since(c.lastProbe) >= c.interval() {
		c.lastProbe = time.Now()
		return true
	}
	return false
}

// Probe sends the HEAD to ProbeURL with the lease's cookies and returns the
// verdict. A nil lease probes without cookies (bare reachability). An
// already-frozen canary refuses with ErrFrozen and does not touch the wire.
//
// The verdict:
//   - live: nil error, hit=false
//   - feature vanished: nil error, hit=true (freeze callback fired)
//   - transport failure: wrapped error, hit=false, no freeze
//   - frozen: ErrFrozen, no wire traffic
func (c *Canary) Probe(lease *Lease) (hit bool, err error) {
	c.mu.Lock()
	if c.frozen {
		c.mu.Unlock()
		return false, ErrFrozen
	}
	c.mu.Unlock()

	code, body, err := c.doProbe(lease)
	if err != nil {
		// Transport failure is inconclusive: the target may be down,
		// rate-limiting, or the network flapping. Count it, never freeze.
		return false, fmt.Errorf("sessionguard: probe %s: %w", c.name, err)
	}

	freeze := false
	reason := ""
	switch {
	case code != c.cfg.ExpectedStatus:
		freeze, reason = true, ReasonStatusDrift
	case c.cfg.ExpectedBodySignature != "" && !strings.Contains(body, c.cfg.ExpectedBodySignature):
		freeze, reason = true, ReasonBodyDrift
	}

	if freeze {
		c.mu.Lock()
		if c.frozen {
			c.mu.Unlock()
			return true, nil // someone else froze first; still a hit
		}
		c.frozen = true
		c.freezeReason = reason
		c.mu.Unlock()
		if c.cfg.OnFreeze != nil {
			c.cfg.OnFreeze(c.name, reason)
		}
	}
	return freeze, nil
}

// ProbeNow is Probe plus TickDue-style bookkeeping: it records now as the
// last-probe time so the wall-clock arm does not re-fire immediately after a
// successful manual probe.
func (c *Canary) ProbeNow(lease *Lease) (bool, error) {
	hit, err := c.Probe(lease)
	c.mu.Lock()
	c.lastProbe = time.Now()
	c.mu.Unlock()
	return hit, err
}

// Frozen reports the freeze state and reason.
func (c *Canary) Frozen() (bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.frozen, c.freezeReason
}

// Unfreeze lifts a freeze; the next Tick/TickDue/Probe cycle starts fresh.
func (c *Canary) Unfreeze() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.frozen = false
	c.freezeReason = ""
	c.statefulN = 0
}

// everyN returns the effective request threshold.
func (c *Canary) everyN() int {
	if c.cfg.EveryN > 0 {
		return c.cfg.EveryN
	}
	return defaultEveryN
}

// interval returns the effective wall-clock interval.
func (c *Canary) interval() time.Duration {
	if c.cfg.Interval > 0 {
		return c.cfg.Interval
	}
	return defaultInterval
}

// doProbe performs the actual probe request. It exists so Probe stays readable.
func (c *Canary) doProbe(lease *Lease) (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()

	// HEAD by default: lightweight, login-only-visible feature check.
	// ProbeMethod (e.g. GET) opts into a body-bearing probe when the probe
	// needs a signature; the body is only read in that case.
	method := c.cfg.ProbeMethod
	if method == "" {
		method = http.MethodHead
	}

	req, err := http.NewRequestWithContext(ctx, method, c.cfg.ProbeURL, nil)
	if err != nil {
		return 0, "", fmt.Errorf("sessionguard: build probe request: %w", err)
	}
	if lease != nil {
		lease.ForEach(func(name, value string) {
			req.AddCookie(&http.Cookie{Name: name, Value: value})
		})
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	// HEAD has no body per spec. Only GET-mode probes carry a body worth
	// signature-checking; for HEAD the empty string means "no signature".
	body := ""
	if method == http.MethodGet {
		var sb strings.Builder
		_, _ = io.Copy(&sb, resp.Body)
		body = sb.String()
	}
	return resp.StatusCode, body, nil
}
