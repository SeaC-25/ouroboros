package api

// tce_gate.go — Target Causal Epoch + request fingerprint cache (v28.1).
//
// FM-3 root cause #2: the model re-observes the same URL because the target
// state MIGHT have changed — but in practice it almost never did. A blunt
// "same URL forbidden" is wrong (legitimate re-tests after real state
// changes); the cache must be keyed on a CAUSAL EPOCH that only advances
// when the remote target actually mutates:
//
//   - non-idempotent observe (POST/PUT/DELETE/PATCH) whose diff shows real
//     change (status change / new cookies / body change), or
//   - an EXECUTE whose command targets the challenge host (egress-detected).
//
// Local workspace writes/computation never advance the epoch (that was the
// flaw in the naive state_epoch proposal — sub-loops write local scripts
// every other turn, which would have invalidated the cache constantly).

import (
	"path/filepath"

	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ouroboros/ouroboros/internal/app"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// tceRegistry tracks per-target epochs and fingerprint -> obs-id cache.
type tceRegistry struct {
	mu       sync.Mutex
	epochs   map[string]int    // target host -> causal epoch
	fingerp  map[string]string // fingerprint -> obs-id
	snippets map[string]string // fingerprint -> cached distilled summary (for dedup hit rendering)
}

var globalTCERegistry = &tceRegistry{
	epochs:   map[string]int{},
	fingerp:  map[string]string{},
	snippets: map[string]string{},
}

// tceTargetKey derives the registry key from a target URL (host[:port]).
func tceTargetKey(targetURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil || parsed.Host == "" {
		return strings.TrimSpace(targetURL)
	}
	return parsed.Host
}

// observeFingerprint hashes the full request shape + current epoch.
func observeFingerprint(targetURL string, specJSON string, epoch int) string {
	spec := struct {
		Method string            `json:"method"`
		Path   string            `json:"path"`
		Query  map[string]string `json:"query"`
		Body   string            `json:"body"`
	}{}
	_ = json.Unmarshal([]byte(specJSON), &spec)
	if spec.Method == "" {
		spec.Method = "GET"
	}
	// Canonical query: sorted keys.
	qkeys := make([]string, 0, len(spec.Query))
	for k := range spec.Query {
		qkeys = append(qkeys, k)
	}
	sort.Strings(qkeys)
	var qb strings.Builder
	for _, k := range qkeys {
		qb.WriteString(k + "=" + spec.Query[k] + "&")
	}
	// The request path comes from BOTH the spec path and the target URL's
	// own path — models often pass no spec (path empty) and differentiate
	// requests purely by the target URL. Include both, normalized.
	targetPath := spec.Path
	if parsed, perr := url.Parse(strings.TrimSpace(targetURL)); perr == nil && targetPath == "" {
		targetPath = parsed.Path
	}
	if targetPath == "" {
		targetPath = "/"
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%s|%d",
		tceTargetKey(targetURL), strings.ToUpper(spec.Method), targetPath, qb.String(), spec.Body, epoch)))
	return hex.EncodeToString(sum[:16])
}

// isCachedObservation reports whether this exact request (at the current
// epoch) already has a cached observation.
func (s *Server) isCachedObservation(targetURL string, specJSON string) bool {
	globalTCERegistry.mu.Lock()
	defer globalTCERegistry.mu.Unlock()
	key := tceTargetKey(targetURL)
	fp := observeFingerprint(targetURL, specJSON, globalTCERegistry.epochs[key])
	_, hit := globalTCERegistry.fingerp[fp]
	return hit
}

// cachedObservationHit renders the dedup response (no network request).
func (s *Server) cachedObservationHit(targetURL string, specJSON string) (string, bool) {
	globalTCERegistry.mu.Lock()
	defer globalTCERegistry.mu.Unlock()
	key := tceTargetKey(targetURL)
	fp := observeFingerprint(targetURL, specJSON, globalTCERegistry.epochs[key])
	obsID, ok := globalTCERegistry.fingerp[fp]
	if !ok {
		return "", false
	}
	snippet := globalTCERegistry.snippets[fp]
	if len(snippet) > 400 {
		snippet = snippet[:400]
	}
	return fmt.Sprintf("[DEDUP_CACHE_HIT] 该请求在当前远端因果纪元 (TCE=%d) 下已执行过（见 %s，响应指纹未变）。既有蒸馏快照：%s\n如需原始细节：ACTION:INSPECT %s <selector>；关联结论：ACTION:QUERY。远端状态变化后纪元自动递增，届时可重新观测。",
		globalTCERegistry.epochs[key], obsID, snippet, obsID), true
}

// tceRememberObservation caches a successful observation.
func (s *Server) tceRememberObservation(targetURL string, specJSON string, obsID string, rawResultJSON string) {
	globalTCERegistry.mu.Lock()
	defer globalTCERegistry.mu.Unlock()
	key := tceTargetKey(targetURL)
	fp := observeFingerprint(targetURL, specJSON, globalTCERegistry.epochs[key])
	globalTCERegistry.fingerp[fp] = obsID
	// Cache the distilled form of this observation for dedup rendering.
	globalTCERegistry.snippets[fp] = distillObservationForAI(rawResultJSON)
	// Post-hoc epoch evaluation: a non-idempotent request with a real diff
	// advances the epoch — note this uses the JUST-COMPLETED epoch's
	// fingerprint for the cache entry above (the response belongs to the
	// state that produced it), and future requests will see the new epoch.
	if tceShouldBump(specJSON, rawResultJSON) {
		globalTCERegistry.epochs[key]++
	}
}

// tceShouldBump evaluates whether the remote target actually mutated.
func tceShouldBump(specJSON string, rawResultJSON string) bool {
	spec := struct {
		Method string `json:"method"`
	}{}
	_ = json.Unmarshal([]byte(specJSON), &spec)
	method := strings.ToUpper(strings.TrimSpace(spec.Method))
	if method != "POST" && method != "PUT" && method != "DELETE" && method != "PATCH" {
		return false
	}
	// Look for a non-empty diff in the raw observation.
	var result struct {
		Diff *struct {
			StatusChanged bool `json:"status_changed"`
			BodyChanged   bool `json:"body_changed"`
		} `json:"diff"`
		SetCookies []string `json:"set_cookies"`
	}
	if json.Unmarshal([]byte(rawResultJSON), &result) != nil {
		return false
	}
	if result.Diff != nil && (result.Diff.StatusChanged || result.Diff.BodyChanged) {
		return true
	}
	// New session cookies are also a state signal on auth flows.
	return len(result.SetCookies) > 0
}

// tceBumpForCommand advances the epoch for any target host a shell command
// interacts with (called from the egress gate path — conservative: a command
// merely CONTAINING the host string bumps it; over-bumping only costs one
// re-request, under-bumping serves a stale cache).
func tceBumpForCommand(command string, inScopeHosts []string) {
	globalTCERegistry.mu.Lock()
	defer globalTCERegistry.mu.Unlock()
	for _, host := range inScopeHosts {
		if host != "" && strings.Contains(command, host) {
			globalTCERegistry.epochs[host]++
		}
	}
}

// scopeTargetHosts lists the in-scope target hosts (for epoch bumping on
// EXECUTE commands that interact with them).
func (s *Server) scopeTargetHosts() []string {
	config, err := app.LoadScopeConfig(filepath.Join(s.workspace, "scope.yaml"))
	if err != nil {
		return nil
	}
	hosts := make([]string, 0, len(config.Scope.Targets))
	for _, t := range config.Scope.Targets {
		if parsed, perr := url.Parse(strings.TrimSpace(t.Value)); perr == nil && parsed.Host != "" {
			hosts = append(hosts, parsed.Host)
		}
	}
	return hosts
}
