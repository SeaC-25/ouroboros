package nativetools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// OpAuthTest is the hydra/medusa-equivalent operation: credential testing
// against an authorized HTTP(S) login endpoint.
const OpAuthTest = "auth_test"

// Environment variables for the auth_test handler. They are read at call time
// via os.LookupEnv (no package-level caching) so t.Setenv takes effect; unset
// variables fall back to the built-in defaults (see AGENTS.md).
const (
	// EnvAuthUserWords points at a newline-separated username wordlist file.
	EnvAuthUserWords = "OURO_AUTH_USERWORDS"
	// EnvAuthPasswords points at a newline-separated password wordlist file.
	EnvAuthPasswords = "OURO_AUTH_PASSWORDS"
	// EnvAuthLockoutThreshold is the number of consecutive failures after which
	// a username is skipped for the remaining passwords.
	EnvAuthLockoutThreshold = "OURO_AUTH_LOCKOUT_THRESHOLD"
)

const (
	defaultAuthLockoutThreshold = 3
	// authPlaceBasic, authPlaceForm and authPlaceJSON are the hydra "place"
	// values emitted in the JSONL records.
	authPlaceBasic = "http-basic"
	authPlaceForm  = "form"
	authPlaceJSON  = "json-api"
	// authProbeBodyLimit bounds each probe response read.
	authProbeBodyLimit = 512 * 1024
	// authMaxWordlistLines caps each wordlist read so a runaway dictionary
	// cannot exhaust memory.
	authMaxWordlistLines = 10000
)

// defaultAuthUsers and defaultAuthPasswords form the built-in fallback
// dictionaries used when the environment wordlists are unset.
var (
	defaultAuthUsers = []string{"admin", "root", "user", "test"}
	// Ten common weak passwords.
	defaultAuthPasswords = []string{
		"123456", "password", "12345678", "qwerty", "abc123",
		"111111", "1234567", "iloveyou", "admin", "welcome",
	}
)

// authRecord is one JSONL line of hydra-compatible output the parser reads
// (internal/app/auth_test_parser.go).
type authRecord struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Place    string `json:"place"` // "http-basic" or "form"
	Success  bool   `json:"success"`
	Attempts int    `json:"attempts"`
}

// authProbe is the verdict for one credential attempt.
type authProbe struct {
	place   string
	success bool
}

// TestAuth runs a hydra-subset credential test against an authorized login
// URL. Mode is auto-detected: a 401 response with WWW-Authenticate: Basic
// selects HTTP Basic; otherwise the handler posts a form login. In form mode
// a sentinel attempt establishes the baseline failure response; later
// responses that differ from the baseline and carry no failure marker count
// as success. Output is hydra-style JSONL, one record per attempted pair.
func TestAuth(ctx context.Context, target string, opts Options) (string, error) {
	target = normalizeURL(strings.TrimSpace(target))
	if target == "" {
		return "", errors.New("auth_test requires a target url")
	}
	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("auth_test requires an http/https target, got %q", target)
	}

	client := opts.httpClient()
	users, err := authWordlist(EnvAuthUserWords, defaultAuthUsers)
	if err != nil {
		return "", err
	}
	passwords, err := authWordlist(EnvAuthPasswords, defaultAuthPasswords)
	if err != nil {
		return "", err
	}

	mode := detectAuthMode(ctx, client, target)
	// Form mode: discover the login field names from the response HTML when
	// possible; fall back to the standard username/password pair.
	userField, passwordField := "username", "password"
	// Sentinel probe establishes the baseline failure response for form and
	// JSON API modes.
	var baseline *authResponse
	switch mode {
	case authPlaceForm:
		page, pageErr := fetchAuthPage(ctx, client, target)
		if pageErr == nil && page.body != "" {
			userField, passwordField = authFormFields(page.body)
		}
		baseline, err = attemptFormLogin(ctx, client, target, userField, passwordField, "ouroboros-sentinel", "ouroboros-sentinel-password")
		if err != nil {
			return "", fmt.Errorf("auth_test baseline probe: %w", err)
		}
	case authPlaceJSON:
		baseline, err = attemptJSONLogin(ctx, client, target, userField, passwordField, "ouroboros-sentinel", "ouroboros-sentinel-password")
		if err != nil {
			return "", fmt.Errorf("auth_test baseline probe: %w", err)
		}
	}

	lockout := authLockoutValue()
	// One worker per username walks that user's password list serially, so
	// hit-stop, lockout counting, and attempt numbers stay deterministic.
	// Cross-user concurrency is bounded by MaxWorkers; records are reassembled
	// in username order for stable JSONL output.
	results := make([][]authRecord, len(users))
	var wg sync.WaitGroup
	sem := make(chan struct{}, opts.maxWorkers())
	for i, user := range users {
		if err := ctx.Err(); err != nil {
			break
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(slot int, user string) {
			defer wg.Done()
			defer func() { <-sem }()
			results[slot] = runAuthUserAttempts(ctx, client, mode, baseline, target, userField, passwordField, user, passwords, lockout)
		}(i, user)
	}
	wg.Wait()

	records := make([]authRecord, 0, len(users)*len(passwords))
	for _, slot := range results {
		records = append(records, slot...)
	}

	var builder strings.Builder
	encoder := json.NewEncoder(&builder)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return "", fmt.Errorf("encode auth record: %w", err)
		}
	}
	return strings.TrimSuffix(builder.String(), "\n"), nil
}

// runAuthUserAttempts walks one username's password list serially: it stops
// after a successful hit, after ctx cancellation, and once the configured
// number of consecutive failures reaches the lockout threshold (account
// lockout protection). One authRecord is produced per attempt.
func runAuthUserAttempts(ctx context.Context, client *http.Client, mode string, baseline *authResponse, target, userField, passwordField, user string, passwords []string, lockout int) []authRecord {
	records := make([]authRecord, 0, len(passwords))
	failures := 0
	for _, password := range passwords {
		if err := ctx.Err(); err != nil {
			break
		}
		probe := probeAuthCredential(ctx, client, mode, baseline, target, userField, passwordField, user, password)
		records = append(records, authRecord{
			Username: user,
			Password: password,
			Place:    probe.place,
			Success:  probe.success,
			Attempts: len(records) + 1,
		})
		if probe.success {
			break
		}
		failures++
		if failures >= lockout {
			break
		}
	}
	return records
}

// probeAuthCredential attempts one credential pair and returns the verdict.
func probeAuthCredential(ctx context.Context, client *http.Client, mode string, baseline *authResponse, target, userField, passwordField, user, password string) authProbe {
	switch mode {
	case authPlaceBasic:
		resp, err := attemptBasicAuth(ctx, client, target, user, password)
		if err == nil && resp.accepted {
			return authProbe{place: authPlaceBasic, success: true}
		}
		return authProbe{place: authPlaceBasic, success: false}
	case authPlaceJSON:
		resp, err := attemptJSONLogin(ctx, client, target, userField, passwordField, user, password)
		if err == nil && authJSONSuccess(resp, baseline) {
			return authProbe{place: authPlaceJSON, success: true}
		}
		return authProbe{place: authPlaceJSON, success: false}
	default:
		resp, err := attemptFormLogin(ctx, client, target, userField, passwordField, user, password)
		if err == nil && resp != nil && authFormSuccess(resp, baseline) {
			return authProbe{place: authPlaceForm, success: true}
		}
		return authProbe{place: authPlaceForm, success: false}
	}
}

// authResponse is the normalized outcome of one login attempt.
type authResponse struct {
	statusCode int
	location   string
	body       string
	accepted   bool // Basic mode: any non-401 response.
}

// fetchAuthPage performs one GET against the login URL so the handler can
// inspect the HTML form (field names) before attempting credentials.
func fetchAuthPage(ctx context.Context, client *http.Client, target string) (*authResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ouroboros-native-hydra/1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, authProbeBodyLimit))
	if err != nil {
		return nil, err
	}
	return &authResponse{
		statusCode: resp.StatusCode,
		body:       string(body),
	}, nil
}

// attemptBasicAuth performs one HTTP Basic request. accepted is true when the
// server replied with anything other than 401.
func attemptBasicAuth(ctx context.Context, client *http.Client, target, user, password string) (*authResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, password)
	req.Header.Set("User-Agent", "ouroboros-native-hydra/1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, authProbeBodyLimit))
	if err != nil {
		return nil, err
	}
	return &authResponse{
		statusCode: resp.StatusCode,
		body:       string(body),
		accepted:   resp.StatusCode != http.StatusUnauthorized,
	}, nil
}

// attemptFormLogin performs one form POST login attempt using the discovered
// field names.
func attemptFormLogin(ctx context.Context, client *http.Client, target, userField, passwordField, user, password string) (*authResponse, error) {
	form := url.Values{}
	form.Set(userField, user)
	form.Set(passwordField, password)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "ouroboros-native-hydra/1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, authProbeBodyLimit))
	if err != nil {
		return nil, err
	}
	return &authResponse{
		statusCode: resp.StatusCode,
		location:   resp.Header.Get("Location"),
		body:       string(body),
	}, nil
}

// authFormSuccess decides whether a form response beat the baseline failure
// response. The baseline response is captured from a sentinel attempt, so
// "different from baseline and not an error indicator" means accepted.
func authFormSuccess(resp *authResponse, baseline *authResponse) bool {
	if resp == nil {
		return false
	}
	if resp.statusCode == http.StatusUnauthorized || resp.statusCode == http.StatusForbidden {
		return false
	}
	if hasAuthFailureMarker(resp.body) {
		return false
	}
	if baseline == nil {
		// No baseline (sentinel failed): treat only explicit redirects as
		// success signals.
		return resp.statusCode == http.StatusFound || resp.statusCode == http.StatusMovedPermanently || resp.statusCode == http.StatusSeeOther
	}
	if resp.statusCode == baseline.statusCode {
		// Same status: still a success when the body differs and shows a
		// redirect target change, but conservative: require a body difference.
		return resp.body != baseline.body
	}
	return true
}

// authFailureMarkers are body fragments that mark an explicit login failure.
var authFailureMarkers = []string{
	"invalid username or password",
	"invalid credentials",
	"incorrect password",
	"login failed",
	"authentication failed",
	"wrong password",
	"unknown user",
	"invalid login",
	"bad credentials",
}

func hasAuthFailureMarker(body string) bool {
	if body == "" {
		return false
	}
	lower := strings.ToLower(body)
	for _, marker := range authFailureMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// detectAuthMode picks the login mode: HTTP Basic when the target responds
// 401 with WWW-Authenticate: Basic; JSON API when the target answers with a
// JSON content type and no HTML form; form login otherwise.
func detectAuthMode(ctx context.Context, client *http.Client, target string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return authPlaceForm
	}
	req.Header.Set("User-Agent", "ouroboros-native-hydra/1")
	resp, err := client.Do(req)
	if err != nil {
		return authPlaceForm
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, authProbeBodyLimit))
	if resp.StatusCode == http.StatusUnauthorized && strings.Contains(strings.ToLower(resp.Header.Get("WWW-Authenticate")), "basic") {
		return authPlaceBasic
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "application/json") && !strings.Contains(strings.ToLower(string(body)), "<form") {
		return authPlaceJSON
	}
	return authPlaceForm
}

// attemptJSONLogin posts one credential pair as a JSON document.
func attemptJSONLogin(ctx context.Context, client *http.Client, target, userField, passwordField, user, password string) (*authResponse, error) {
	payload, err := json.Marshal(map[string]string{
		userField:     user,
		passwordField: password,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ouroboros-native-hydra/1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, authProbeBodyLimit))
	if err != nil {
		return nil, err
	}
	return &authResponse{
		statusCode: resp.StatusCode,
		location:   resp.Header.Get("Location"),
		body:       string(body),
	}, nil
}

// authJSONSuccess decides a JSON API login outcome: 401/403 are failures,
// 200 with a token-shaped field (token/jwt/session/api_key) is accepted,
// other 2xx mirror the form heuristic (differ from baseline, no error field).
func authJSONSuccess(resp *authResponse, baseline *authResponse) bool {
	if resp == nil {
		return false
	}
	if resp.statusCode == http.StatusUnauthorized || resp.statusCode == http.StatusForbidden {
		return false
	}
	if resp.statusCode < 200 || resp.statusCode >= 300 {
		return false
	}
	lower := strings.ToLower(resp.body)
	for _, tokenKey := range []string{"\"token\"", "\"jwt\"", "\"session\"", "\"api_key\"", "\"access_token\""} {
		if strings.Contains(lower, tokenKey) {
			return true
		}
	}
	if strings.Contains(lower, "\"error\"") || strings.Contains(lower, "\"message\": \"invalid") {
		return false
	}
	if baseline != nil && resp.body == baseline.body {
		return false
	}
	return true
}

// authWordlist reads a newline-separated wordlist file when the environment
// variable names one; otherwise the built-in defaults apply. A configured but
// unreadable/empty list is an error: the operator pointed the handler at a
// dictionary on purpose.
func authWordlist(envVar string, defaults []string) ([]string, error) {
	path, ok := os.LookupEnv(envVar)
	if !ok || strings.TrimSpace(path) == "" {
		return defaults, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("auth_test wordlist %s: %w", envVar, err)
	}
	defer func() { _ = file.Close() }()
	words := make([]string, 0, 64)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), authProbeBodyLimit)
	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		if word == "" || strings.HasPrefix(word, "#") {
			continue
		}
		words = append(words, word)
		if len(words) >= authMaxWordlistLines {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("auth_test wordlist %s: %w", envVar, err)
	}
	if len(words) == 0 {
		return nil, fmt.Errorf("auth_test wordlist %s: file %s contains no entries", envVar, path)
	}
	return words, nil
}

// authLockoutValue reads OURO_AUTH_LOCKOUT_THRESHOLD at call time. The value
// is the number of consecutive failures that stop probing a user. An
// explicit "0" disables lockout detection entirely (operators running
// against throwaway lab targets asked for the full wordlist); unset or
// invalid values keep the default. The disable is implemented as a sentinel
// because the consumer compares failures >= threshold per attempt.
func authLockoutValue() int {
	raw, ok := os.LookupEnv(EnvAuthLockoutThreshold)
	if !ok {
		return defaultAuthLockoutThreshold
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "0" {
		return int(^uint(0) >> 1) // max int: never reached, lockout disabled
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil || value < 1 {
		return defaultAuthLockoutThreshold
	}
	return value
}

// authFormFields parses the login form from an HTML body when present. The
// handler only needs field names; unknown/absent forms fall back to the
// standard username/password pair.
func authFormFields(body string) (string, string) {
	userField, passwordField := "username", "password"
	// <input ... name="..." ... type="password"> blocks are scanned in order;
	// the first password field marks the form, and the nearest preceding
	// non-password text/email input is the username field. Matching runs on
	// the ORIGINAL body: form field names are case-sensitive server-side
	// (userName vs username are different parameters), so lowercasing the
	// body here would silently break credential testing against camelCase
	// forms. Case-insensitivity is applied only to the attribute/type
	// comparison via the (?i) pattern prefix.
	inputPattern := regexp.MustCompile(`(?i)<input\b[^>]*>`)
	type authInput struct {
		name string
		kind string
	}
	inputs := make([]authInput, 0, 8)
	for _, match := range inputPattern.FindAllString(body, 64) {
		nameMatch := regexp.MustCompile(`name\s*=\s*"([^"]*)"|name\s*=\s*'([^']*)'|name=([^\s>]+)`).FindStringSubmatch(match)
		name := ""
		for i := 1; i < len(nameMatch); i++ {
			if nameMatch[i] != "" {
				name = html.UnescapeString(nameMatch[i])
				break
			}
		}
		typeMatch := regexp.MustCompile(`type\s*=\s*"([^"]*)"|type\s*=\s*'([^']*)'|type=([^\s>]+)`).FindStringSubmatch(match)
		kind := "text"
		for i := 1; i < len(typeMatch); i++ {
			if typeMatch[i] != "" {
				kind = strings.ToLower(html.UnescapeString(typeMatch[i]))
				break
			}
		}
		if name == "" {
			continue
		}
		inputs = append(inputs, authInput{name: name, kind: kind})
	}
	lastText := ""
	for _, input := range inputs {
		switch input.kind {
		case "password":
			if input.name != "" {
				passwordField = input.name
				if lastText != "" {
					userField = lastText
				}
				return userField, passwordField
			}
		case "text", "email", "tel", "":
			lastText = input.name
		}
	}
	return userField, passwordField
}
