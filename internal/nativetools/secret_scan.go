package nativetools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// OpSecretScan is the gitleaks-equivalent operation.
const OpSecretScan = "secret_scan"

// maxScanFileSize skips large artifacts so a directory scan stays bounded.
const maxScanFileSize = 2 << 20

// Leak is one gitleaks-json record.
type Leak struct {
	RuleID    string `json:"RuleID"`
	File      string `json:"File"`
	StartLine int    `json:"StartLine"`
	Secret    string `json:"Secret"`
	Match     string `json:"Match"`
}

// secretRule is one built-in detector.
type secretRule struct {
	id      string
	pattern *regexp.Regexp
	// entropyOnly marks generic rules that additionally require high entropy
	// on the captured secret to suppress placeholder matches.
	entropyOnly bool
	minEntropy  float64
	// placeholderProne marks generic rules whose matches are commonly
	// documentation placeholders; only these consult isPlaceholder.
	placeholderProne bool
}

// builtInSecretRules is the embedded rule set. It intentionally mirrors the
// most common gitleaks rules rather than the full upstream catalogue; the
// managed gitleaks bundle remains available for complete coverage.
var builtInSecretRules = []secretRule{
	{id: "aws-access-token", pattern: regexp.MustCompile(`\b(AKIA[0-9A-Z]{16})\b`)},
	{id: "google-api-key", pattern: regexp.MustCompile(`\b(AIza[0-9A-Za-z\-_]{35})\b`)},
	{id: "github-pat", pattern: regexp.MustCompile(`\b(ghp_[0-9A-Za-z]{36})\b`)},
	{id: "github-oauth", pattern: regexp.MustCompile(`\b(gho_[0-9A-Za-z]{36})\b`)},
	{id: "github-app-token", pattern: regexp.MustCompile(`\b(ghu_[0-9A-Za-z]{36})\b`)},
	{id: "github-refresh-token", pattern: regexp.MustCompile(`\b(ghr_[0-9A-Za-z]{36})\b`)},
	{id: "github-fine-grained-pat", pattern: regexp.MustCompile(`\b(github_pat_[0-9A-Za-z_]{22,})\b`)},
	{id: "slack-token", pattern: regexp.MustCompile(`\b(xox[baprs]-[0-9A-Za-z-]{10,})\b`)},
	{id: "stripe-secret-key", pattern: regexp.MustCompile(`\b(sk_live_[0-9A-Za-z]{16,})\b`)},
	{id: "openai-api-key", pattern: regexp.MustCompile(`\b(sk-[A-Za-z0-9]{20,})\b`)},
	{id: "private-key", pattern: regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY-----`)},
	{id: "generic-api-key", pattern: regexp.MustCompile(`(?i)(?:api[_-]?key|apikey|secret|passwd|password|token)["'\s:=]{1,4}([0-9A-Za-z\-_/+]{16,})`), entropyOnly: true, minEntropy: 3.0, placeholderProne: true},
}

// ScanSecrets walks the target (file or directory) and returns
// gitleaks-compatible JSON covering every rule hit.
func ScanSecrets(ctx context.Context, target string, _ Options) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("secret_scan requires a file or directory path")
	}

	paths, err := collectScanPaths(target)
	if err != nil {
		return "", err
	}

	leaks := []Leak{}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		fileLeaks, err := scanFile(path)
		if err != nil {
			continue // unreadable files are skipped, like gitleaks' best-effort scan
		}
		leaks = append(leaks, fileLeaks...)
	}
	sort.Slice(leaks, func(i, j int) bool {
		if leaks[i].File != leaks[j].File {
			return leaks[i].File < leaks[j].File
		}
		return leaks[i].StartLine < leaks[j].StartLine
	})

	encoded, err := json.Marshal(leaks)
	if err != nil {
		return "", fmt.Errorf("encode secret scan results: %w", err)
	}
	return string(encoded), nil
}

func collectScanPaths(target string) ([]string, error) {
	info, err := os.Stat(target)
	if err != nil {
		return nil, fmt.Errorf("stat secret scan target: %w", err)
	}
	if !info.IsDir() {
		return []string{target}, nil
	}
	paths := []string{}
	err = filepath.WalkDir(target, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := entry.Name()
		if entry.IsDir() {
			if name == ".git" || name == "node_modules" || name == "dist" || name == "vendor" {
				return fs.SkipDir
			}
			return nil
		}
		if info, err := entry.Info(); err == nil && info.Size() > maxScanFileSize {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk secret scan target: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

func scanFile(path string) ([]Leak, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if isBinaryContent(data) {
		return nil, nil
	}

	leaks := []Leak{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		for _, rule := range builtInSecretRules {
			match := rule.pattern.FindStringSubmatch(line)
			if match == nil {
				continue
			}
			secret := match[len(match)-1]
			if rule.entropyOnly && shannonEntropy(secret) < rule.minEntropy {
				continue
			}
			if rule.placeholderProne && isPlaceholder(secret) {
				continue
			}
			leaks = append(leaks, Leak{
				RuleID:    rule.id,
				File:      path,
				StartLine: lineNumber,
				Secret:    secret,
				Match:     strings.TrimSpace(line),
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return leaks, nil
}

// isBinaryContent sniffs for NUL bytes, which text scanners skip.
func isBinaryContent(data []byte) bool {
	limit := len(data)
	if limit > 8000 {
		limit = 8000
	}
	for i := 0; i < limit; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

// isPlaceholder filters obvious documentation values.
func isPlaceholder(secret string) bool {
	lower := strings.ToLower(secret)
	for _, marker := range []string{"example", "placeholder", "changeme", "your_", "xxxx", "dummy", "redacted"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// shannonEntropy returns the per-character entropy of s in bits.
func shannonEntropy(s string) float64 {
	if s == "" {
		return 0
	}
	counts := make(map[rune]int, len(s))
	for _, r := range s {
		counts[r]++
	}
	entropy := 0.0
	total := float64(len(s))
	for _, count := range counts {
		p := float64(count) / total
		entropy -= p * math.Log2(p)
	}
	return entropy
}
