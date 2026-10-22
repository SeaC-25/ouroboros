package nativetools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// authTestRequestCount is a mutex-guarded per-user attempt counter for the
// httptest servers in this file.
type authTestRequestCount struct {
	mu    sync.Mutex
	count map[string]int
}

func newAuthTestRequestCount() *authTestRequestCount {
	return &authTestRequestCount{count: map[string]int{}}
}

func (c *authTestRequestCount) add(user string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count[user]++
}

func (c *authTestRequestCount) get(user string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count[user]
}

func decodeAuthJSONL(t *testing.T, output string) []authRecord {
	t.Helper()
	records := []authRecord{}
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record authRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode auth JSONL line %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

// writeAuthWordlists writes the two env wordlist files and returns their
// paths; passwords are ordered so assertion order is deterministic.
func writeAuthWordlists(t *testing.T, users, passwords []string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	usersFile := filepath.Join(dir, "users.txt")
	passwordsFile := filepath.Join(dir, "passwords.txt")
	if err := os.WriteFile(usersFile, []byte(strings.Join(users, "\n")), 0o600); err != nil {
		t.Fatalf("write users wordlist: %v", err)
	}
	if err := os.WriteFile(passwordsFile, []byte(strings.Join(passwords, "\n")), 0o600); err != nil {
		t.Fatalf("write passwords wordlist: %v", err)
	}
	t.Setenv(EnvAuthUserWords, usersFile)
	t.Setenv(EnvAuthPasswords, passwordsFile)
	return usersFile, passwordsFile
}

func TestTestAuthHTTPBasicFindsCredentials(t *testing.T) {
	writeAuthWordlists(t, []string{"admin"}, []string{"letmein", "s3cret"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if ok && user == "admin" && password == "s3cret" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("welcome"))
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	out, err := TestAuth(context.Background(), server.URL, Options{Timeout: 20 * time.Second, MaxWorkers: 4})
	if err != nil {
		t.Fatalf("TestAuth: %v", err)
	}
	records := decodeAuthJSONL(t, out)
	if len(records) != 2 {
		t.Fatalf("expected 2 records from wordlist, got %#v", records)
	}
	if records[0].Place != "http-basic" || records[1].Place != "http-basic" {
		t.Fatalf("expected http-basic place, got %#v", records)
	}
	if !records[1].Success || records[1].Password != "s3cret" || records[1].Attempts != 2 {
		t.Fatalf("expected second password s3cret to hit, got %#v", records)
	}
	if records[0].Success {
		t.Fatalf("expected letmein to fail, got %#v", records)
	}
}

func TestTestAuthFormLoginFindsCredentials(t *testing.T) {
	writeAuthWordlists(t, []string{"admin"}, []string{"letmein", "s3cret"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			_, _ = w.Write([]byte(`<form><input name="user" type="text"><input name="pass" type="password"></form>`))
			return
		}
		_ = r.ParseForm()
		if r.PostFormValue("user") == "admin" && r.PostFormValue("pass") == "s3cret" {
			http.Redirect(w, r, "/dashboard", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<p>invalid username or password</p>`))
	}))
	defer server.Close()

	out, err := TestAuth(context.Background(), server.URL, Options{Timeout: 20 * time.Second, MaxWorkers: 4})
	if err != nil {
		t.Fatalf("TestAuth: %v", err)
	}
	records := decodeAuthJSONL(t, out)
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %#v", records)
	}
	if records[0].Place != "form" {
		t.Fatalf("expected form place, got %#v", records)
	}
	if !records[1].Success || records[1].Password != "s3cret" {
		t.Fatalf("expected s3cret form hit, got %#v", records)
	}
}

func TestTestAuthFormLoginUsesDiscoveredFieldNames(t *testing.T) {
	writeAuthWordlists(t, []string{"admin"}, []string{"s3cret"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			_, _ = w.Write([]byte(`<form><input name="login" type="text"><input name="pwd" type="password"></form>`))
			return
		}
		_ = r.ParseForm()
		if r.PostFormValue("login") == "admin" && r.PostFormValue("pwd") == "s3cret" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<p>hello dashboard</p>"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<p>login failed</p>"))
	}))
	defer server.Close()

	out, err := TestAuth(context.Background(), server.URL, Options{Timeout: 20 * time.Second, MaxWorkers: 4})
	if err != nil {
		t.Fatalf("TestAuth: %v", err)
	}
	records := decodeAuthJSONL(t, out)
	if len(records) != 1 || !records[0].Success {
		t.Fatalf("expected admin/s3cret hit with discovered field names, got %#v", records)
	}
}

func TestTestAuthStopsUserAfterHit(t *testing.T) {
	writeAuthWordlists(t, []string{"admin", "root"}, []string{"letmein", "s3cret", "password123", "welcome1"})
	counter := newAuthTestRequestCount()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if ok {
			counter.add(user)
		}
		if ok && user == "admin" && password == "s3cret" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	out, err := TestAuth(context.Background(), server.URL, Options{Timeout: 20 * time.Second, MaxWorkers: 4})
	if err != nil {
		t.Fatalf("TestAuth: %v", err)
	}
	records := decodeAuthJSONL(t, out)
	adminHits := 0
	rootRecords := 0
	for _, record := range records {
		switch record.Username {
		case "admin":
			if record.Success {
				adminHits++
			}
		case "root":
			rootRecords++
		}
	}
	if adminHits != 1 {
		t.Fatalf("expected exactly one admin hit, got %#v", records)
	}
	if got := counter.get("admin"); got != 2 {
		t.Fatalf("expected admin to stop right after the hit (2 attempts), got %d", got)
	}
	// root never hits: default lockout threshold (3 consecutive failures)
	// stops it after three passwords even without a hit.
	if rootRecords != 3 {
		t.Fatalf("expected root to stop at the lockout threshold (3 records), got %d", rootRecords)
	}
}

func TestTestAuthLockoutSkipsUserAfterThreshold(t *testing.T) {
	t.Setenv(EnvAuthLockoutThreshold, "2")
	t.Setenv(EnvAuthUserWords, "")
	t.Setenv(EnvAuthPasswords, "")
	counter := newAuthTestRequestCount()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _, ok := r.BasicAuth()
		if ok {
			counter.add(user)
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	out, err := TestAuth(context.Background(), server.URL, Options{Timeout: 20 * time.Second, MaxWorkers: 4})
	if err != nil {
		t.Fatalf("TestAuth: %v", err)
	}
	records := decodeAuthJSONL(t, out)
	perUser := map[string]int{}
	for _, record := range records {
		perUser[record.Username]++
	}
	for _, user := range defaultAuthUsers {
		if got := counter.get(user); got > 2 {
			t.Fatalf("expected %s to stop after 2 attempts (lockout threshold), got %d", user, got)
		}
		if perUser[user] > 2 {
			t.Fatalf("expected at most 2 JSONL records for %s, got %d", user, perUser[user])
		}
	}
	if len(perUser) == 0 || perUser[defaultAuthUsers[0]] != 2 {
		t.Fatalf("expected exactly 2 records for the first user, got %#v", perUser)
	}
}

func TestTestAuthEnvWordlistsTakeEffect(t *testing.T) {
	_, _ = writeAuthWordlists(t, []string{"ops"}, []string{"letmein", "s3cret"})
	t.Setenv(EnvAuthLockoutThreshold, "99")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if ok && user == "ops" && password == "s3cret" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	out, err := TestAuth(context.Background(), server.URL, Options{Timeout: 20 * time.Second, MaxWorkers: 2})
	if err != nil {
		t.Fatalf("TestAuth: %v", err)
	}
	records := decodeAuthJSONL(t, out)
	if len(records) != 2 {
		t.Fatalf("expected exactly 2 attempts from env wordlists, got %#v", records)
	}
	if records[0].Username != "ops" || records[1].Username != "ops" {
		t.Fatalf("expected env user ops only, got %#v", records)
	}
	if records[0].Password != "letmein" || records[1].Password != "s3cret" {
		t.Fatalf("expected env passwords in file order, got %#v", records)
	}
	if !records[1].Success {
		t.Fatalf("expected s3cret to hit, got %#v", records)
	}
}

func TestTestAuthRejectsNonHTTPAndEmptyTargets(t *testing.T) {
	if _, err := TestAuth(context.Background(), "", Options{}); err == nil {
		t.Fatal("expected error for empty target")
	}
	if _, err := TestAuth(context.Background(), "ftp://example.com", Options{}); err == nil {
		t.Fatal("expected error for non-http target")
	}
}

func TestTestAuthContextCancellationPropagates(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	if _, err := TestAuth(ctx, server.URL, Options{Timeout: 10 * time.Second, MaxWorkers: 2}); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestTestAuthMissingWordlistFileErrors(t *testing.T) {
	t.Setenv(EnvAuthUserWords, filepath.Join(t.TempDir(), "absent-users.txt"))
	t.Setenv(EnvAuthPasswords, "")
	if _, err := TestAuth(context.Background(), "http://127.0.0.1:1/", Options{Timeout: 5 * time.Second}); err == nil {
		t.Fatal("expected error for configured but missing wordlist file")
	}
}
