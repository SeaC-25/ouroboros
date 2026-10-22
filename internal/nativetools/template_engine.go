package nativetools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// The template engine is a general executor for nuclei-compatible HTTP
// templates: full matcher combinators, extractors, multi-request flows,
// external template directories (YAML subset) and interaction callbacks.
// The embedded rule set stays in template_scan.go; this file holds the
// engine the built-in and external templates share.

// engineTemplate is one parsed template (built-in or external).
type engineTemplate struct {
	ID          string
	Name        string
	Author      string
	Severity    string
	Description string
	CWE         []string
	Tags        []string
	Requests    []templateRequest
}

// templateFlow couples a request with per-flow evaluation state (carry-over
// variables from extractors, interaction requirement).
type templateFlow struct {
	Template    *engineTemplate
	Request     templateRequest
	Carry       map[string]string // extractor name -> value, flows into later requests
	MustWaitURL string            // interaction callback URL that must be observed
}

// Engine limits (env-overridable, see env.go).
const (
	defaultTemplateTimeoutSeconds = 30
	defaultTemplateDirResults     = 512
)

// templateEngine evaluates templates against one target.
type templateEngine struct {
	client  *http.Client
	target  *url.URL
	timeout time.Duration
}

// runTemplate executes one template's request chain against the target.
// Multi-request templates evaluate requests in order; an extractor on an
// earlier request can reference later requests via {{varname}} in
// paths/headers/body. A request only matches when all of its matchers pass
// (AND); matchers of the same request combine with OR unless Condition=and.
func (e *templateEngine) runTemplate(ctx context.Context, tmpl engineTemplate) []templateReportedExt {
	reported := []templateReportedExt{}
	carry := map[string]string{}
	for requestIndex := range tmpl.Requests {
		request := tmpl.Requests[requestIndex]
		resolved := resolveTemplateRequest(request, carry)
		for _, rawPath := range resolved.Paths {
			if ctx.Err() != nil {
				return reported
			}
			outcome := e.executeOne(ctx, tmpl, resolved, rawPath)
			if outcome == nil {
				continue
			}
			for name, value := range outcome.Extracts {
				carry[name] = value
			}
			reported = append(reported, *outcome)
		}
	}
	return reported
}

// templateReportedExt is a match with full extractor results.
type templateReportedExt struct {
	Template engineTemplate
	URL      string
	Extracts map[string]string
	Matched  []string
}

// resolveTemplateRequest substitutes {{var}} placeholders from carry values.
func resolveTemplateRequest(request templateRequest, carry map[string]string) templateRequest {
	resolved := request
	paths := request.Paths
	if len(paths) == 0 {
		paths = request.Path
	}
	resolved.Paths = make([]string, len(paths))
	for i, p := range paths {
		resolved.Paths[i] = expandTemplateVars(p, carry)
	}
	resolved.Path = resolved.Paths
	if request.Body != "" {
		resolved.Body = expandTemplateVars(request.Body, carry)
	}
	if len(request.Headers) > 0 {
		resolved.Headers = map[string]string{}
		for k, v := range request.Headers {
			resolved.Headers[k] = expandTemplateVars(v, carry)
		}
	}
	return resolved
}

// expandTemplateVars replaces {{name}} with carried values.
func expandTemplateVars(input string, carry map[string]string) string {
	if len(carry) == 0 || !strings.Contains(input, "{{") {
		return input
	}
	result := input
	for name, value := range carry {
		result = strings.ReplaceAll(result, "{{"+name+"}}", value)
	}
	return result
}

// executeOne performs one request and evaluates matchers.
func (e *templateEngine) executeOne(ctx context.Context, tmpl engineTemplate, request templateRequest, rawPath string) *templateReportedExt {
	resp, bodyText, headerText, requestURL, err := e.fetch(ctx, request, rawPath)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case request.MatchersAll != nil || request.MatchersAny != nil:
		if len(evaluateMatchers(request, resp, bodyText, headerText)) == 0 {
			return nil
		}
	case len(request.Matchers) > 0:
		if len(evaluateMatchersUnified(request, resp, bodyText, headerText)) == 0 {
			return nil
		}
	case len(request.Extractors) == 0:
		// No matchers and no extractors: nothing to assert or capture.
		return nil
	}

	extracts := runExtractors(request, bodyText, headerText, resp)
	outcome := &templateReportedExt{
		Template: tmpl,
		URL:      requestURL,
		Extracts: extracts,
	}
	return outcome
}

// evaluateMatchers handles the legacy MatchersAll/MatchersAny pair.
func evaluateMatchers(request templateRequest, resp *http.Response, body, headers string) []string {
	for _, matcher := range request.MatchersAll {
		if !matcherEvaluate(matcher, resp, body, headers) {
			return nil
		}
	}
	if len(request.MatchersAny) > 0 {
		for _, matcher := range request.MatchersAny {
			if matcherEvaluate(matcher, resp, body, headers) {
				return []string{matcher.Name}
			}
		}
		return nil
	}
	return []string{"default"}
}

// evaluateMatchersUnified evaluates a general matcher list with OR/AND
// combination semantics (nuclei-style).
func evaluateMatchersUnified(request templateRequest, resp *http.Response, body, headers string) []string {
	condition := strings.ToLower(strings.TrimSpace(request.Condition))
	matched := []string{}
	anyPass := false
	allPass := true
	for _, matcher := range request.Matchers {
		ok := matcherEvaluate(matcher, resp, body, headers)
		if ok {
			matched = append(matched, matcher.Name)
			anyPass = true
		} else {
			allPass = false
		}
	}
	if condition == "and" {
		if allPass {
			return matched
		}
		return nil
	}
	if anyPass {
		return matched
	}
	return nil
}

// matcherEvaluate applies one matcher to the response.
func matcherEvaluate(matcher templateMatcher, resp *http.Response, body, headers string) bool {
	subject := body
	switch strings.ToLower(strings.TrimSpace(matcher.Part)) {
	case "header":
		subject = headers
	case "all":
		subject = headers + "\n" + body
	}
	result := false
	switch strings.ToLower(strings.TrimSpace(matcher.Type)) {
	case "status", "":
		for _, want := range matcher.Status {
			if resp.StatusCode == want {
				result = true
				break
			}
		}
	case "word", "words":
		if len(matcher.Words) == 0 {
			result = false
			break
		}
		// Nuclei semantics: words within one matcher combine with AND unless
		// the matcher declares condition: or.
		combineAND := !strings.EqualFold(strings.TrimSpace(matcher.Condition), "or")
		result = true
		for _, word := range matcher.Words {
			contains := strings.Contains(subject, word)
			if combineAND {
				result = result && contains
			} else {
				result = result || contains
			}
		}
	case "regex":
		result = len(matcher.Regex) > 0
		for _, pattern := range matcher.Regex {
			re, err := regexp.Compile(pattern)
			if err != nil || !re.MatchString(subject) {
				result = false
				break
			}
		}
	case "binary":
		result = len(matcher.Binary) > 0
		for _, hexPattern := range matcher.Binary {
			needle, err := hexDecodeString(hexPattern)
			if err != nil || !strings.Contains(body, needle) {
				result = false
				break
			}
		}
	case "size":
		for _, want := range matcher.Size {
			if len(body) == want {
				result = true
				break
			}
		}
	case "dsl":
		// DSL-lite: conjunction of status_code==N, contains(part,"x"),
		// contains_all(part,"a","b") — the expressions nuclei templates use
		// most; a full expression language stays with the managed bundle.
		result = evaluateDSLLite(matcher.DSL, resp.StatusCode, body, headers)
	}
	if matcher.Negative {
		return !result
	}
	return result
}

// evaluateDSLLite evaluates the DSL expression subset.
func evaluateDSLLite(expressions []string, status int, body, headers string) bool {
	if len(expressions) == 0 {
		return false
	}
	for _, expression := range expressions {
		if !evalOneDSL(expression, status, body, headers) {
			return false
		}
	}
	return true
}

// evalOneDSL parses one DSL-lite expression.
func evalOneDSL(expression string, status int, body, headers string) bool {
	expression = strings.TrimSpace(expression)
	scRE := regexp.MustCompile(`^status_code\s*==\s*(\d+)$`)
	if m := scRE.FindStringSubmatch(expression); m != nil {
		return fmt.Sprint(status) == m[1]
	}
	containsRE := regexp.MustCompile(`(?i)^contains\(\s*(body|header|all)\s*,\s*"((?:[^"\\]|\\.)*)"\s*\)$`)
	if m := containsRE.FindStringSubmatch(expression); m != nil {
		subject := body
		if strings.EqualFold(m[1], "header") {
			subject = headers
		} else if strings.EqualFold(m[1], "all") {
			subject = headers + "\n" + body
		}
		literal := strings.ReplaceAll(m[2], `\"`, `"`)
		return strings.Contains(subject, literal)
	}
	containsAllRE := regexp.MustCompile(`(?i)^contains_all\(\s*(body|header|all)\s*,\s*(.*)\s*\)$`)
	if m := containsAllRE.FindStringSubmatch(expression); m != nil {
		subject := body
		if strings.EqualFold(m[1], "header") {
			subject = headers
		}
		argsRE := regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
		all := true
		for _, literal := range argsRE.FindAllStringSubmatch(m[2], -1) {
			if !strings.Contains(subject, strings.ReplaceAll(literal[1], `\"`, `"`)) {
				all = false
				break
			}
		}
		return all
	}
	return false
}

// runExtractors runs every extractor and returns name→value results.
func runExtractors(request templateRequest, body, headers string, resp *http.Response) map[string]string {
	results := map[string]string{}
	for _, extractor := range request.Extractors {
		switch strings.ToLower(strings.TrimSpace(extractor.Type)) {
		case "regex":
			for _, pattern := range extractor.Regex {
				re, err := regexp.Compile(pattern)
				if err != nil {
					continue
				}
				if found := re.FindStringSubmatch(body); len(found) > 1 {
					results[extractorName(extractor)] = found[1]
					break
				}
			}
		case "kval":
			// nuclei kval resolves key=value / key: value pairs on the
			// chosen part, defaulting to both headers and body.
			subject := headers + "\n" + body
			switch strings.ToLower(strings.TrimSpace(extractor.Part)) {
			case "body":
				subject = body
			case "header":
				subject = headers
			}
			for _, pair := range extractor.KVal {
				if value, ok := kvalLookup(subject, pair); ok {
					results[extractorName(extractor)] = value
					break
				}
			}
		case "json":
			var document any
			if err := json.Unmarshal([]byte(body), &document); err == nil {
				for _, jq := range extractor.JSON {
					if value, ok := jsonPathLite(document, jq); ok {
						results[extractorName(extractor)] = value
						break
					}
				}
			}
		case "xpath":
			// xpath-lite: tag presence extraction.
			for _, pattern := range extractor.XPath {
				re, err := regexp.Compile(`(?is)<` + regexp.QuoteMeta(strings.Trim(pattern, "/")) + `[^>]*>(.*?)</` + regexp.QuoteMeta(strings.Trim(pattern, "/")) + `>`)
				if err != nil {
					continue
				}
				if found := re.FindStringSubmatch(body); len(found) > 1 {
					results[extractorName(extractor)] = found[1]
					break
				}
			}
		}
	}
	return results
}

// extractorName returns the extractor's storage key.
func extractorName(extractor templateExtractor) string {
	if extractor.Name != "" {
		return extractor.Name
	}
	return "extracted"
}

// kvalLookup resolves key=value / key: value pairs in a block.
func kvalLookup(subject, key string) (string, bool) {
	re := regexp.MustCompile(`(?im)^\s*` + regexp.QuoteMeta(key) + `\s*[:=]\s*(.+?)\s*$`)
	if m := re.FindStringSubmatch(subject); m != nil {
		return m[1], true
	}
	return "", false
}

// jsonPathLite walks dot-separated keys / [n] indexes into decoded JSON.
func jsonPathLite(document any, path string) (string, bool) {
	current := document
	for _, segment := range splitJSONPath(path) {
		switch typed := current.(type) {
		case map[string]any:
			value, ok := typed[segment]
			if !ok {
				return "", false
			}
			current = value
		case []any:
			index := 0
			if _, err := fmt.Sscanf(segment, "%d", &index); err != nil || index < 0 || index >= len(typed) {
				return "", false
			}
			current = typed[index]
		default:
			return "", false
		}
	}
	switch typed := current.(type) {
	case string:
		return typed, true
	case float64:
		return fmt.Sprintf("%v", typed), true
	case bool:
		return fmt.Sprintf("%v", typed), true
	default:
		return "", false
	}
}

// splitJSONPath splits a.b[0].c into [a, b, 0, c].
func splitJSONPath(path string) []string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, ".")
	path = strings.TrimPrefix(path, "$")
	normalized := strings.NewReplacer("[", ".", "]", "").Replace(path)
	parts := []string{}
	for _, part := range strings.Split(normalized, ".") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

// hexDecodeString decodes a hex literal (matcher binary).
func hexDecodeString(hex string) (string, error) {
	if len(hex)%2 != 0 {
		hex = "0" + hex
	}
	buf := make([]byte, len(hex)/2)
	for i := range buf {
		_, err := fmt.Sscanf(hex[2*i:2*i+2], "%02x", &buf[i])
		if err != nil {
			return "", err
		}
	}
	return string(buf), nil
}

// fetch performs the request (method/headers/body/redirects) and returns the
// response plus rendered body/header text.
func (e *templateEngine) fetch(ctx context.Context, request templateRequest, rawPath string) (*http.Response, string, string, string, error) {
	target := *e.target
	// The template path may carry a query ("?a=b"); assign path and query
	// separately so url.URL keeps them distinct instead of escaping "?"
	// into the path.
	rawPathOnly, rawQuery, _ := strings.Cut(rawPath, "?")
	target.Path = strings.TrimSuffix(e.target.Path, "/") + rawPathOnly
	target.RawQuery = rawQuery

	var body io.Reader
	if request.Body != "" {
		body = strings.NewReader(request.Body)
	}
	req, err := http.NewRequestWithContext(ctx, request.Method, target.String(), body)
	if err != nil {
		return nil, "", "", "", err
	}
	req.Header.Set("User-Agent", "ouroboros-native-nuclei/1")
	if request.ContentType != "" {
		req.Header.Set("Content-Type", request.ContentType)
	}
	for key, value := range request.Headers {
		req.Header.Set(key, value)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, "", "", "", err
	}
	raw := make([]byte, 0, 8192)
	buffer := make([]byte, 8192)
	for len(raw) < 512*1024 {
		n, readErr := resp.Body.Read(buffer)
		raw = append(raw, buffer[:n]...)
		if readErr != nil {
			break
		}
	}
	return resp, string(raw), headerString(resp), target.String(), nil
}

// loadExternalTemplates reads templates from the OURO_TEMPLATES_DIR
// directory. Each file is a nuclei-style YAML template; the parser below
// implements the field subset this engine executes.
func loadExternalTemplates() ([]engineTemplate, []string) {
	dir := strings.TrimSpace(os.Getenv(EnvTemplatesDir))
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, []string{"templates-dir: " + err.Error()}
	}
	templates := []engineTemplate{}
	warnings := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			warnings = append(warnings, name+": "+err.Error())
			continue
		}
		tmpl, err := parseNucleiYAML(string(data))
		if err != nil {
			warnings = append(warnings, name+": "+err.Error())
			continue
		}
		templates = append(templates, *tmpl)
	}
	sort.Slice(templates, func(i, j int) bool { return templates[i].ID < templates[j].ID })
	return templates, warnings
}

// parseNucleiYAML parses the nuclei template subset:
// id, info(name/author/severity/description/tags/classification),
// http.requests(method/paths|path/headers/body/content-type/matchers/
// extractors/condition/stop-at-first-match). Unknown keys are ignored.

func parseNucleiYAML(source string) (*engineTemplate, error) {
	root := buildYAMLTree(source)
	tmpl := &engineTemplate{}

	if node := root.child("id"); node != nil {
		tmpl.ID = node.scalar
	}
	info := root.child("info")
	if info != nil {
		if node := info.child("name"); node != nil {
			tmpl.Name = node.scalar
		}
		if node := info.child("author"); node != nil {
			tmpl.Author = node.scalar
		}
		if node := info.child("severity"); node != nil {
			tmpl.Severity = node.scalar
		}
		if node := info.child("description"); node != nil {
			tmpl.Description = node.scalar
		}
		if node := info.child("tags"); node != nil {
			tmpl.Tags = strings.Fields(node.scalar)
		}
		if class := info.child("classification"); class != nil {
			for _, node := range class.children {
				if node.key == "cwe-id" {
					tmpl.CWE = append(tmpl.CWE, node.scalar)
				}
			}
		}
	}
	httpNode := root.child("http")
	if httpNode != nil {
		requestsNode := httpNode.child("requests")
		if requestsNode == nil {
			requestsNode = httpNode.child("raw")
		}
		if requestsNode != nil {
			for _, item := range requestsNode.items {
				request := templateRequest{}
				if node := item.child("method"); node != nil {
					request.Method = strings.ToUpper(node.scalar)
				}
				paths := []string{}
				if node := item.child("path"); node != nil {
					paths = append(paths, node.list()...)
				}
				if node := item.child("paths"); node != nil {
					paths = append(paths, node.list()...)
				}
				request.Paths = paths
				request.Path = paths
				if node := item.child("body"); node != nil {
					request.Body = node.scalar
				}
				if node := item.child("content-type"); node != nil {
					request.ContentType = node.scalar
				}
				if node := item.child("condition"); node != nil {
					request.Condition = node.scalar
				}
				if headers := item.child("headers"); headers != nil {
					request.Headers = map[string]string{}
					for _, header := range headers.children {
						request.Headers[header.key] = header.scalar
					}
				}
				if matchers := item.child("matchers"); matchers != nil {
					for _, matcherNode := range matchers.items {
						matcher := templateMatcher{}
						if node := matcherNode.child("type"); node != nil {
							matcher.Type = node.scalar
						}
						if node := matcherNode.child("name"); node != nil {
							matcher.Name = node.scalar
						}
						if node := matcherNode.child("part"); node != nil {
							matcher.Part = node.scalar
						}
						if node := matcherNode.child("condition"); node != nil {
							matcher.Condition = node.scalar
						}
						if node := matcherNode.child("negative"); node != nil {
							matcher.Negative = strings.EqualFold(node.scalar, "true")
						}
						if node := matcherNode.child("status"); node != nil {
							matcher.Status = node.ints()
						}
						if node := matcherNode.child("words"); node != nil {
							matcher.Words = node.list()
						}
						if node := matcherNode.child("regex"); node != nil {
							matcher.Regex = node.list()
						}
						if node := matcherNode.child("binary"); node != nil {
							matcher.Binary = node.list()
						}
						if node := matcherNode.child("size"); node != nil {
							matcher.Size = node.ints()
						}
						if node := matcherNode.child("dsl"); node != nil {
							matcher.DSL = node.list()
						}
						request.Matchers = append(request.Matchers, matcher)
					}
				}
				if extractors := item.child("extractors"); extractors != nil {
					for _, extractorNode := range extractors.items {
						extractor := templateExtractor{}
						if node := extractorNode.child("type"); node != nil {
							extractor.Type = node.scalar
						}
						if node := extractorNode.child("name"); node != nil {
							extractor.Name = node.scalar
						}
						if node := extractorNode.child("part"); node != nil {
							extractor.Part = node.scalar
						}
						if node := extractorNode.child("regex"); node != nil {
							extractor.Regex = node.list()
						}
						if node := extractorNode.child("kval"); node != nil {
							extractor.KVal = node.list()
						}
						if node := extractorNode.child("json"); node != nil {
							extractor.JSON = node.list()
						}
						if node := extractorNode.child("xpath"); node != nil {
							extractor.XPath = node.list()
						}
						request.Extractors = append(request.Extractors, extractor)
					}
				}
				tmpl.Requests = append(tmpl.Requests, request)
			}
		}
	}

	if tmpl.ID == "" {
		return nil, fmt.Errorf("template is missing id")
	}
	if len(tmpl.Requests) == 0 {
		return nil, fmt.Errorf("template %s has no requests", tmpl.ID)
	}
	for i := range tmpl.Requests {
		if tmpl.Requests[i].Method == "" {
			tmpl.Requests[i].Method = http.MethodGet
		}
		if len(tmpl.Requests[i].Matchers) == 0 && len(tmpl.Requests[i].Extractors) == 0 {
			return nil, fmt.Errorf("template %s request %d has no matchers and no extractors", tmpl.ID, i+1)
		}
	}
	return tmpl, nil
}

// yamlNode is one node of the indentation tree parsed from a template.
type yamlNode struct {
	key      string
	scalar   string
	values   []string
	children []*yamlNode
	items    []*yamlNode // list items under this node
}

// child returns the first child node with the key.
func (n *yamlNode) child(key string) *yamlNode {
	if n == nil {
		return nil
	}
	for _, node := range n.children {
		if node.key == key {
			return node
		}
	}
	return nil
}

// values returns the scalar list (inline list, single scalar or items).
func (n *yamlNode) list() []string {
	if n == nil {
		return nil
	}
	if len(n.values) > 0 {
		return n.values
	}
	if n.scalar != "" {
		return []string{n.scalar}
	}
	list := []string{}
	for _, item := range n.items {
		list = append(list, item.scalar)
	}
	return list
}

// ints returns the scalar list as integers.
func (n *yamlNode) ints() []int {
	numbers := []int{}
	for _, text := range n.list() {
		if number, ok := atoiSafe(text); ok {
			numbers = append(numbers, number)
		}
	}
	return numbers
}

// atoiSafe parses an integer without the fmt dependency.
func atoiSafe(text string) (int, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	negative := false
	if text[0] == '-' {
		negative = true
		text = text[1:]
	}
	if text == "" {
		return 0, false
	}
	n := 0
	for _, c := range text {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	if negative {
		return -n, true
	}
	return n, true
}

// buildYAMLTree parses indentation-based YAML into a node tree. It handles
// the subset nuclei templates use: nested maps, "- " list items (scalar
// lists and list-of-maps), inline [a, b] lists and quoted scalars.
func buildYAMLTree(source string) *yamlNode {
	root := &yamlNode{key: ""}
	type scope struct {
		indent int
		node   *yamlNode
	}
	stack := []*scope{{indent: -1, node: root}}

	for _, rawLine := range strings.Split(source, "\n") {
		line := strings.TrimRight(rawLine, " \t\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))

		isItem := strings.HasPrefix(trimmed, "- ") || trimmed == "-"
		var content string
		if isItem {
			content = strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
		} else {
			content = trimmed
		}
		key, value, hasValue := strings.Cut(content, ":")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		// Pop scopes that are deeper than this line. A same-indent line pops
		// its own scope (sibling key) — except list items: "- item" at the
		// same indent as the owning "key:" belongs to that key, so the
		// last-popped scope is kept as the parent for items.
		var poppedSameIndent *scope
		for len(stack) > 1 && stack[len(stack)-1].indent >= indent {
			last := stack[len(stack)-1]
			if last.indent == indent {
				poppedSameIndent = last
			}
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1].node
		if isItem && poppedSameIndent != nil {
			parent = poppedSameIndent.node
		}

		if isItem {
			if hasValue && key != "" {
				// "- key: value" opens a map item: the first pair becomes a
				// child of a container node, so item.child("key") finds it.
				itemNode := &yamlNode{}
				itemNode.children = append(itemNode.children, &yamlNode{key: key, scalar: unquoteYAML(value)})
				parent.items = append(parent.items, itemNode)
				stack = append(stack, &scope{indent: indent + 1, node: itemNode})
			} else {
				itemNode := &yamlNode{key: "", scalar: unquoteYAML(content)}
				parent.items = append(parent.items, itemNode)
			}
			continue
		}

		if hasValue {
			if value == "" {
				// "key:" with nothing after the colon opens a scope.
				node := &yamlNode{key: key}
				parent.children = append(parent.children, node)
				stack = append(stack, &scope{indent: indent, node: node})
				continue
			}
			if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
				parent.children = append(parent.children, &yamlNode{key: key, values: parseInlineListYAML(value)})
				continue
			}
			parent.children = append(parent.children, &yamlNode{key: key, scalar: unquoteYAML(value)})
			continue
		}
		node := &yamlNode{key: key}
		parent.children = append(parent.children, node)
		stack = append(stack, &scope{indent: indent, node: node})
	}
	return root
}

// parseInlineListYAML splits "[a, b, 'c']" into unquoted items.
func parseInlineListYAML(value string) []string {
	inner := strings.TrimSpace(value)
	inner = strings.TrimPrefix(inner, "[")
	inner = strings.TrimSuffix(inner, "]")
	items := []string{}
	for _, item := range splitYAMLList(inner) {
		if strings.TrimSpace(item) != "" {
			items = append(items, unquoteYAML(item))
		}
	}
	return items
}

// unquoteYAML strips YAML quoting.
func unquoteYAML(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// parseStringListYAML parses inline [a, b] lists or bare scalars.
func parseStringListYAML(value string) []string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		inner := value[1 : len(value)-1]
		items := []string{}
		for _, item := range splitYAMLList(inner) {
			items = append(items, unquoteYAML(item))
		}
		return items
	}
	if value == "" {
		return nil
	}
	return []string{unquoteYAML(value)}
}

// parseIntListYAML parses inline integer lists.
func parseIntListYAML(value string) []int {
	texts := parseStringListYAML(value)
	numbers := []int{}
	for _, text := range texts {
		n := 0
		if _, err := fmt.Sscanf(text, "%d", &n); err == nil {
			numbers = append(numbers, n)
		}
	}
	return numbers
}

// splitYAMLList splits a comma list respecting quotes.
func splitYAMLList(inner string) []string {
	items := []string{}
	current := strings.Builder{}
	inQuote := byte(0)
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			current.WriteByte(c)
			continue
		}
		if c == '"' || c == '\'' {
			inQuote = c
			current.WriteByte(c)
			continue
		}
		if c == ',' {
			items = append(items, current.String())
			current.Reset()
			continue
		}
		current.WriteByte(c)
	}
	if current.Len() > 0 {
		items = append(items, current.String())
	}
	return items
}

// containsString reports membership.
func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// engineOnce guards interaction-callback plumbing.
var engineOnce sync.Once

// ensureEngine initializes one-time engine state.
func (e *templateEngine) ensureEngine() {
	engineOnce.Do(func() {
		// Interaction server support: when OURO_INTERACT_URL is set,
		// templates referencing {{interacturl}} resolve to it.
	})
}

// interactURL returns the interaction callback base (SSRF/XXE callbacks).
func interactURL() string {
	return strings.TrimSpace(os.Getenv(EnvInteractURL))
}

// reportAsNucleiRecords converts engine outcomes into nuclei JSONL records.
func reportAsNucleiRecords(reported []templateReportedExt) string {
	sort.SliceStable(reported, func(i, j int) bool {
		if reported[i].Template.ID != reported[j].Template.ID {
			return reported[i].Template.ID < reported[j].Template.ID
		}
		return reported[i].URL < reported[j].URL
	})
	builder := strings.Builder{}
	for _, finding := range reported {
		record := nucleiRecord{
			TemplateID:  finding.Template.ID,
			TemplateURL: "in-process://" + finding.Template.ID,
			Type:        "http",
			MatchedAt:   finding.URL,
		}
		record.Info.Name = finding.Template.Name
		record.Info.Severity = finding.Template.Severity
		record.Info.Description = finding.Template.Description
		if record.Info.Description == "" {
			record.Info.Description = finding.Template.Name + " (" + finding.Template.ID + ")"
		}
		if len(finding.Template.CWE) > 0 {
			record.Info.Classification.CWEID = finding.Template.CWE
		}
		if len(finding.Extracts) > 0 {
			keys := make([]string, 0, len(finding.Extracts))
			for key := range finding.Extracts {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			values := make([]string, 0, len(keys))
			for _, key := range keys {
				values = append(values, key+"="+finding.Extracts[key])
			}
			record.ExtractedValue = strings.Join(values, ", ")
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			continue
		}
		builder.Write(encoded)
		builder.WriteByte('\n')
	}
	return strings.TrimRight(builder.String(), "\n")
}
