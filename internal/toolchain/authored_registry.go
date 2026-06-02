package toolchain

import (
	"sort"
	"strings"
)

type AuthoredToolRegistry struct {
	adapters map[string]AuthoredToolAdapterDefinition
	parsers  map[string]AuthoredToolParserDefinition
}

type AuthoredToolAdapterDefinition struct {
	ID          string
	Description string
	Operations  []AuthoredToolOperationDefinition
}

type AuthoredToolOperationDefinition struct {
	ID               string
	Description      string
	RequiredInputs   []AuthoredToolInputContract
	SupportedParsers []string
	DependencyNames  []string
}

type AuthoredToolInputContract struct {
	Name  string
	Types []string
}

type AuthoredToolParserDefinition struct {
	ID          string
	Format      string
	Description string
}

func DefaultAuthoredToolRegistry() AuthoredToolRegistry {
	adapters := []AuthoredToolAdapterDefinition{
		{
			ID:          "managed_binary",
			Description: "Execute a workspace-managed binary through Ouroboros scope, isolation, and authorization gates.",
			Operations: []AuthoredToolOperationDefinition{
				{
					ID:               "http_probe",
					Description:      "Probe HTTP(S) targets and normalize response observations.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "target_url", Types: []string{"url"}}},
					SupportedParsers: []string{"httpx_jsonl", "web_api_parser", "web_login_state_parser", "route_observation_parser", "auth_state_parser", "web_evidence_parser", "generic_json", "generic_jsonl"},
					DependencyNames:  []string{"httpx"},
				},
				{
					ID:               "template_scan",
					Description:      "Run template-based checks against an in-scope HTTP(S) target.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "target_url", Types: []string{"url"}}},
					SupportedParsers: []string{"nuclei_jsonl", "web_evidence_parser", "generic_jsonl"},
					DependencyNames:  []string{"nuclei"},
				},
				{
					ID:               "path_fuzz",
					Description:      "Discover paths or parameters using a managed wordlist.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "target_url", Types: []string{"url"}}, {Name: "wordlist_path", Types: []string{"path", "file"}}},
					SupportedParsers: []string{"ffuf_json", "route_observation_parser", "generic_json"},
					DependencyNames:  []string{"ffuf"},
				},
				{
					ID:               "crawl",
					Description:      "Crawl an in-scope web surface and emit discovered routes.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "target_url", Types: []string{"url"}}},
					SupportedParsers: []string{"katana_jsonl", "route_observation_parser", "generic_jsonl"},
					DependencyNames:  []string{"katana"},
				},
				{
					ID:               "archive_discovery",
					Description:      "Discover historical URLs for an in-scope web surface.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "target_url", Types: []string{"url"}}},
					SupportedParsers: []string{"url_list_text", "route_observation_parser", "generic_jsonl"},
					DependencyNames:  []string{"gau", "waybackurls"},
				},
				{
					ID:               "xss_scan",
					Description:      "Run XSS-focused checks against an in-scope HTTP(S) target.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "target_url", Types: []string{"url"}}},
					SupportedParsers: []string{"dalfox_json", "web_evidence_parser", "generic_json"},
					DependencyNames:  []string{"dalfox"},
				},
				{
					ID:               "parameter_discovery",
					Description:      "Discover request parameters for an in-scope HTTP(S) target.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "target_url", Types: []string{"url"}}},
					SupportedParsers: []string{"arjun_json", "route_observation_parser", "generic_json"},
					DependencyNames:  []string{"arjun"},
				},
				{
					ID:               "injection_test",
					Description:      "Run injection-focused checks against an in-scope HTTP(S) target.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "target_url", Types: []string{"url"}}},
					SupportedParsers: []string{"sqlmap_json", "web_evidence_parser", "generic_json"},
					DependencyNames:  []string{"sqlmap"},
				},
				{
					ID:               "subdomain_discovery",
					Description:      "Discover subdomains for an authorized root domain.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "root_domain", Types: []string{"domain"}}},
					SupportedParsers: []string{"subfinder_jsonl", "amass_json", "host_observation_parser", "identity_observation_parser", "generic_jsonl"},
					DependencyNames:  []string{"subfinder", "amass"},
				},
				{
					ID:               "dns_probe",
					Description:      "Resolve and classify DNS records for authorized names.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "domain", Types: []string{"domain"}}},
					SupportedParsers: []string{"dnsx_jsonl", "dnsrecon_json", "host_observation_parser", "infra_evidence_parser", "generic_jsonl"},
					DependencyNames:  []string{"dnsx", "dnsrecon"},
				},
				{
					ID:               "port_scan",
					Description:      "Enumerate reachable ports on authorized hosts or networks.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "target", Types: []string{"host", "ip", "network", "cidr"}}},
					SupportedParsers: []string{"nmap_xml", "naabu_jsonl", "host_observation_parser", "service_observation_parser", "generic_jsonl"},
					DependencyNames:  []string{"nmap", "naabu"},
				},
				{
					ID:               "service_fingerprint",
					Description:      "Fingerprint authorized services without bypassing workspace scope policy.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "target", Types: []string{"host", "ip", "network", "cidr"}}},
					SupportedParsers: []string{"nmap_xml", "infra_parser", "service_observation_parser", "infra_evidence_parser"},
					DependencyNames:  []string{"nmap"},
				},
				{
					ID:               "tls_assessment",
					Description:      "Assess TLS configuration for an authorized endpoint.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "target_host", Types: []string{"host", "domain"}}},
					SupportedParsers: []string{"testssl_json", "sslscan_xml", "service_observation_parser", "infra_evidence_parser", "generic_json"},
					DependencyNames:  []string{"testssl.sh", "sslscan"},
				},
				{
					ID:               "apk_decompile",
					Description:      "Decompile or inspect an authorized APK artifact.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "artifact_path", Types: []string{"file", "path"}}},
					SupportedParsers: []string{"apktool_text", "jadx_metadata_json", "ctf_parser", "artifact_analysis_parser"},
					DependencyNames:  []string{"apktool", "jadx"},
				},
				{
					ID:               "static_code_scan",
					Description:      "Run static application or APK source analysis on an authorized artifact.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "artifact_path", Types: []string{"file", "path"}}},
					SupportedParsers: []string{"mobsfscan_json", "semgrep_sarif", "ctf_parser", "artifact_analysis_parser", "candidate_verification_parser"},
					DependencyNames:  []string{"mobsfscan", "semgrep"},
				},
				{
					ID:               "secret_scan",
					Description:      "Scan authorized source or artifact paths for exposed secrets.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "artifact_path", Types: []string{"file", "path"}}},
					SupportedParsers: []string{"gitleaks_json", "trufflehog_json", "ctf_parser", "artifact_analysis_parser", "ctf_proof_parser"},
					DependencyNames:  []string{"gitleaks", "trufflehog"},
				},
				{
					ID:               "firmware_analyze",
					Description:      "Inspect authorized firmware artifacts.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "artifact_path", Types: []string{"file", "path"}}},
					SupportedParsers: []string{"binwalk_text", "ctf_parser", "artifact_analysis_parser"},
					DependencyNames:  []string{"binwalk"},
				},
				{
					ID:               "metadata_extract",
					Description:      "Extract metadata from authorized local artifacts.",
					RequiredInputs:   []AuthoredToolInputContract{{Name: "artifact_path", Types: []string{"file", "path"}}},
					SupportedParsers: []string{"exiftool_json", "ctf_parser", "artifact_analysis_parser"},
					DependencyNames:  []string{"exiftool"},
				},
			},
		},
	}
	parsers := []AuthoredToolParserDefinition{
		{ID: "web_api_parser", Format: "json", Description: "Workspace web/API pack parser."},
		{ID: "infra_parser", Format: "json", Description: "Workspace infra/internal pack parser."},
		{ID: "ctf_parser", Format: "json", Description: "Workspace artifact, mobile, and CTF pack parser."},
		{ID: "route_observation_parser", Format: "json", Description: "Domain route observation parser."},
		{ID: "auth_state_parser", Format: "json", Description: "Domain authentication state parser."},
		{ID: "web_evidence_parser", Format: "json", Description: "Domain web evidence parser."},
		{ID: "host_observation_parser", Format: "json", Description: "Domain host observation parser."},
		{ID: "service_observation_parser", Format: "json", Description: "Domain service observation parser."},
		{ID: "identity_observation_parser", Format: "json", Description: "Domain identity observation parser."},
		{ID: "infra_evidence_parser", Format: "json", Description: "Domain infra evidence parser."},
		{ID: "challenge_classification_parser", Format: "json", Description: "Domain challenge classification parser."},
		{ID: "artifact_analysis_parser", Format: "json", Description: "Domain artifact analysis parser."},
		{ID: "candidate_verification_parser", Format: "json", Description: "Domain candidate verification parser."},
		{ID: "ctf_proof_parser", Format: "json", Description: "Domain CTF proof parser."},
		{ID: "web_login_state_parser", Format: "json", Description: "HTTP login-state observation parser."},
		{ID: "generic_json", Format: "json", Description: "Generic single-object JSON parser contract."},
		{ID: "generic_jsonl", Format: "jsonl", Description: "Generic JSONL parser contract."},
		{ID: "url_list_text", Format: "text", Description: "Plain text URL list output."},
		{ID: "httpx_jsonl", Format: "jsonl", Description: "ProjectDiscovery httpx JSONL output."},
		{ID: "nuclei_jsonl", Format: "jsonl", Description: "ProjectDiscovery nuclei JSONL output."},
		{ID: "ffuf_json", Format: "json", Description: "ffuf JSON output."},
		{ID: "katana_jsonl", Format: "jsonl", Description: "ProjectDiscovery katana JSONL output."},
		{ID: "subfinder_jsonl", Format: "jsonl", Description: "ProjectDiscovery subfinder JSONL output."},
		{ID: "amass_json", Format: "json", Description: "Amass JSON output."},
		{ID: "dnsx_jsonl", Format: "jsonl", Description: "ProjectDiscovery dnsx JSONL output."},
		{ID: "dnsrecon_json", Format: "json", Description: "dnsrecon JSON output."},
		{ID: "dalfox_json", Format: "json", Description: "Dalfox JSON output."},
		{ID: "arjun_json", Format: "json", Description: "Arjun JSON output."},
		{ID: "sqlmap_json", Format: "json", Description: "sqlmap JSON output."},
		{ID: "nmap_xml", Format: "xml", Description: "nmap XML output."},
		{ID: "naabu_jsonl", Format: "jsonl", Description: "ProjectDiscovery naabu JSONL output."},
		{ID: "testssl_json", Format: "json", Description: "testssl.sh JSON output."},
		{ID: "sslscan_xml", Format: "xml", Description: "sslscan XML output."},
		{ID: "apktool_text", Format: "text", Description: "apktool text and manifest summary output."},
		{ID: "jadx_metadata_json", Format: "json", Description: "jadx metadata JSON output."},
		{ID: "mobsfscan_json", Format: "json", Description: "mobsfscan JSON output."},
		{ID: "semgrep_sarif", Format: "sarif", Description: "Semgrep SARIF output."},
		{ID: "gitleaks_json", Format: "json", Description: "gitleaks JSON output."},
		{ID: "trufflehog_json", Format: "json", Description: "TruffleHog JSON output."},
		{ID: "binwalk_text", Format: "text", Description: "binwalk text output."},
		{ID: "exiftool_json", Format: "json", Description: "ExifTool JSON output."},
	}
	return newAuthoredToolRegistry(adapters, parsers)
}

func newAuthoredToolRegistry(adapters []AuthoredToolAdapterDefinition, parsers []AuthoredToolParserDefinition) AuthoredToolRegistry {
	registry := AuthoredToolRegistry{
		adapters: make(map[string]AuthoredToolAdapterDefinition, len(adapters)),
		parsers:  make(map[string]AuthoredToolParserDefinition, len(parsers)),
	}
	for _, adapter := range adapters {
		adapter.ID = strings.TrimSpace(adapter.ID)
		if adapter.ID == "" {
			continue
		}
		adapter.Operations = normalizedOperations(adapter.Operations)
		registry.adapters[adapter.ID] = adapter
	}
	for _, parser := range parsers {
		parser.ID = strings.TrimSpace(parser.ID)
		if parser.ID == "" {
			continue
		}
		registry.parsers[parser.ID] = parser
	}
	return registry
}

func normalizedOperations(operations []AuthoredToolOperationDefinition) []AuthoredToolOperationDefinition {
	normalized := make([]AuthoredToolOperationDefinition, 0, len(operations))
	for _, operation := range operations {
		operation.ID = strings.TrimSpace(operation.ID)
		if operation.ID == "" {
			continue
		}
		operation.SupportedParsers = trimmedStrings(operation.SupportedParsers)
		operation.DependencyNames = trimmedStrings(operation.DependencyNames)
		for i := range operation.RequiredInputs {
			operation.RequiredInputs[i].Name = strings.TrimSpace(operation.RequiredInputs[i].Name)
			operation.RequiredInputs[i].Types = trimmedStrings(operation.RequiredInputs[i].Types)
		}
		normalized = append(normalized, operation)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return normalized[i].ID < normalized[j].ID
	})
	return normalized
}

func (r AuthoredToolRegistry) Adapters() []AuthoredToolAdapterDefinition {
	adapters := make([]AuthoredToolAdapterDefinition, 0, len(r.adapters))
	for _, adapter := range r.adapters {
		adapters = append(adapters, adapter)
	}
	sort.SliceStable(adapters, func(i, j int) bool {
		return adapters[i].ID < adapters[j].ID
	})
	return adapters
}

func (r AuthoredToolRegistry) Parsers() []AuthoredToolParserDefinition {
	parsers := make([]AuthoredToolParserDefinition, 0, len(r.parsers))
	for _, parser := range r.parsers {
		parsers = append(parsers, parser)
	}
	sort.SliceStable(parsers, func(i, j int) bool {
		return parsers[i].ID < parsers[j].ID
	})
	return parsers
}

func (r AuthoredToolRegistry) AdapterExists(adapterID string) bool {
	_, ok := r.adapters[strings.TrimSpace(adapterID)]
	return ok
}

func (r AuthoredToolRegistry) ParserExists(parserID string) bool {
	_, ok := r.parsers[strings.TrimSpace(parserID)]
	return ok
}

func (r AuthoredToolRegistry) AdapterOperation(adapterID string, operationID string) (AuthoredToolOperationDefinition, bool) {
	adapter, ok := r.adapters[strings.TrimSpace(adapterID)]
	if !ok {
		return AuthoredToolOperationDefinition{}, false
	}
	operationID = strings.TrimSpace(operationID)
	for _, operation := range adapter.Operations {
		if operation.ID == operationID {
			return operation, true
		}
	}
	return AuthoredToolOperationDefinition{}, false
}
