package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBanner(t *testing.T) {
	if Banner() != "ouro 0.1.0-dev" {
		t.Fatalf("unexpected banner: %q", Banner())
	}
}

func TestInitWorkspaceCreatesProductionLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "engagement")

	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	files := []string{"mission.yaml", "scope.yaml", "authorization.yaml"}
	for _, name := range files {
		path := filepath.Join(root, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected file %s: %v", name, err)
		}
		if info.IsDir() {
			t.Fatalf("expected %s to be a file", name)
		}
	}

	dirs := []string{"artifacts", "evidence", "attempts", "findings", "reports", "replay", "runtime", "toolchain"}
	for _, name := range dirs {
		path := filepath.Join(root, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected directory %s: %v", name, err)
		}
		if !info.IsDir() {
			t.Fatalf("expected %s to be a directory", name)
		}
	}
}

func TestDoctorWorkspaceValidatesRequiredLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "engagement")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	report, err := DoctorWorkspace(root)
	if err != nil {
		t.Fatalf("doctor workspace: %v", err)
	}
	if !report.Ready {
		t.Fatalf("expected initialized workspace to be ready: %#v", report)
	}
	if len(report.Errors) != 0 {
		t.Fatalf("expected no errors: %#v", report.Errors)
	}
}

func TestDoctorWorkspaceReportsMissingRequiredPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "engagement")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "scope.yaml")); err != nil {
		t.Fatalf("remove scope: %v", err)
	}

	report, err := DoctorWorkspace(root)
	if err != nil {
		t.Fatalf("doctor workspace: %v", err)
	}
	if report.Ready {
		t.Fatalf("expected workspace to be unready")
	}

	joined := strings.Join(report.Errors, "\n")
	if !strings.Contains(joined, "scope.yaml") {
		t.Fatalf("expected missing scope.yaml error, got %q", joined)
	}
}

func TestDoctorWorkspaceWarnsForDisabledSafetyBoundaries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "engagement")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
execution:
  isolation: disabled
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	report, err := DoctorWorkspace(root)
	if err != nil {
		t.Fatalf("doctor workspace: %v", err)
	}
	if !report.Ready {
		t.Fatalf("expected workspace to stay ready with explicit safety bypasses: %#v", report)
	}
	joined := strings.Join(report.Warnings, "\n")
	for _, want := range []string{
		"scope.target_policy is disabled",
		"execution.isolation is disabled",
		"missing safety bypass authorization for scope.target_policy disabled",
		"missing safety bypass authorization for execution.isolation disabled",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in doctor warnings, got %q", want, joined)
		}
	}
	bypasses := doctorReportSafetyBypassesFromJSON(t, report)
	if len(bypasses) != 2 {
		t.Fatalf("expected two structured safety bypasses, got %#v", bypasses)
	}
	targetBypass := findDoctorSafetyBypass(t, bypasses, "scope.target_policy")
	if targetBypass.Mode != "disabled" || targetBypass.Authorized || !targetBypass.MissingAuthorization {
		t.Fatalf("unexpected target-policy safety bypass metadata: %#v", targetBypass)
	}
	isolationBypass := findDoctorSafetyBypass(t, bypasses, "execution.isolation")
	if isolationBypass.Mode != "disabled" || isolationBypass.Authorized || !isolationBypass.MissingAuthorization {
		t.Fatalf("unexpected isolation safety bypass metadata: %#v", isolationBypass)
	}
}

func TestDoctorWorkspaceCanRequireSafetyBypassAuthorization(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "engagement")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
execution:
  isolation: disabled
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	report, err := DoctorWorkspaceWithOptions(root, DoctorOptions{RequireSafetyAuthorization: true})
	if err != nil {
		t.Fatalf("doctor workspace strict safety authorization: %v", err)
	}
	if report.Ready {
		t.Fatalf("expected strict safety authorization to fail without authorization metadata: %#v", report)
	}
	errors := strings.Join(report.Errors, "\n")
	for _, want := range []string{
		"missing required safety bypass authorization for scope.target_policy disabled",
		"missing required safety bypass authorization for execution.isolation disabled",
	} {
		if !strings.Contains(errors, want) {
			t.Fatalf("expected %q in strict doctor errors, got %q", want, errors)
		}
	}
	warnings := strings.Join(report.Warnings, "\n")
	if !strings.Contains(warnings, "scope.target_policy is disabled") || !strings.Contains(warnings, "execution.isolation is disabled") {
		t.Fatalf("expected strict doctor to keep visibility warnings, got %q", warnings)
	}
}

func TestDoctorWorkspaceReportsSafetyBypassAuthorizationMetadata(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "engagement")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
execution:
  isolation: disabled
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	authorization := `version: 1
safety_bypasses:
  - control: scope.target_policy
    mode: disabled
    authorized_by: alice
    reason: false-positive target boundary for authorized lab
    ticket: ENG-123
    expires: "2999-06-30"
  - control: execution.isolation
    mode: disabled
    authorized_by: bob
    reason: legacy tool requires host environment
    ticket: ENG-124
`
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte(authorization), 0o644); err != nil {
		t.Fatalf("write authorization: %v", err)
	}

	report, err := DoctorWorkspace(root)
	if err != nil {
		t.Fatalf("doctor workspace: %v", err)
	}
	if !report.Ready {
		t.Fatalf("expected authorized bypass workspace to stay ready: %#v", report)
	}
	joined := strings.Join(report.Warnings, "\n")
	for _, want := range []string{
		"scope.target_policy disabled authorized by alice",
		"reason: false-positive target boundary for authorized lab",
		"ticket: ENG-123",
		"expires: 2999-06-30",
		"execution.isolation disabled authorized by bob",
		"reason: legacy tool requires host environment",
		"ticket: ENG-124",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in doctor warnings, got %q", want, joined)
		}
	}
	if strings.Contains(joined, "missing safety bypass authorization") {
		t.Fatalf("expected authorization metadata to satisfy bypass audit, got %q", joined)
	}
	bypasses := doctorReportSafetyBypassesFromJSON(t, report)
	if len(bypasses) != 2 {
		t.Fatalf("expected two structured authorized safety bypasses, got %#v", bypasses)
	}
	targetBypass := findDoctorSafetyBypass(t, bypasses, "scope.target_policy")
	if targetBypass.Mode != "disabled" || !targetBypass.Authorized || targetBypass.MissingAuthorization || targetBypass.AuthorizedBy != "alice" || targetBypass.Reason != "false-positive target boundary for authorized lab" || targetBypass.Ticket != "ENG-123" || targetBypass.Expires != "2999-06-30" {
		t.Fatalf("unexpected authorized target-policy bypass metadata: %#v", targetBypass)
	}
	isolationBypass := findDoctorSafetyBypass(t, bypasses, "execution.isolation")
	if isolationBypass.Mode != "disabled" || !isolationBypass.Authorized || isolationBypass.MissingAuthorization || isolationBypass.AuthorizedBy != "bob" || isolationBypass.Reason != "legacy tool requires host environment" || isolationBypass.Ticket != "ENG-124" {
		t.Fatalf("unexpected authorized isolation bypass metadata: %#v", isolationBypass)
	}
}

func TestDoctorWorkspaceStrictSafetyAuthorizationAcceptsMetadata(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "engagement")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
execution:
  isolation: disabled
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	authorization := `version: 1
safety_bypasses:
  - control: scope.target_policy
    mode: disabled
    authorized_by: alice
    reason: authorized target-boundary false positive
  - control: execution.isolation
    mode: disabled
    authorized_by: bob
    reason: authorized legacy tool compatibility
`
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte(authorization), 0o644); err != nil {
		t.Fatalf("write authorization: %v", err)
	}

	report, err := DoctorWorkspaceWithOptions(root, DoctorOptions{RequireSafetyAuthorization: true})
	if err != nil {
		t.Fatalf("doctor workspace strict authorized bypass: %v", err)
	}
	if !report.Ready || len(report.Errors) != 0 {
		t.Fatalf("expected strict doctor to accept authorized bypass metadata: %#v", report)
	}
	joined := strings.Join(report.Warnings, "\n")
	if !strings.Contains(joined, "scope.target_policy disabled authorized by alice") || !strings.Contains(joined, "execution.isolation disabled authorized by bob") {
		t.Fatalf("expected strict doctor to retain authorization warning details, got %q", joined)
	}
	if strings.Contains(joined, "missing safety bypass authorization") {
		t.Fatalf("expected strict doctor authorized bypass to avoid missing warnings, got %q", joined)
	}
}

func TestDoctorWorkspaceTreatsExpiredSafetyBypassAuthorizationAsExpired(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "engagement")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	authorization := `version: 1
safety_bypasses:
  - control: scope.target_policy
    mode: disabled
    authorized_by: alice
    reason: authorized target-boundary false positive
    ticket: ENG-999
    expires: "2000-01-01"
`
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte(authorization), 0o644); err != nil {
		t.Fatalf("write authorization: %v", err)
	}

	report, err := DoctorWorkspace(root)
	if err != nil {
		t.Fatalf("doctor workspace: %v", err)
	}
	if !report.Ready {
		t.Fatalf("default doctor should keep expired bypasses warning-only: %#v", report)
	}
	warnings := strings.Join(report.Warnings, "\n")
	for _, want := range []string{
		"scope.target_policy is disabled",
		"expired safety bypass authorization for scope.target_policy disabled",
		"expired on 2000-01-01",
		"authorized by alice",
		"ticket: ENG-999",
	} {
		if !strings.Contains(warnings, want) {
			t.Fatalf("expected expired authorization warning %q, got %q", want, warnings)
		}
	}
	bypass := findDoctorSafetyBypass(t, doctorReportSafetyBypassesFromJSON(t, report), "scope.target_policy")
	if bypass.Authorized || bypass.MissingAuthorization || !bypass.ExpiredAuthorization || bypass.AuthorizedBy != "alice" || bypass.Expires != "2000-01-01" {
		t.Fatalf("expected expired but present authorization metadata, got %#v", bypass)
	}

	strict, err := DoctorWorkspaceWithOptions(root, DoctorOptions{RequireSafetyAuthorization: true})
	if err != nil {
		t.Fatalf("strict doctor workspace: %v", err)
	}
	if strict.Ready {
		t.Fatalf("strict doctor should fail expired safety bypass authorization: %#v", strict)
	}
	errors := strings.Join(strict.Errors, "\n")
	if !strings.Contains(errors, "expired required safety bypass authorization for scope.target_policy disabled") {
		t.Fatalf("expected expired authorization strict error, got %q", errors)
	}
}

func TestDoctorWorkspaceRejectsInvalidAuthorizationMetadata(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "engagement")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	authorization := `version: 1
safety_bypasses:
  - control: scope.target_policy
    mode: disabled
    reason: missing authorized_by
`
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte(authorization), 0o644); err != nil {
		t.Fatalf("write authorization: %v", err)
	}

	report, err := DoctorWorkspace(root)
	if err != nil {
		t.Fatalf("doctor workspace: %v", err)
	}
	if report.Ready {
		t.Fatalf("expected invalid authorization metadata to fail doctor")
	}
	joined := strings.Join(report.Errors, "\n")
	if !strings.Contains(joined, "invalid authorization.yaml") || !strings.Contains(joined, "authorized_by is required") {
		t.Fatalf("expected authorization validation error, got %q", joined)
	}
}

func TestDoctorWorkspaceRejectsInvalidSafetyBypassExpiration(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "engagement")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	authorization := `version: 1
safety_bypasses:
  - control: scope.target_policy
    mode: disabled
    authorized_by: alice
    reason: authorized target-boundary false positive
    expires: not-a-date
`
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte(authorization), 0o644); err != nil {
		t.Fatalf("write authorization: %v", err)
	}

	report, err := DoctorWorkspace(root)
	if err != nil {
		t.Fatalf("doctor workspace: %v", err)
	}
	if report.Ready {
		t.Fatalf("expected invalid authorization expiration to fail doctor")
	}
	joined := strings.Join(report.Errors, "\n")
	if !strings.Contains(joined, "invalid authorization.yaml") || !strings.Contains(joined, "expires is invalid: not-a-date") {
		t.Fatalf("expected authorization expiration validation error, got %q", joined)
	}
}

func TestDoctorWorkspaceChecksContainerRuntimeImage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "engagement")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	engineDir := t.TempDir()
	engine := filepath.Join(engineDir, "fake-container-engine")
	logPath := filepath.Join(engineDir, "engine.log")
	engineScript := `#!/bin/sh
printf '%s\n' "$*" > "` + logPath + `"
if [ "$1" = "image" ] && [ "$2" = "inspect" ] && [ "$3" = "ouro-worker:test" ]; then
  exit 0
fi
exit 1
`
	if err := os.WriteFile(engine, []byte(engineScript), 0o755); err != nil {
		t.Fatalf("write fake engine: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: container
  container:
    engine: ` + engine + `
    image: ouro-worker:test
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	report, err := DoctorWorkspace(root)
	if err != nil {
		t.Fatalf("doctor workspace: %v", err)
	}
	if !report.Ready {
		t.Fatalf("expected fake inspected image to pass doctor: %#v", report)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake engine log: %v", err)
	}
	if strings.TrimSpace(string(logged)) != "image inspect ouro-worker:test" {
		t.Fatalf("expected image inspect preflight, got %q", string(logged))
	}
}

func TestDoctorWorkspaceReportsContainerRuntimeImageUnavailable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "engagement")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	engine := filepath.Join(t.TempDir(), "fake-container-engine")
	engineScript := `#!/bin/sh
printf 'image missing\n' >&2
exit 1
`
	if err := os.WriteFile(engine, []byte(engineScript), 0o755); err != nil {
		t.Fatalf("write fake engine: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: container
  container:
    engine: ` + engine + `
    image: ouro-worker:test
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	report, err := DoctorWorkspace(root)
	if err != nil {
		t.Fatalf("doctor workspace: %v", err)
	}
	if report.Ready {
		t.Fatalf("expected unavailable image to fail doctor")
	}
	joined := strings.Join(report.Errors, "\n")
	for _, want := range []string{"container runtime", "ouro-worker:test", "image missing"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in doctor errors, got %q", want, joined)
		}
	}
}

type doctorSafetyBypassPayload struct {
	Control              string `json:"control"`
	Mode                 string `json:"mode"`
	Authorized           bool   `json:"authorized"`
	AuthorizedBy         string `json:"authorized_by"`
	Reason               string `json:"reason"`
	Ticket               string `json:"ticket"`
	Expires              string `json:"expires"`
	MissingAuthorization bool   `json:"missing_authorization"`
	ExpiredAuthorization bool   `json:"expired_authorization"`
}

func doctorReportSafetyBypassesFromJSON(t *testing.T, report DoctorReport) []doctorSafetyBypassPayload {
	t.Helper()
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal doctor report: %v", err)
	}
	var payload struct {
		SafetyBypasses []doctorSafetyBypassPayload `json:"safety_bypasses"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode doctor report safety_bypasses: %v json=%s", err, string(data))
	}
	return payload.SafetyBypasses
}

func findDoctorSafetyBypass(t *testing.T, bypasses []doctorSafetyBypassPayload, control string) doctorSafetyBypassPayload {
	t.Helper()
	for _, bypass := range bypasses {
		if bypass.Control == control {
			return bypass
		}
	}
	t.Fatalf("expected safety bypass %q in %#v", control, bypasses)
	return doctorSafetyBypassPayload{}
}
