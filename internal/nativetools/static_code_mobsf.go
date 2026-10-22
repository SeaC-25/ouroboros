package nativetools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// mobsfscanOutput mirrors the mobsfscan JSON shape the parser reads
// (internal/app/mobsfscan_parser.go: results{rule:{metadata,files[]}}).
type mobsfscanOutput struct {
	Results map[string]mobsfscanResult `json:"results"`
}

type mobsfscanResult struct {
	Metadata mobsfscanMetadata `json:"metadata"`
	Files    []mobsfscanFile   `json:"files"`
}

type mobsfscanMetadata struct {
	Description string `json:"description"`
	Severity    string `json:"severity"`
	CWE         string `json:"cwe"`
	OWASP       string `json:"owasp-mobile"`
	MASVS       string `json:"masvs"`
}

type mobsfscanFile struct {
	FilePath    string `json:"file_path"`
	MatchLines  []int  `json:"match_lines"`
	LineNumber  int    `json:"line_number"`
	StartLine   int    `json:"start_line"`
	MatchString string `json:"match_string"`
}

// mobsfscanRule is one embedded rule of the native subset.
type mobsfscanRule struct {
	ID          string
	Severity    string
	Description string
	CWE         string
	OWASP       string
	MASVS       string
	Pattern     *regexp.Regexp
}

// mobsfscanRules covers the high-signal mobile-specific weaknesses the
// mobsfscan rule set flags; the managed mobsfscan bundle remains the
// reference for the full corpus.
var mobsfscanRules = []mobsfscanRule{
	{
		ID:          "hardcoded_api_key",
		Severity:    "HIGH",
		Description: "Hardcoded API key or secret in mobile source",
		CWE:         "CWE-798",
		OWASP:       "M2: Privacy Violation",
		MASVS:       "MASVS-STORAGE-1",
		Pattern:     regexp.MustCompile(`\b(AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z\-_]{35}|ghp_[0-9A-Za-z]{36}|sk-[0-9A-Za-z]{20,})\b`),
	},
	{
		ID:          "insecure_random",
		Severity:    "MEDIUM",
		Description: "Insecure random number generator used for security purpose",
		CWE:         "CWE-330",
		OWASP:       "M5: Insufficient Cryptography",
		MASVS:       "MASVS-CRYPTO-1",
		Pattern:     regexp.MustCompile(`(?i)\bjava\.util\.Random\b|(?i)\bnew Random\(\)|\brandom\.random\(\)`),
	},
	{
		ID:          "ssl_bypass",
		Severity:    "HIGH",
		Description: "SSL/TLS certificate validation bypassed",
		CWE:         "CWE-295",
		OWASP:       "M3: Insecure Communication",
		MASVS:       "MASVS-NETWORK-1",
		Pattern:     regexp.MustCompile(`(?i)checkServerTrusted|setHostnameVerifier\(\s*ALLOW_ALL|TrustAllCerts|InsecureSkipVerify:\s*true|verify\s*=\s*False`),
	},
	{
		ID:          "webview_js_enabled",
		Severity:    "MEDIUM",
		Description: "WebView JavaScript enabled without safe browsing",
		CWE:         "CWE-79",
		OWASP:       "M7: Poor Code Quality",
		MASVS:       "MASVS-PLATFORM-5",
		Pattern:     regexp.MustCompile(`(?i)setJavaScriptEnabled\(true\)`),
	},
	{
		ID:          "webview_file_access",
		Severity:    "HIGH",
		Description: "WebView allows file system access",
		CWE:         "CWE-200",
		OWASP:       "M1: Improper Credential Usage",
		MASVS:       "MASVS-PLATFORM-5",
		Pattern:     regexp.MustCompile(`(?i)setAllowFileAccess\(true\)|setAllowUniversalAccessFromFileURLs\(true\)`),
	},
	{
		ID:          "debuggable",
		Severity:    "HIGH",
		Description: "Application marked debuggable",
		CWE:         "CWE-489",
		OWASP:       "M7: Poor Code Quality",
		MASVS:       "MASVS-PLATFORM-2",
		Pattern:     regexp.MustCompile(`(?i)android:debuggable\s*=\s*"true"`),
	},
	{
		ID:          "allow_backup",
		Severity:    "MEDIUM",
		Description: "Application allows backup of app data",
		CWE:         "CWE-530",
		OWASP:       "M2: Privacy Violation",
		MASVS:       "MASVS-STORAGE-6",
		Pattern:     regexp.MustCompile(`(?i)android:allowBackup\s*=\s*"true"`),
	},
	{
		ID:          "hardcoded_password",
		Severity:    "HIGH",
		Description: "Hardcoded password or secret assignment",
		CWE:         "CWE-259",
		OWASP:       "M2: Privacy Violation",
		MASVS:       "MASVS-STORAGE-1",
		Pattern:     regexp.MustCompile(`(?i)\b(password|passwd|secret|api_key|apikey|token)\b\s*[:=]\s*"[^"]{4,}"`),
	},
}

// ScanStaticCodeMobsf runs the embedded mobile rule subset over an artifact
// directory or file and emits mobsfscan-compatible JSON.
func ScanStaticCodeMobsf(ctx context.Context, target string, _ Options) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("static_code_scan requires an artifact path")
	}
	paths, err := collectScanPaths(target)
	if err != nil {
		return "", err
	}

	output := mobsfscanOutput{Results: map[string]mobsfscanResult{}}
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
		for _, rule := range mobsfscanRules {
			locations := rule.Pattern.FindAllIndex(data, -1)
			if len(locations) == 0 {
				continue
			}
			result := output.Results[rule.ID]
			result.Metadata = mobsfscanMetadata{
				Description: rule.Description,
				Severity:    rule.Severity,
				CWE:         rule.CWE,
				OWASP:       rule.OWASP,
				MASVS:       rule.MASVS,
			}
			for _, location := range locations {
				line := 1 + countByte(data[:location[0]], '\n')
				text := string(data[location[0]:location[1]])
				if len(text) > 120 {
					text = text[:120]
				}
				result.Files = append(result.Files, mobsfscanFile{
					FilePath:    path,
					StartLine:   line,
					MatchLines:  []int{line},
					LineNumber:  line,
					MatchString: text,
				})
			}
			output.Results[rule.ID] = result
		}
	}

	encoded, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("encode mobsfscan output: %w", err)
	}
	return string(encoded), nil
}

// countByte counts occurrences of b in data.
func countByte(data []byte, b byte) int {
	n := 0
	for _, c := range data {
		if c == b {
			n++
		}
	}
	return n
}
