package toolchain

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltInAppSecToolsDeclareToolLevelContracts(t *testing.T) {
	registry := DefaultAuthoredToolRegistry()
	byName := builtInToolByNameForTest(t)

	for _, name := range []string{
		"nuclei",
		"httpx",
		"subfinder",
		"ffuf",
		"katana",
		"gau",
		"waybackurls",
		"dalfox",
		"arjun",
		"sqlmap",
		"nmap",
		"naabu",
		"dnsx",
		"amass",
		"dnsrecon",
		"testssl.sh",
		"sslscan",
		"apktool",
		"jadx",
		"mobsfscan",
		"semgrep",
		"gitleaks",
		"trufflehog",
		"binwalk",
		"exiftool",
	} {
		tool := byName[name]
		if strings.TrimSpace(tool.Dependency) != name || strings.TrimSpace(tool.DependencyKind) != "binary" {
			t.Fatalf("expected binary dependency contract for %s, got %#v", name, tool)
		}
		if strings.TrimSpace(tool.TargetKind) == "" {
			t.Fatalf("expected target kind for %s, got %#v", name, tool)
		}
		if strings.TrimSpace(tool.AdapterID) == "" || strings.TrimSpace(tool.Operation) == "" || strings.TrimSpace(tool.Parser) == "" {
			t.Fatalf("expected adapter operation parser contract for %s, got %#v", name, tool)
		}
		operation, ok := registry.AdapterOperation(tool.AdapterID, tool.Operation)
		if !ok {
			t.Fatalf("expected registered adapter operation for %s, got %#v", name, tool)
		}
		if !registry.ParserExists(tool.Parser) {
			t.Fatalf("expected registered parser for %s, got %#v", name, tool)
		}
		if !containsTrimmed(operation.SupportedParsers, tool.Parser) {
			t.Fatalf("expected operation %s to support parser %s for %s", operation.ID, tool.Parser, name)
		}
		if !containsTrimmed(operation.DependencyNames, tool.Dependency) {
			t.Fatalf("expected operation %s to allow dependency %s for %s, got %#v", operation.ID, tool.Dependency, name, operation.DependencyNames)
		}
	}

	templates := byName["nuclei-templates"]
	if templates.Dependency != "nuclei-templates" || templates.DependencyKind != "rule_bundle" || templates.RuleBundleFor != "nuclei" {
		t.Fatalf("expected nuclei-templates to be a nuclei rule bundle contract, got %#v", templates)
	}
}

func TestArchiveDiscoveryToolsUseDomainScopeContracts(t *testing.T) {
	registry := DefaultAuthoredToolRegistry()
	operation, ok := registry.AdapterOperation("managed_binary", "archive_discovery")
	if !ok {
		t.Fatalf("expected archive_discovery operation to be registered")
	}
	if len(operation.RequiredInputs) != 1 || operation.RequiredInputs[0].Name != "root_domain" || !containsTrimmed(operation.RequiredInputs[0].Types, "domain") {
		t.Fatalf("expected archive_discovery to require root_domain domain input, got %#v", operation.RequiredInputs)
	}

	byName := builtInToolByNameForTest(t)
	for _, name := range []string{"gau", "waybackurls"} {
		tool := byName[name]
		if tool.TargetKind != "domain" || tool.Operation != "archive_discovery" || tool.Parser != "url_list_text" {
			t.Fatalf("expected %s archive discovery to use domain scope and url_list_text parser, got %#v", name, tool)
		}
	}
}

func TestBuiltInAppSecToolsCoverManifestDependencies(t *testing.T) {
	manifests, err := LoadManifests(filepath.Join("..", "..", "toolchains"))
	if err != nil {
		t.Fatalf("load root toolchain manifests: %v", err)
	}
	byDependency := map[string]BuiltInTool{}
	for _, tool := range BuiltInAppSecTools() {
		if strings.TrimSpace(tool.Dependency) == "" {
			continue
		}
		byDependency[tool.Dependency] = tool
	}

	for _, manifest := range manifests {
		manifestCapabilities := map[string]bool{}
		for _, capability := range manifest.Capabilities {
			manifestCapabilities[capability] = true
		}
		for _, dependency := range manifest.Dependencies {
			tool, ok := byDependency[dependency.Name]
			if !ok {
				t.Fatalf("expected manifest dependency %s/%s to have a built-in tool contract", manifest.Name, dependency.Name)
			}
			if tool.Pack != manifest.Name {
				t.Fatalf("expected dependency %s in pack %s, got catalog tool %#v", dependency.Name, manifest.Name, tool)
			}
			if dependencyKind(dependency) != tool.DependencyKind {
				t.Fatalf("expected dependency %s kind %s, got catalog tool %#v", dependency.Name, dependencyKind(dependency), tool)
			}
			for _, capability := range tool.Capabilities {
				if !manifestCapabilities[capability] {
					t.Fatalf("expected catalog capability %s for %s to be declared by manifest %s", capability, tool.Name, manifest.Name)
				}
			}
		}
	}
}

func builtInToolByNameForTest(t *testing.T) map[string]BuiltInTool {
	t.Helper()

	byName := map[string]BuiltInTool{}
	for _, tool := range BuiltInAppSecTools() {
		if _, exists := byName[tool.Name]; exists {
			t.Fatalf("duplicate built-in tool name: %s", tool.Name)
		}
		byName[tool.Name] = tool
	}
	return byName
}
