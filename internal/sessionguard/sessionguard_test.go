package sessionguard

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// session.go — SessionLease COW + SessionStore
// ---------------------------------------------------------------------------

func TestLeaseImmutabilityAcrossVersionSwitch(t *testing.T) {
	tests := []struct {
		name string
		v1   map[string]string
		v2   map[string]string
	}{
		{name: "cookie_value_replaced", v1: map[string]string{"sid": "alpha"}, v2: map[string]string{"sid": "beta"}},
		{name: "cookie_added", v1: map[string]string{"sid": "alpha"}, v2: map[string]string{"sid": "alpha", "tok": "t2"}},
		{name: "cookie_removed", v1: map[string]string{"sid": "alpha", "tok": "t1"}, v2: map[string]string{"sid": "alpha"}},
		{name: "empty_to_full", v1: nil, v2: map[string]string{"sid": "beta"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewSessionStore()
			old, err := store.Publish("sess", tt.v1)
			if err != nil {
				t.Fatalf("Publish v1: %v", err)
			}
			oldVersion := old.Version()
			oldLen := old.Len()

			// Snapshot the old lease's full contents before switching.
			type kv struct{ k, v string }
			var oldPairs []kv
			old.ForEach(func(k, v string) { oldPairs = append(oldPairs, kv{k, v}) })

			new, err := store.Publish("sess", tt.v2)
			if err != nil {
				t.Fatalf("Publish v2: %v", err)
			}

			if new.Version() != oldVersion+1 {
				t.Fatalf("version must increment: old=%d new=%d", oldVersion, new.Version())
			}
			if old.Version() != oldVersion {
				t.Fatalf("old lease version mutated: got %d want %d", old.Version(), oldVersion)
			}
			if old.Len() != oldLen {
				t.Fatalf("old lease cookie count mutated: got %d want %d", old.Len(), oldLen)
			}

			got := map[string]string{}
			old.ForEach(func(k, v string) { got[k] = v })
			if len(got) != len(oldPairs) {
				t.Fatalf("old lease content mutated: got %d pairs, originally %d", len(got), len(oldPairs))
			}
			for _, p := range oldPairs {
				if v, ok := got[p.k]; !ok || v != p.v {
					t.Fatalf("old lease pair (%s=%s) changed to (%s, exists=%v)", p.k, p.v, v, ok)
				}
			}

			// Store's current version must reflect v2, not the old snapshot.
			cur, err := store.Lease("sess")
			if err != nil {
				t.Fatalf("Lease after publish: %v", err)
			}
			if cur != new {
				t.Fatalf("store must hand out the newest lease after publish")
			}
			if v, ok := new.Cookie("sid"); tt.v2 != nil && (v != tt.v2["sid"] || !ok) {
				t.Fatalf("current lease sid=%q ok=%v want %q", v, ok, tt.v2["sid"])
			}
		})
	}
}

func TestLeaseIsolationFromCallerMap(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(m map[string]string)
	}{
		{name: "overwrite_value", mutate: func(m map[string]string) { m["sid"] = "hijacked" }},
		{name: "add_key", mutate: func(m map[string]string) { m["extra"] = "x" }},
		{name: "delete_key", mutate: func(m map[string]string) { delete(m, "sid") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewSessionStore()
			src := map[string]string{"sid": "alpha"}
			lease, err := store.Publish("s", src)
			if err != nil {
				t.Fatalf("Publish: %v", err)
			}
			tt.mutate(src)
			if v, _ := lease.Cookie("sid"); v != "alpha" {
				t.Fatalf("lease saw caller mutation: sid=%q", v)
			}
			if tt.name == "add_key" {
				if _, ok := lease.Cookie("extra"); ok {
					t.Fatalf("lease saw caller-added key")
				}
			}
			// NewLease directly behaves the same: fresh fixture per subtest.
			src2 := map[string]string{"sid": "alpha"}
			direct := NewLease(src2)
			tt.mutate(src2)
			if v, _ := direct.Cookie("sid"); v != "alpha" {
				t.Fatalf("NewLease aliased caller map: sid=%q", v)
			}
		})
	}
}

func TestStoreVersionMonotonicAcrossSessions(t *testing.T) {
	store := NewSessionStore()
	var last int64
	for _, tt := range []struct {
		session string
	}{{"a"}, {"b"}, {"a"}, {"c"}, {"b"}} {
		lease, err := store.Publish(tt.session, map[string]string{"k": tt.session})
		if err != nil {
			t.Fatalf("Publish %s: %v", tt.session, err)
		}
		if lease.Version() != last+1 {
			t.Fatalf("version not monotonic: got %d want %d", lease.Version(), last+1)
		}
		last = lease.Version()
	}
	versions := store.Versions()
	if len(versions) != 5 {
		t.Fatalf("Versions() len = %d want 5", len(versions))
	}
}

func TestStoreUnknownSessionAndEmptyName(t *testing.T) {
	store := NewSessionStore()
	if _, err := store.Lease("ghost"); !errors.Is(err, ErrUnknownSession) {
		t.Fatalf("Lease unknown: err=%v want ErrUnknownSession", err)
	}
	if err := store.Freeze("ghost", "x"); !errors.Is(err, ErrUnknownSession) {
		t.Fatalf("Freeze unknown: err=%v want ErrUnknownSession", err)
	}
	if err := store.Unfreeze("ghost"); !errors.Is(err, ErrUnknownSession) {
		t.Fatalf("Unfreeze unknown: err=%v want ErrUnknownSession", err)
	}
	if err := store.Delete("ghost"); !errors.Is(err, ErrUnknownSession) {
		t.Fatalf("Delete unknown: err=%v want ErrUnknownSession", err)
	}
	if _, err := store.Publish("", map[string]string{}); !errors.Is(err, ErrUnknownSession) {
		t.Fatalf("Publish empty name: err=%v want ErrUnknownSession", err)
	}
	if ok, _ := store.Frozen("ghost"); ok {
		t.Fatalf("Frozen unknown session must be (false,\"\")")
	}
}

func TestStoreFreezeRefusesPublishAndMarksLeases(t *testing.T) {
	store := NewSessionStore()
	if _, err := store.Publish("s", map[string]string{"sid": "1"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	// Snapshot taken while live.
	live, err := store.Lease("s")
	if err != nil {
		t.Fatalf("Lease: %v", err)
	}
	if err := store.Freeze("s", "canary"); err != nil {
		t.Fatalf("Freeze: %v", err)
	}
	if _, err := store.Publish("s", map[string]string{"sid": "2"}); !errors.Is(err, ErrFrozen) {
		t.Fatalf("Publish over frozen: err=%v want ErrFrozen", err)
	}
	// Frozen flag is NOT retroactively applied to pre-freeze snapshots (COW).
	if live.Frozen() {
		t.Fatalf("pre-freeze lease must not be retroactively marked frozen")
	}
	// New snapshots from the store are marked frozen.
	fr, err := store.Lease("s")
	if err != nil {
		t.Fatalf("Lease frozen: %v", err)
	}
	if !fr.Frozen() {
		t.Fatalf("post-freeze lease must be marked frozen")
	}
	if v, _ := fr.Cookie("sid"); v != "1" {
		t.Fatalf("frozen lease must still expose last cookies for audit: sid=%q", v)
	}
	ok, reason := store.Frozen("s")
	if !ok || reason != "canary" {
		t.Fatalf("Frozen()=(%v,%q) want (true,\"canary\")", ok, reason)
	}
	if err := store.Unfreeze("s"); err != nil {
		t.Fatalf("Unfreeze: %v", err)
	}
	if _, err := store.Publish("s", map[string]string{"sid": "3"}); err != nil {
		t.Fatalf("Publish after unfreeze: %v", err)
	}
	cur, _ := store.Lease("s")
	if v, _ := cur.Cookie("sid"); v != "3" {
		t.Fatalf("post-unfreeze publish lost: sid=%q", v)
	}
}

func TestStoreDeleteKeepsHeldLeasesValid(t *testing.T) {
	store := NewSessionStore()
	lease, err := store.Publish("s", map[string]string{"sid": "1"})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := store.Delete("s"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Lease("s"); !errors.Is(err, ErrUnknownSession) {
		t.Fatalf("Lease after delete: err=%v want ErrUnknownSession", err)
	}
	if v, _ := lease.Cookie("sid"); v != "1" {
		t.Fatalf("held lease invalidated by delete: sid=%q", v)
	}
}

func TestStoreConcurrentPublishLease(t *testing.T) {
	store := NewSessionStore()
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := map[int64]bool{}
	for i := range 32 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				name := fmt.Sprintf("sess%d", i%4)
				l, err := store.Publish(name, map[string]string{"sid": fmt.Sprint(i, j)})
				if err != nil {
					t.Errorf("Publish: %v", err)
					return
				}
				mu.Lock()
				if seen[l.Version()] {
					t.Errorf("duplicate version %d handed out", l.Version())
				}
				seen[l.Version()] = true
				mu.Unlock()
				got, err := store.Lease(name)
				if err != nil {
					t.Errorf("Lease: %v", err)
					return
				}
				_ = got.Len()
			}
		}(i)
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// blacklist.go — logout-semantic hard stop
// ---------------------------------------------------------------------------

func TestIsDestructiveAuthPath(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		// default marker hits
		{name: "logout_path", raw: "https://t.example/user/logout", want: true},
		{name: "logout_case_insensitive", raw: "https://t.example/User/LogOut", want: true},
		{name: "signout", raw: "https://t.example/signout", want: true},
		{name: "sign_out", raw: "https://t.example/users/sign_out", want: true},
		{name: "revoke_token", raw: "https://t.example/oauth2/revoke", want: true},
		{name: "session_destroy", raw: "https://t.example/session/destroy", want: true},
		{name: "destroy_session", raw: "https://t.example/accounts/destroy", want: true},
		{name: "end_session", raw: "https://t.example/auth/end", want: true},
		{name: "logout_with_query", raw: "https://t.example/logout?next=/home", want: true},
		{name: "session_substring", raw: "https://t.example/session/keepalive", want: true},
		// benign must pass
		{name: "login", raw: "https://t.example/login", want: false},
		{name: "dashboard", raw: "https://t.example/dashboard", want: false},
		{name: "logout_in_query_only", raw: "https://t.example/keepalive?next=logout", want: false},
		{name: "hostname_logout_ignored", raw: "https://logout.example.com/", want: false},
		{name: "empty_path", raw: "https://t.example/", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDestructiveAuthPath(mustURL(t, tt.raw)); got != tt.want {
				t.Fatalf("IsDestructiveAuthPath(%s) = %v want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestBlacklistCustomMarkers(t *testing.T) {
	b := NewBlacklist()
	if got := b.MatchString("https://t.example/kickme"); got {
		t.Fatalf("custom marker not set yet must not match")
	}
	b.SetCustom([]string{"KICKME", "  ", "invalidate-me"})
	if got := b.MatchString("https://t.example/kickme"); !got {
		t.Fatalf("custom marker kickme must match")
	}
	if got := b.MatchString("https://t.example/ Invalidate-Me "); !got {
		t.Fatalf("custom marker with spaces must match (raw fallback)")
	}
	if got := b.MatchString("https://t.example/logout"); !got {
		t.Fatalf("defaults must still match alongside custom")
	}
	b.SetCustom(nil)
	if got := b.MatchString("https://t.example/kickme"); got {
		t.Fatalf("custom cleared must not match")
	}
	if got := len(b.Custom()); got != 0 {
		t.Fatalf("Custom() len = %d want 0", got)
	}
}

func TestBlacklistEdgeInputs(t *testing.T) {
	b := NewBlacklist()
	if got := b.MatchString(""); got {
		t.Fatalf("empty string must not match")
	}
	if got := IsDestructiveAuthPath(nil); got {
		t.Fatalf("nil URL must not match")
	}
	// Unparseable junk still errs toward interception when it mentions logout.
	if got := b.MatchString("::not a url::/LOGOUT"); !got {
		t.Fatalf("raw fallback must catch logout in junk")
	}
	// Relative path-only form parses as URL with empty host; path matches.
	if got := b.MatchString("/users/sign_out"); !got {
		t.Fatalf("relative sign_out path must match")
	}
}

// ---------------------------------------------------------------------------
// canary.go — sentinel probe / freeze
// ---------------------------------------------------------------------------

func TestCanaryFreezeOnStatusDrift(t *testing.T) {
	var froze atomic.Bool
	var gotSession, gotReason atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden) // feature gone: login-only page now 403
	}))
	defer srv.Close()

	cfg := CanaryConfig{
		ProbeURL:       srv.URL + "/console",
		ExpectedStatus: http.StatusOK,
		Client:         srv.Client(),
		OnFreeze:       func(session, reason string) { froze.Store(true); gotSession.Store(session); gotReason.Store(reason) },
	}
	c, err := NewCanary("sess-a", cfg)
	if err != nil {
		t.Fatalf("NewCanary: %v", err)
	}
	hit, err := c.Probe(NewLease(map[string]string{"sid": "x"}))
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if !hit {
		t.Fatalf("status drift must be a hit")
	}
	if !froze.Load() {
		t.Fatalf("OnFreeze must fire on drift")
	}
	if gotSession.Load() != "sess-a" {
		t.Fatalf("OnFreeze session = %v", gotSession.Load())
	}
	if gotReason.Load() != ReasonStatusDrift {
		t.Fatalf("reason = %v want %s", gotReason.Load(), ReasonStatusDrift)
	}
	// Frozen canary refuses further probes without touching the wire.
	if _, err := c.Probe(nil); !errors.Is(err, ErrFrozen) {
		t.Fatalf("probe after freeze: err=%v want ErrFrozen", err)
	}
	frozen, reason := c.Frozen()
	if !frozen || reason != ReasonStatusDrift {
		t.Fatalf("Frozen()=(%v,%q)", frozen, reason)
	}
	// Unfreeze restores operation; live endpoint now → no hit.
	c.Unfreeze()
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	hit, err = c.Probe(NewLease(map[string]string{"sid": "x"}))
	if err != nil || hit {
		t.Fatalf("after unfreeze+recovery: hit=%v err=%v", hit, err)
	}
}

func TestCanaryBodySignatureDrift(t *testing.T) {
	var sig atomic.Value
	sig.Store("WELCOME-OPS-CONSOLE")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("signature probe must be GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sig.Load().(string)))
	}))
	defer srv.Close()

	cfg := CanaryConfig{
		ProbeURL:              srv.URL + "/console",
		ExpectedStatus:        http.StatusOK,
		ExpectedBodySignature: "WELCOME-OPS-CONSOLE",
		ProbeMethod:           http.MethodGet,
		Client:                srv.Client(),
	}
	c, err := NewCanary("sess-b", cfg)
	if err != nil {
		t.Fatalf("NewCanary: %v", err)
	}
	lease := NewLease(map[string]string{"sid": "x"})
	if hit, err := c.Probe(lease); err != nil || hit {
		t.Fatalf("live probe: hit=%v err=%v", hit, err)
	}
	// Body signature disappears while status stays 200 → body drift freeze.
	sig.Store("PLEASE-LOGIN")
	hit, err := c.Probe(lease)
	if err != nil {
		t.Fatalf("drift probe: %v", err)
	}
	if !hit {
		t.Fatalf("body drift must be a hit")
	}
	if _, reason := c.Frozen(); reason != ReasonBodyDrift {
		t.Fatalf("reason = %q want %s", reason, ReasonBodyDrift)
	}
}

func TestCanaryEveryNThreshold(t *testing.T) {
	tests := []struct {
		name      string
		everyN    int
		ticks     int
		wantFired int
	}{
		{name: "default_20_fires_on_20th", everyN: 0, ticks: 20, wantFired: 1},
		{name: "explicit_3_fires_twice_in_6", everyN: 3, ticks: 6, wantFired: 2},
		{name: "not_yet_at_19", everyN: 0, ticks: 19, wantFired: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()
			var probes atomic.Int32
			cfg := CanaryConfig{
				ProbeURL:       srv.URL,
				ExpectedStatus: http.StatusOK,
				EveryN:         tt.everyN,
				Client:         srv.Client(),
				OnFreeze:       func(string, string) { t.Errorf("live endpoint must not freeze") },
			}
			c, err := NewCanary("s", cfg)
			if err != nil {
				t.Fatalf("NewCanary: %v", err)
			}
			srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				probes.Add(1)
				w.WriteHeader(http.StatusOK)
			})
			fired := 0
			for range tt.ticks {
				if c.Tick() {
					fired++
					if _, err := c.Probe(NewLease(nil)); err != nil {
						t.Fatalf("Probe: %v", err)
					}
				}
			}
			if fired != tt.wantFired {
				t.Fatalf("fired %d times want %d", fired, tt.wantFired)
			}
			if got := probes.Load(); int(got) != tt.wantFired {
				t.Fatalf("probe count = %d want %d", got, tt.wantFired)
			}
		})
	}
}

func TestCanaryTickDueInterval(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c, err := NewCanary("s", CanaryConfig{
		ProbeURL:       srv.URL,
		ExpectedStatus: http.StatusOK,
		Interval:       50 * time.Millisecond,
		Client:         srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewCanary: %v", err)
	}
	if c.TickDue() {
		t.Fatalf("interval not elapsed yet")
	}
	time.Sleep(60 * time.Millisecond)
	if !c.TickDue() {
		t.Fatalf("interval elapsed must fire")
	}
	// Clock reset by the fire: immediately after, not due.
	if c.TickDue() {
		t.Fatalf("clock must reset after firing")
	}
}

func TestCanaryTransportErrorDoesNotFreeze(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // dead target → transport failure

	c, err := NewCanary("s", CanaryConfig{
		ProbeURL:       url,
		ExpectedStatus: http.StatusOK,
		Client:         &http.Client{Timeout: 2 * time.Second},
	})
	if err != nil {
		t.Fatalf("NewCanary: %v", err)
	}
	hit, err := c.Probe(NewLease(nil))
	if err == nil {
		t.Fatalf("transport failure must return error")
	}
	if hit {
		t.Fatalf("transport failure must not be a hit")
	}
	if frozen, _ := c.Frozen(); frozen {
		t.Fatalf("transport failure must not freeze")
	}
}

func TestCanaryValidationAndCookieAttachment(t *testing.T) {
	var gotCookie atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ck, err := r.Cookie("sid"); err == nil {
			gotCookie.Store(ck.Value)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tests := []struct {
		name    string
		cname   string
		cfg     CanaryConfig
		wantErr bool
	}{
		{name: "missing_name", cname: "", cfg: CanaryConfig{ProbeURL: "http://x", ExpectedStatus: 200}, wantErr: true},
		{name: "missing_probe_url", cname: "s", cfg: CanaryConfig{ExpectedStatus: 200}, wantErr: true},
		{name: "missing_expected_status", cname: "s", cfg: CanaryConfig{ProbeURL: "http://x"}, wantErr: true},
		{name: "valid", cname: "s", cfg: CanaryConfig{ProbeURL: srv.URL, ExpectedStatus: 200, Client: srv.Client()}, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCanary(tt.cname, tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewCanary err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}

	c, err := NewCanary("s", CanaryConfig{ProbeURL: srv.URL, ExpectedStatus: 200, Client: srv.Client()})
	if err != nil {
		t.Fatalf("NewCanary: %v", err)
	}
	lease := NewLease(map[string]string{"sid": "SESS-123"})
	if _, err := c.Probe(lease); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if gotCookie.Load() != "SESS-123" {
		t.Fatalf("probe must attach lease cookies, got %v", gotCookie.Load())
	}
}

func TestCanaryDefaults(t *testing.T) {
	c, err := NewCanary("s", CanaryConfig{
		ProbeURL:       "http://127.0.0.1:1/x", // unreachable; never probed here
		ExpectedStatus: 200,
	})
	if err != nil {
		t.Fatalf("NewCanary: %v", err)
	}
	// 19 ticks with default EveryN=20 must not ask for a probe.
	for i := range defaultEveryN - 1 {
		if c.Tick() {
			t.Fatalf("default threshold fired early at tick %d", i+1)
		}
	}
	if !c.Tick() {
		t.Fatalf("default threshold must fire on tick %d", defaultEveryN)
	}
	if c.interval() != defaultInterval {
		t.Fatalf("interval default = %v", c.interval())
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}
