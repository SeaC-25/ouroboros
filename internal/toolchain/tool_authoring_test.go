package toolchain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthoredToolToManifestAcceptsStructuredNativeToolSpec(t *testing.T) {
	spec := AuthoredToolSpec{
		Name:         "web-login-state-enumerator",
		Version:      "0.1.0",
		Capabilities: []string{"web.auth_state_tracking", "evidence.collection"},
		Parser:       "web_login_state_parser",
		Adapter:      AuthoredToolAdapter{ID: "managed_binary", Operation: "http_probe"},
		Inputs: []AuthoredToolInput{
			{Name: "target_url", Type: "url", Required: true},
		},
		ExpectedEvidence: []AuthoredToolEvidence{
			{Type: "http_observation", Fields: []string{"status_code", "body_hash"}},
		},
		Safety: AuthoredToolSafety{
			TargetPolicy:              "enforce",
			Isolation:                 "process",
			AllowTargetPolicyDisabled: true,
			AllowIsolationDisabled:    true,
		},
		Dependencies: []Dependency{
			{Name: "httpx", Kind: "binary", Source: "projectdiscovery/httpx"},
		},
	}

	manifest, err := AuthoredToolToManifest(spec)
	if err != nil {
		t.Fatalf("convert authored tool: %v", err)
	}
	if manifest.Name != spec.Name || manifest.Version != spec.Version {
		t.Fatalf("expected authored name/version to be retained, got %#v", manifest)
	}
	if manifest.Runner != "native" || len(manifest.RunCommand) != 0 {
		t.Fatalf("expected authored tools to become native manifests without raw commands, got %#v", manifest)
	}
	if manifest.Parser != spec.Parser || manifest.Healthcheck != "built_in" {
		t.Fatalf("expected parser and built_in healthcheck, got %#v", manifest)
	}
	if len(manifest.Dependencies) != 1 || manifest.Dependencies[0].Name != "httpx" {
		t.Fatalf("expected dependencies to be retained, got %#v", manifest.Dependencies)
	}
}

func TestValidateAuthoredToolSpecRejectsRawScripts(t *testing.T) {
	spec := validAuthoredToolSpec()
	spec.RawScript = "nmap -sV $TARGET"

	err := ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "raw_script") {
		t.Fatalf("expected raw script rejection, got %v", err)
	}

	spec = validAuthoredToolSpec()
	spec.Runner = "command"
	spec.RunCommand = []string{"sh", "-c", "nuclei -u $TARGET"}
	err = ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "runner") || !strings.Contains(err.Error(), "run_command") {
		t.Fatalf("expected command-runner rejection, got %v", err)
	}
}

func TestValidateAuthoredToolSpecRequiresNativeAdapterOperation(t *testing.T) {
	spec := validAuthoredToolSpec()

	err := ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "adapter") || !strings.Contains(err.Error(), "operation") {
		t.Fatalf("expected adapter/operation validation error, got %v", err)
	}

	spec.Adapter = AuthoredToolAdapter{ID: "managed_binary", Operation: "http_probe"}
	if err := ValidateAuthoredToolSpec(spec); err != nil {
		t.Fatalf("expected authored tool adapter contract to validate: %v", err)
	}
}

func TestDefaultAuthoredToolRegistryIncludesPentestOperations(t *testing.T) {
	registry := DefaultAuthoredToolRegistry()

	for _, operation := range []string{
		"http_probe",
		"template_scan",
		"path_fuzz",
		"crawl",
		"subdomain_discovery",
		"port_scan",
		"service_fingerprint",
		"apk_decompile",
		"static_code_scan",
		"secret_scan",
	} {
		registered, ok := registry.AdapterOperation("managed_binary", operation)
		if !ok {
			t.Fatalf("expected managed_binary operation %s to be registered", operation)
		}
		if registered.ID != operation || len(registered.RequiredInputs) == 0 {
			t.Fatalf("expected operation metadata for %s, got %#v", operation, registered)
		}
	}

	for _, parser := range []string{
		"web_api_parser",
		"web_login_state_parser",
		"route_observation_parser",
		"service_observation_parser",
		"artifact_analysis_parser",
		"httpx_jsonl",
		"nuclei_jsonl",
		"ffuf_json",
		"katana_jsonl",
		"subfinder_jsonl",
		"nmap_xml",
		"naabu_jsonl",
		"mobsfscan_json",
		"semgrep_sarif",
	} {
		if !registry.ParserExists(parser) {
			t.Fatalf("expected parser %s to be registered", parser)
		}
	}
}

func TestValidateAuthoredToolSpecRejectsUnknownRegistryContracts(t *testing.T) {
	spec := validAuthoredToolSpec()
	spec.Adapter = AuthoredToolAdapter{ID: "shell", Operation: "http_probe"}

	err := ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "adapter id is not registered: shell") {
		t.Fatalf("expected unknown adapter rejection, got %v", err)
	}

	spec = validAuthoredToolSpec()
	spec.Adapter = AuthoredToolAdapter{ID: "managed_binary", Operation: "curl_probe"}
	err = ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "adapter operation is not registered: managed_binary/curl_probe") {
		t.Fatalf("expected unknown operation rejection, got %v", err)
	}

	spec = validAuthoredToolSpec()
	spec.Adapter = AuthoredToolAdapter{ID: "managed_binary", Operation: "http_probe"}
	spec.Parser = "bespoke_parser"
	err = ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "parser is not registered: bespoke_parser") {
		t.Fatalf("expected unknown parser rejection, got %v", err)
	}
}

func TestValidateAuthoredToolSpecChecksOperationInputsAndParserCompatibility(t *testing.T) {
	spec := validAuthoredToolSpec()
	spec.Adapter = AuthoredToolAdapter{ID: "managed_binary", Operation: "http_probe"}
	spec.Inputs = []AuthoredToolInput{{Name: "target", Type: "url", Required: true}}

	err := ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "adapter operation http_probe requires input target_url") {
		t.Fatalf("expected required input validation error, got %v", err)
	}

	spec = validAuthoredToolSpec()
	spec.Adapter = AuthoredToolAdapter{ID: "managed_binary", Operation: "http_probe"}
	spec.Inputs = []AuthoredToolInput{{Name: "target_url", Type: "string", Required: true}}
	err = ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "input target_url type must be one of url") {
		t.Fatalf("expected input type validation error, got %v", err)
	}

	spec = validAuthoredToolSpec()
	spec.Adapter = AuthoredToolAdapter{ID: "managed_binary", Operation: "template_scan"}
	spec.Parser = "httpx_jsonl"
	err = ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "parser httpx_jsonl is not supported by adapter operation template_scan") {
		t.Fatalf("expected parser compatibility validation error, got %v", err)
	}
}

func TestValidateAuthoredToolSpecRejectsDependencyCommandArrays(t *testing.T) {
	spec := validAuthoredToolSpec()
	spec.Adapter = AuthoredToolAdapter{ID: "managed_binary", Operation: "http_probe"}
	spec.Dependencies = []Dependency{
		{Name: "httpx", Kind: "binary", HealthCommand: []string{"httpx", "-version"}},
	}

	err := ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "dependencies 0 health_command") {
		t.Fatalf("expected authored dependency health_command rejection, got %v", err)
	}

	spec.Dependencies = []Dependency{
		{Name: "httpx", Kind: "binary", InstallCommand: []string{"ouro-toolchain", "install", "httpx"}},
	}
	err = ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "dependencies 0 install_command") {
		t.Fatalf("expected authored dependency install_command rejection, got %v", err)
	}
}

func TestLoadAuthoredToolSpecRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authored-tool.yaml")
	data := validAuthoredToolSpecYAMLForToolchainTest() + "scriptlet: nuclei -u $TARGET\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write authored tool spec: %v", err)
	}

	_, err := LoadAuthoredToolSpec(path)
	if err == nil || !strings.Contains(err.Error(), "scriptlet") {
		t.Fatalf("expected unknown field rejection, got %v", err)
	}
}

func TestValidateAuthoredToolSpecRequiresEvidenceAndSafety(t *testing.T) {
	spec := validAuthoredToolSpec()
	spec.Adapter = AuthoredToolAdapter{ID: "managed_binary", Operation: "http_probe"}
	spec.ExpectedEvidence = nil
	err := ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "expected_evidence") {
		t.Fatalf("expected evidence validation error, got %v", err)
	}

	spec = validAuthoredToolSpec()
	spec.Adapter = AuthoredToolAdapter{ID: "managed_binary", Operation: "http_probe"}
	spec.Safety = AuthoredToolSafety{}
	err = ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "safety") {
		t.Fatalf("expected safety validation error, got %v", err)
	}
}

func TestValidateAuthoredToolSpecRequiresExplicitSafetyClosePaths(t *testing.T) {
	spec := validAuthoredToolSpec()
	spec.Adapter = AuthoredToolAdapter{ID: "managed_binary", Operation: "http_probe"}
	spec.Safety.TargetPolicy = "disabled"
	err := ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "allow_target_policy_disabled") {
		t.Fatalf("expected target-policy close path validation error, got %v", err)
	}
	spec.Safety.AllowTargetPolicyDisabled = true
	if err := ValidateAuthoredToolSpec(spec); err != nil {
		t.Fatalf("expected explicit target-policy close path to validate: %v", err)
	}

	spec = validAuthoredToolSpec()
	spec.Adapter = AuthoredToolAdapter{ID: "managed_binary", Operation: "http_probe"}
	spec.Safety.Isolation = "disabled"
	err = ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "allow_isolation_disabled") {
		t.Fatalf("expected isolation close path validation error, got %v", err)
	}
	spec.Safety.AllowIsolationDisabled = true
	if err := ValidateAuthoredToolSpec(spec); err != nil {
		t.Fatalf("expected explicit isolation close path to validate: %v", err)
	}
}

func validAuthoredToolSpec() AuthoredToolSpec {
	return AuthoredToolSpec{
		Name:         "web-login-state-enumerator",
		Version:      "0.1.0",
		Capabilities: []string{"web.auth_state_tracking"},
		Parser:       "httpx_jsonl",
		Inputs: []AuthoredToolInput{
			{Name: "target_url", Type: "url", Required: true},
		},
		ExpectedEvidence: []AuthoredToolEvidence{
			{Type: "http_observation", Fields: []string{"status_code"}},
		},
		Safety: AuthoredToolSafety{
			TargetPolicy: "enforce",
			Isolation:    "process",
		},
		Dependencies: []Dependency{
			{Name: "httpx", Kind: "binary", Optional: true, Source: "projectdiscovery/httpx"},
		},
	}
}

func validAuthoredToolSpecYAMLForToolchainTest() string {
	return `name: web-login-state-enumerator
version: "0.1.0"
capabilities:
  - web.auth_state_tracking
parser: web_login_state_parser
adapter:
  id: managed_binary
  operation: http_probe
inputs:
  - name: target_url
    type: url
    required: true
expected_evidence:
  - type: http_observation
    fields:
      - status_code
safety:
  target_policy: enforce
  isolation: process
`
}
