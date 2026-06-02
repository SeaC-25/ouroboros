package toolchain

type BuiltInTool struct {
	Name         string
	Pack         string
	Category     string
	Capabilities []string
}

func BuiltInAppSecTools() []BuiltInTool {
	return []BuiltInTool{
		{Name: "nuclei", Pack: "web-api-pack", Category: "template-scanning", Capabilities: []string{"web.candidate_generation", "evidence.collection"}},
		{Name: "nuclei-templates", Pack: "web-api-pack", Category: "template-scanning", Capabilities: []string{"web.candidate_generation", "evidence.collection"}},
		{Name: "httpx", Pack: "web-api-pack", Category: "http-probing", Capabilities: []string{"web.discovery", "web.route_enumeration"}},
		{Name: "subfinder", Pack: "web-api-pack", Category: "asset-discovery", Capabilities: []string{"web.discovery"}},
		{Name: "ffuf", Pack: "web-api-pack", Category: "content-discovery", Capabilities: []string{"web.route_enumeration"}},
		{Name: "katana", Pack: "web-api-pack", Category: "crawling", Capabilities: []string{"web.route_enumeration"}},
		{Name: "gau", Pack: "web-api-pack", Category: "archive-discovery", Capabilities: []string{"web.route_enumeration"}},
		{Name: "waybackurls", Pack: "web-api-pack", Category: "archive-discovery", Capabilities: []string{"web.route_enumeration"}},
		{Name: "dalfox", Pack: "web-api-pack", Category: "xss-testing", Capabilities: []string{"web.candidate_generation", "evidence.collection"}},
		{Name: "arjun", Pack: "web-api-pack", Category: "parameter-discovery", Capabilities: []string{"web.route_enumeration"}},
		{Name: "sqlmap", Pack: "web-api-pack", Category: "injection-testing", Capabilities: []string{"web.candidate_generation", "evidence.collection"}},
		{Name: "nmap", Pack: "infra-pack", Category: "service-enumeration", Capabilities: []string{"infra.host_discovery", "infra.service_enumeration"}},
		{Name: "naabu", Pack: "infra-pack", Category: "port-discovery", Capabilities: []string{"infra.host_discovery", "infra.service_enumeration"}},
		{Name: "dnsx", Pack: "infra-pack", Category: "dns-enumeration", Capabilities: []string{"infra.host_discovery", "infra.service_enumeration"}},
		{Name: "amass", Pack: "infra-pack", Category: "asset-discovery", Capabilities: []string{"infra.host_discovery", "infra.identity_graph"}},
		{Name: "dnsrecon", Pack: "infra-pack", Category: "dns-enumeration", Capabilities: []string{"infra.host_discovery", "infra.service_enumeration"}},
		{Name: "testssl.sh", Pack: "infra-pack", Category: "tls-assessment", Capabilities: []string{"infra.service_enumeration", "evidence.collection"}},
		{Name: "sslscan", Pack: "infra-pack", Category: "tls-assessment", Capabilities: []string{"infra.service_enumeration", "evidence.collection"}},
		{Name: "apktool", Pack: "ctf-pack", Category: "mobile-app-analysis", Capabilities: []string{"ctf.artifact_analysis", "evidence.collection"}},
		{Name: "jadx", Pack: "ctf-pack", Category: "mobile-app-analysis", Capabilities: []string{"ctf.artifact_analysis", "evidence.collection"}},
		{Name: "mobsfscan", Pack: "ctf-pack", Category: "mobile-app-analysis", Capabilities: []string{"ctf.artifact_analysis", "evidence.collection"}},
		{Name: "semgrep", Pack: "ctf-pack", Category: "static-analysis", Capabilities: []string{"ctf.artifact_analysis", "evidence.collection"}},
		{Name: "gitleaks", Pack: "ctf-pack", Category: "secret-detection", Capabilities: []string{"ctf.artifact_analysis", "evidence.collection"}},
		{Name: "trufflehog", Pack: "ctf-pack", Category: "secret-detection", Capabilities: []string{"ctf.artifact_analysis", "evidence.collection"}},
		{Name: "binwalk", Pack: "ctf-pack", Category: "firmware-analysis", Capabilities: []string{"ctf.artifact_analysis", "evidence.collection"}},
		{Name: "exiftool", Pack: "ctf-pack", Category: "metadata-analysis", Capabilities: []string{"ctf.artifact_analysis", "evidence.collection"}},
	}
}
