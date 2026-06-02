package toolchain

import (
	"strings"
	"testing"
)

func TestAuthoredToolToManifestAcceptsStructuredNativeToolSpec(t *testing.T) {
	spec := AuthoredToolSpec{
		Name:         "web-login-state-enumerator",
		Version:      "0.1.0",
		Capabilities: []string{"web.auth_state_tracking", "evidence.collection"},
		Parser:       "web_login_state_parser",
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
			{Name: "httpx", Kind: "binary", HealthCommand: []string{"httpx", "-version"}},
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

func TestValidateAuthoredToolSpecRequiresEvidenceAndSafety(t *testing.T) {
	spec := validAuthoredToolSpec()
	spec.ExpectedEvidence = nil
	err := ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "expected_evidence") {
		t.Fatalf("expected evidence validation error, got %v", err)
	}

	spec = validAuthoredToolSpec()
	spec.Safety = AuthoredToolSafety{}
	err = ValidateAuthoredToolSpec(spec)
	if err == nil || !strings.Contains(err.Error(), "safety") {
		t.Fatalf("expected safety validation error, got %v", err)
	}
}

func TestValidateAuthoredToolSpecRequiresExplicitSafetyClosePaths(t *testing.T) {
	spec := validAuthoredToolSpec()
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
		Parser:       "web_login_state_parser",
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
	}
}
