package toolchain

type BuiltInTool struct {
	Name           string
	Pack           string
	Category       string
	Capabilities   []string
	Dependency     string
	DependencyKind string
	TargetKind     string
	AdapterID      string
	Operation      string
	Parser         string
	RuleBundleFor  string
}

func BuiltInAppSecTools() []BuiltInTool {
	return []BuiltInTool{
		builtInBinaryTool("nuclei", "web-api-pack", "template-scanning", []string{"web.candidate_generation", "evidence.collection"}, "url", "template_scan", "nuclei_jsonl"),
		{Name: "nuclei-templates", Pack: "web-api-pack", Category: "template-scanning", Capabilities: []string{"web.candidate_generation", "evidence.collection"}, Dependency: "nuclei-templates", DependencyKind: "rule_bundle", TargetKind: "rule_bundle", RuleBundleFor: "nuclei"},
		builtInBinaryTool("httpx", "web-api-pack", "http-probing", []string{"web.discovery", "web.route_enumeration"}, "url", "http_probe", "httpx_jsonl"),
		builtInBinaryTool("subfinder", "web-api-pack", "asset-discovery", []string{"web.discovery"}, "domain", "subdomain_discovery", "subfinder_jsonl"),
		builtInBinaryTool("ffuf", "web-api-pack", "content-discovery", []string{"web.route_enumeration"}, "url", "path_fuzz", "ffuf_json"),
		builtInBinaryTool("katana", "web-api-pack", "crawling", []string{"web.route_enumeration"}, "url", "crawl", "katana_jsonl"),
		builtInBinaryTool("gau", "web-api-pack", "archive-discovery", []string{"web.route_enumeration"}, "url", "archive_discovery", "url_list_text"),
		builtInBinaryTool("waybackurls", "web-api-pack", "archive-discovery", []string{"web.route_enumeration"}, "url", "archive_discovery", "url_list_text"),
		builtInBinaryTool("dalfox", "web-api-pack", "xss-testing", []string{"web.candidate_generation", "evidence.collection"}, "url", "xss_scan", "dalfox_json"),
		builtInBinaryTool("arjun", "web-api-pack", "parameter-discovery", []string{"web.route_enumeration"}, "url", "parameter_discovery", "arjun_json"),
		builtInBinaryTool("sqlmap", "web-api-pack", "injection-testing", []string{"web.candidate_generation", "evidence.collection"}, "url", "injection_test", "sqlmap_json"),
		builtInBinaryTool("nmap", "infra-pack", "service-enumeration", []string{"infra.host_discovery", "infra.service_enumeration"}, "host", "service_fingerprint", "nmap_xml"),
		builtInBinaryTool("naabu", "infra-pack", "port-discovery", []string{"infra.host_discovery", "infra.service_enumeration"}, "host", "port_scan", "naabu_jsonl"),
		builtInBinaryTool("dnsx", "infra-pack", "dns-enumeration", []string{"infra.host_discovery", "infra.service_enumeration"}, "domain", "dns_probe", "dnsx_jsonl"),
		builtInBinaryTool("amass", "infra-pack", "asset-discovery", []string{"infra.host_discovery", "infra.identity_graph"}, "domain", "subdomain_discovery", "amass_json"),
		builtInBinaryTool("dnsrecon", "infra-pack", "dns-enumeration", []string{"infra.host_discovery", "infra.service_enumeration"}, "domain", "dns_probe", "dnsrecon_json"),
		builtInBinaryTool("testssl.sh", "infra-pack", "tls-assessment", []string{"infra.service_enumeration", "evidence.collection"}, "host", "tls_assessment", "testssl_json"),
		builtInBinaryTool("sslscan", "infra-pack", "tls-assessment", []string{"infra.service_enumeration", "evidence.collection"}, "host", "tls_assessment", "sslscan_xml"),
		builtInBinaryTool("apktool", "ctf-pack", "mobile-app-analysis", []string{"ctf.artifact_analysis", "evidence.collection"}, "artifact", "apk_decompile", "apktool_text"),
		builtInBinaryTool("jadx", "ctf-pack", "mobile-app-analysis", []string{"ctf.artifact_analysis", "evidence.collection"}, "artifact", "apk_decompile", "jadx_metadata_json"),
		builtInBinaryTool("mobsfscan", "ctf-pack", "mobile-app-analysis", []string{"ctf.artifact_analysis", "evidence.collection"}, "artifact", "static_code_scan", "mobsfscan_json"),
		builtInBinaryTool("semgrep", "ctf-pack", "static-analysis", []string{"ctf.artifact_analysis", "evidence.collection"}, "artifact", "static_code_scan", "semgrep_sarif"),
		builtInBinaryTool("gitleaks", "ctf-pack", "secret-detection", []string{"ctf.artifact_analysis", "evidence.collection"}, "artifact", "secret_scan", "gitleaks_json"),
		builtInBinaryTool("trufflehog", "ctf-pack", "secret-detection", []string{"ctf.artifact_analysis", "evidence.collection"}, "artifact", "secret_scan", "trufflehog_json"),
		builtInBinaryTool("binwalk", "ctf-pack", "firmware-analysis", []string{"ctf.artifact_analysis", "evidence.collection"}, "artifact", "firmware_analyze", "binwalk_text"),
		builtInBinaryTool("exiftool", "ctf-pack", "metadata-analysis", []string{"ctf.artifact_analysis", "evidence.collection"}, "artifact", "metadata_extract", "exiftool_json"),
	}
}

func builtInBinaryTool(name string, pack string, category string, capabilities []string, targetKind string, operation string, parser string) BuiltInTool {
	return BuiltInTool{
		Name:           name,
		Pack:           pack,
		Category:       category,
		Capabilities:   capabilities,
		Dependency:     name,
		DependencyKind: "binary",
		TargetKind:     targetKind,
		AdapterID:      "managed_binary",
		Operation:      operation,
		Parser:         parser,
	}
}
