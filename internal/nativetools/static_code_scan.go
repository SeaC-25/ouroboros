package nativetools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// staticCodeFinding is one rule hit on one source line.
type staticCodeFinding struct {
	RuleID    string
	Level     string
	Message   string
	URI       string
	StartLine int
}

// staticCodeRule is one embedded semgrep-style rule of the native subset.
type staticCodeRule struct {
	ID       string
	Severity string // semgrep severity: ERROR / WARNING / INFO
	Message  string
	CWE      string
	Pattern  *regexp.Regexp
}

// staticCodeRules covers the most common source-level leaks and weaknesses
// across languages; the managed semgrep bundle remains the reference for the
// full AST-accurate corpus.
var staticCodeRules = []staticCodeRule{
	{
		ID:       "aws-access-key-literal",
		Severity: "ERROR",
		Message:  "Hardcoded AWS access key",
		CWE:      "CWE-798",
		Pattern:  regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	},
	{
		ID:       "github-token-literal",
		Severity: "ERROR",
		Message:  "Hardcoded GitHub token",
		CWE:      "CWE-798",
		Pattern:  regexp.MustCompile(`\bghp_[0-9A-Za-z]{36}\b`),
	},
	{
		ID:       "generic-api-key-literal",
		Severity: "ERROR",
		Message:  "Hardcoded API key or service token",
		CWE:      "CWE-798",
		Pattern:  regexp.MustCompile(`\b(sk-[0-9A-Za-z]{20,}|AIza[0-9A-Za-z\-_]{35}|xox[baprs]-[0-9A-Za-z-]{10,})\b`),
	},
	{
		ID:       "hardcoded-password",
		Severity: "ERROR",
		Message:  "Hardcoded password assignment",
		CWE:      "CWE-259",
		Pattern:  regexp.MustCompile(`(?i)\b(password|passwd|pwd)\b\s*[:=]\s*"[^"]{4,}"`),
	},
	{
		ID:       "embedded-private-key",
		Severity: "ERROR",
		Message:  "Embedded private key material",
		CWE:      "CWE-321",
		Pattern:  regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY-----`),
	},
	{
		ID:       "eval-base64-decode",
		Severity: "ERROR",
		Message:  "Dynamic code evaluation over decoded payload",
		CWE:      "CWE-95",
		Pattern:  regexp.MustCompile(`(?i)\beval\s*\(\s*(?:base64_decode|b64decode|atob|base64\.b64decode|base64\.decode)`),
	},
	{
		ID:       "os-system-invocation",
		Severity: "WARNING",
		Message:  "OS command constructed from source code",
		CWE:      "CWE-78",
		Pattern:  regexp.MustCompile(`\bos\.system\s*\(|\bexec\s*\(\s*["'](?:sh|bash|cmd|powershell)\b`),
	},
	{
		ID:       "sql-string-concatenation",
		Severity: "WARNING",
		Message:  "SQL query built with string concatenation",
		CWE:      "CWE-89",
		Pattern:  regexp.MustCompile(`(?i)(?:SELECT|INSERT|UPDATE|DELETE)\s.{0,80}(?:FROM|INTO|SET|WHERE)\s.{0,40}["']?\s*(?:\+\s*\w|\.\s*\w|%s|"` + "`" + `\s*\+|\$\{)`),
	},
	{
		ID:       "insecure-random",
		Severity: "WARNING",
		Message:  "Insecure random generator in security-sensitive code",
		CWE:      "CWE-330",
		Pattern:  regexp.MustCompile(`(?i)\brand\.Intn?\s*\(|\brandom\.random\s*\(|\bMath\.random\s*\(\s*\)`),
	},
	{
		ID:       "tls-verification-disabled",
		Severity: "ERROR",
		Message:  "TLS certificate verification disabled",
		CWE:      "CWE-295",
		Pattern:  regexp.MustCompile(`(?i)verify\s*=\s*False|InsecureSkipVerify\s*:\s*true`),
	},
}

// sarifLog is the SARIF 2.1.0 subset the semgrep parser consumes
// (internal/app/semgrep_parser.go).
type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool        sarifTool         `json:"tool"`
	Results     []sarifResult     `json:"results"`
	Invocations []sarifInvocation `json:"invocations,omitempty"`
}

type sarifInvocation struct {
	ExecutionSuccessful bool `json:"executionSuccessful"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	Rules          []sarifRule `json:"rules"`
	InformationURI string      `json:"informationUri,omitempty"`
}

type sarifRule struct {
	ID               string          `json:"id"`
	ShortDescription sarifMessage    `json:"shortDescription"`
	FullDescription  sarifMessage    `json:"fullDescription"`
	Properties       sarifProperties `json:"properties"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifProperties struct {
	Precision string `json:"precision"`
	CWE       string `json:"cwe,omitempty"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
}

// sarifLevels maps semgrep severity to SARIF level.
var sarifLevels = map[string]string{
	"ERROR":   "error",
	"WARNING": "warning",
	"INFO":    "note",
}

// ScanStaticCodeSemgrep runs the embedded rule subset over an artifact
// directory or file and emits a SARIF 2.1.0 subset compatible with semgrep
// scan --sarif output.
func ScanStaticCodeSemgrep(ctx context.Context, target string, _ Options) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("static_code_scan requires an artifact path")
	}
	paths, err := collectScanPaths(target)
	if err != nil {
		return "", err
	}

	results := []sarifResult{}
	for _, path := range paths {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		info, err := os.Stat(path)
		if err != nil || info.Size() > maxScanFileSize {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil || isBinaryContent(data) {
			continue
		}
		for _, finding := range scanStaticCodeFile(path, data) {
			uri, err := filepath.Rel(target, path)
			if err != nil {
				uri = filepath.ToSlash(path)
			}
			uri = filepath.ToSlash(uri)
			results = append(results, sarifResult{
				RuleID:  finding.RuleID,
				Level:   finding.Level,
				Message: sarifMessage{Text: finding.Message},
				Locations: []sarifLocation{{
					PhysicalLocation: sarifPhysicalLocation{
						ArtifactLocation: sarifArtifactLocation{URI: uri},
						Region:           sarifRegion{StartLine: finding.StartLine},
					},
				}},
			})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Locations[0].PhysicalLocation.ArtifactLocation.URI != results[j].Locations[0].PhysicalLocation.ArtifactLocation.URI {
			return results[i].Locations[0].PhysicalLocation.ArtifactLocation.URI < results[j].Locations[0].PhysicalLocation.ArtifactLocation.URI
		}
		return results[i].Locations[0].PhysicalLocation.Region.StartLine < results[j].Locations[0].PhysicalLocation.Region.StartLine
	})

	log := sarifLog{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "Semgrep",
				InformationURI: "https://semgrep.dev",
				Rules:          sarifRulesFor(results),
			}},
			Results: results,
			Invocations: []sarifInvocation{{
				ExecutionSuccessful: true,
			}},
		}},
	}
	encoded, err := json.Marshal(log)
	if err != nil {
		return "", fmt.Errorf("encode semgrep SARIF output: %w", err)
	}
	return string(encoded), nil
}

// scanStaticCodeFile applies every rule to one file and returns its findings.
func scanStaticCodeFile(path string, data []byte) []staticCodeFinding {
	findings := []staticCodeFinding{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		for _, rule := range staticCodeRules {
			if rule.Pattern.MatchString(line) {
				findings = append(findings, staticCodeFinding{
					RuleID:    rule.ID,
					Level:     sarifLevels[rule.Severity],
					Message:   rule.Message,
					StartLine: lineNumber,
				})
			}
		}
	}
	return findings
}

// sarifRulesFor returns the deduplicated rule descriptors referenced by the
// emitted results.
func sarifRulesFor(results []sarifResult) []sarifRule {
	used := map[string]bool{}
	rules := []sarifRule{}
	for _, result := range results {
		if used[result.RuleID] {
			continue
		}
		used[result.RuleID] = true
		for _, rule := range staticCodeRules {
			if rule.ID != result.RuleID {
				continue
			}
			rules = append(rules, sarifRule{
				ID:               rule.ID,
				ShortDescription: sarifMessage{Text: rule.Message},
				FullDescription:  sarifMessage{Text: rule.Message},
				Properties: sarifProperties{
					Precision: "medium",
					CWE:       rule.CWE,
				},
			})
		}
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	return rules
}
