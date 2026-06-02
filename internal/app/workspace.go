package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/toolchain"
	"gopkg.in/yaml.v3"
)

var requiredWorkspaceFiles = []string{
	"mission.yaml",
	"scope.yaml",
	"providers.yaml",
}

var requiredWorkspaceDirs = []string{
	"artifacts",
	"evidence",
	"attempts",
	"findings",
	"reports",
	"replay",
	"runtime",
	"toolchain",
}

var requiredToolchainPacks = []string{
	"web-api-pack",
	"infra-pack",
	"ctf-pack",
}

type DoctorReport struct {
	Ready          bool                 `json:"ready"`
	Errors         []string             `json:"errors"`
	Warnings       []string             `json:"warnings"`
	SafetyBypasses []DoctorSafetyBypass `json:"safety_bypasses"`
}

type DoctorOptions struct {
	RequireSafetyAuthorization bool
	Now                        time.Time
}

type DoctorSafetyBypass struct {
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

type ProviderConfig struct {
	Version   int        `yaml:"version"`
	Providers []Provider `yaml:"providers"`
}

type Provider struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type"`
	Model   string `yaml:"model"`
	Enabled bool   `yaml:"enabled"`
}

type AuthorizationConfig struct {
	Version        int                         `yaml:"version"`
	SafetyBypasses []SafetyBypassAuthorization `yaml:"safety_bypasses"`
}

type SafetyBypassAuthorization struct {
	Control      string `yaml:"control"`
	Mode         string `yaml:"mode"`
	AuthorizedBy string `yaml:"authorized_by"`
	Reason       string `yaml:"reason"`
	Ticket       string `yaml:"ticket,omitempty"`
	Expires      string `yaml:"expires,omitempty"`
}

func InitWorkspace(root string) error {
	if root == "" {
		return fmt.Errorf("workspace path is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create workspace root: %w", err)
	}

	files := map[string]string{
		"mission.yaml":       "version: 1\nmission:\n  name: default\n  goal: \"\"\n",
		"scope.yaml":         "version: 1\nscope:\n  target_policy: enforce\n  targets: []\n  exclusions: []\nexecution:\n  isolation: process\n",
		"authorization.yaml": "version: 1\nsafety_bypasses: []\n",
		"providers.yaml":     defaultProviderConfigYAML,
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := writeFileIfMissing(path, content); err != nil {
			return err
		}
	}

	for _, name := range requiredWorkspaceDirs {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create workspace directory %s: %w", name, err)
		}
	}

	if err := ensureWorkspaceToolchains(root); err != nil {
		return err
	}

	return nil
}

func DoctorWorkspace(root string) (DoctorReport, error) {
	return DoctorWorkspaceWithOptions(root, DoctorOptions{})
}

func DoctorWorkspaceWithOptions(root string, options DoctorOptions) (DoctorReport, error) {
	if root == "" {
		return DoctorReport{Ready: false, Errors: []string{"workspace path is required"}}, nil
	}

	report := DoctorReport{Ready: true}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return DoctorReport{Ready: false, Errors: []string{fmt.Sprintf("workspace root missing: %s", root)}}, nil
		}
		return DoctorReport{}, fmt.Errorf("stat workspace root: %w", err)
	}
	if !info.IsDir() {
		return DoctorReport{Ready: false, Errors: []string{fmt.Sprintf("workspace root is not a directory: %s", root)}}, nil
	}

	for _, name := range requiredWorkspaceFiles {
		path := filepath.Join(root, name)
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				report.Errors = append(report.Errors, fmt.Sprintf("required file missing: %s", name))
				continue
			}
			return DoctorReport{}, fmt.Errorf("stat %s: %w", name, err)
		}
		if info.IsDir() {
			report.Errors = append(report.Errors, fmt.Sprintf("required file is a directory: %s", name))
		}
	}

	for _, name := range requiredWorkspaceDirs {
		path := filepath.Join(root, name)
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				report.Errors = append(report.Errors, fmt.Sprintf("required directory missing: %s", name))
				continue
			}
			return DoctorReport{}, fmt.Errorf("stat %s: %w", name, err)
		}
		if !info.IsDir() {
			report.Errors = append(report.Errors, fmt.Sprintf("required directory is a file: %s", name))
		}
	}

	if err := validateYAMLFile(filepath.Join(root, "mission.yaml")); err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("invalid mission.yaml: %v", err))
	}
	authorization, authorizationFound, authorizationErr := loadWorkspaceAuthorization(filepath.Join(root, "authorization.yaml"))
	if authorizationErr != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("invalid authorization.yaml: %v", authorizationErr))
	}
	if err := validateYAMLFile(filepath.Join(root, "scope.yaml")); err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("invalid scope.yaml: %v", err))
	} else if config, err := LoadScopeConfig(filepath.Join(root, "scope.yaml")); err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("invalid scope.yaml: %v", err))
	} else {
		if err := ValidateScopeConfig(config); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("invalid scope.yaml: %v", err))
		} else {
			now := doctorNow(options)
			report.SafetyBypasses = scopeConfigSafetyBypasses(config, authorization, authorizationFound, now)
			report.Warnings = append(report.Warnings, scopeConfigWarnings(config, authorization, authorizationFound, now)...)
			if options.RequireSafetyAuthorization {
				report.Errors = append(report.Errors, scopeConfigSafetyAuthorizationErrors(config, authorization, authorizationFound, now)...)
			}
			if executionIsolationMode(config.Execution.Isolation) == "container" {
				if err := CheckContainerRuntime(config.Execution.Container); err != nil {
					report.Errors = append(report.Errors, fmt.Sprintf("container runtime unavailable: %v", err))
				}
			}
		}
	}
	if config, err := LoadProviderConfig(filepath.Join(root, "providers.yaml")); err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("invalid providers.yaml: %v", err))
	} else if err := ValidateProviderConfig(config); err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("invalid providers.yaml: %v", err))
	}
	report.Errors = append(report.Errors, validateWorkspaceToolchains(root)...)

	report.Ready = len(report.Errors) == 0
	return report, nil
}

func doctorNow(options DoctorOptions) time.Time {
	if !options.Now.IsZero() {
		return options.Now.UTC()
	}
	return time.Now().UTC()
}

func scopeConfigSafetyBypasses(config ScopeConfig, authorization AuthorizationConfig, authorizationFound bool, now time.Time) []DoctorSafetyBypass {
	var bypasses []DoctorSafetyBypass
	targetPolicy, _ := normalizeScopeTargetPolicy(config.Scope.TargetPolicy)
	if targetPolicy == "disabled" {
		bypasses = append(bypasses, safetyBypassReport(authorization, authorizationFound, "scope.target_policy", "disabled", now))
	}
	if executionIsolationMode(config.Execution.Isolation) == "disabled" {
		bypasses = append(bypasses, safetyBypassReport(authorization, authorizationFound, "execution.isolation", "disabled", now))
	}
	return bypasses
}

func safetyBypassReport(config AuthorizationConfig, found bool, control string, mode string, now time.Time) DoctorSafetyBypass {
	report := DoctorSafetyBypass{
		Control: control,
		Mode:    mode,
	}
	authorization, ok := findSafetyBypassAuthorization(config, control, mode)
	if !found || !ok {
		report.MissingAuthorization = true
		return report
	}
	report.AuthorizedBy = strings.TrimSpace(authorization.AuthorizedBy)
	report.Reason = strings.TrimSpace(authorization.Reason)
	report.Ticket = strings.TrimSpace(authorization.Ticket)
	report.Expires = strings.TrimSpace(authorization.Expires)
	if safetyBypassAuthorizationExpired(authorization, now) {
		report.ExpiredAuthorization = true
		return report
	}
	report.Authorized = true
	return report
}

func scopeConfigWarnings(config ScopeConfig, authorization AuthorizationConfig, authorizationFound bool, now time.Time) []string {
	var warnings []string
	targetPolicy, _ := normalizeScopeTargetPolicy(config.Scope.TargetPolicy)
	if targetPolicy == "disabled" {
		warnings = append(warnings, "scope.target_policy is disabled; command-runner target boundary and exclusion checks are bypassed")
		warnings = append(warnings, safetyBypassAuthorizationWarning(authorization, authorizationFound, "scope.target_policy", "disabled", now))
	}
	if executionIsolationMode(config.Execution.Isolation) == "disabled" {
		warnings = append(warnings, "execution.isolation is disabled; command workers inherit the host environment and run from the workspace root")
		warnings = append(warnings, safetyBypassAuthorizationWarning(authorization, authorizationFound, "execution.isolation", "disabled", now))
	}
	return warnings
}

func scopeConfigSafetyAuthorizationErrors(config ScopeConfig, authorization AuthorizationConfig, authorizationFound bool, now time.Time) []string {
	var errors []string
	targetPolicy, _ := normalizeScopeTargetPolicy(config.Scope.TargetPolicy)
	if targetPolicy == "disabled" {
		errors = append(errors, safetyBypassAuthorizationErrors(authorization, authorizationFound, "scope.target_policy", "disabled", now)...)
	}
	if executionIsolationMode(config.Execution.Isolation) == "disabled" {
		errors = append(errors, safetyBypassAuthorizationErrors(authorization, authorizationFound, "execution.isolation", "disabled", now)...)
	}
	return errors
}

func safetyBypassAuthorizationErrors(config AuthorizationConfig, found bool, control string, mode string, now time.Time) []string {
	if hasCurrentSafetyBypassAuthorization(config, found, control, mode, now) {
		return nil
	}
	authorization, ok := findSafetyBypassAuthorization(config, control, mode)
	if found && ok && safetyBypassAuthorizationExpired(authorization, now) {
		return []string{fmt.Sprintf("expired required safety bypass authorization for %s %s; renew authorization.yaml safety_bypasses expires", control, mode)}
	}
	return []string{fmt.Sprintf("missing required safety bypass authorization for %s %s; add authorization.yaml safety_bypasses entry with authorized_by and reason", control, mode)}
}

func safetyBypassAuthorizationWarning(config AuthorizationConfig, found bool, control string, mode string, now time.Time) string {
	authorization, ok := findSafetyBypassAuthorization(config, control, mode)
	if !found || !ok {
		return fmt.Sprintf("missing safety bypass authorization for %s %s; add authorization.yaml safety_bypasses entry with authorized_by and reason", control, mode)
	}
	parts := []string{
		fmt.Sprintf("%s %s authorized by %s", control, mode, strings.TrimSpace(authorization.AuthorizedBy)),
		"reason: " + strings.TrimSpace(authorization.Reason),
	}
	if ticket := strings.TrimSpace(authorization.Ticket); ticket != "" {
		parts = append(parts, "ticket: "+ticket)
	}
	if expires := strings.TrimSpace(authorization.Expires); expires != "" {
		parts = append(parts, "expires: "+expires)
	}
	if safetyBypassAuthorizationExpired(authorization, now) {
		return fmt.Sprintf("expired safety bypass authorization for %s %s; expired on %s; %s", control, mode, strings.TrimSpace(authorization.Expires), strings.Join(parts, "; "))
	}
	return strings.Join(parts, "; ")
}

func hasCurrentSafetyBypassAuthorization(config AuthorizationConfig, found bool, control string, mode string, now time.Time) bool {
	if !found {
		return false
	}
	authorization, ok := findSafetyBypassAuthorization(config, control, mode)
	return ok && !safetyBypassAuthorizationExpired(authorization, now)
}

func safetyBypassAuthorizationExpired(authorization SafetyBypassAuthorization, now time.Time) bool {
	expires := strings.TrimSpace(authorization.Expires)
	if expires == "" {
		return false
	}
	cutoff, err := parseSafetyBypassExpirationCutoff(expires)
	if err != nil {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return !now.UTC().Before(cutoff)
}

func findSafetyBypassAuthorization(config AuthorizationConfig, control string, mode string) (SafetyBypassAuthorization, bool) {
	for _, authorization := range config.SafetyBypasses {
		if strings.EqualFold(strings.TrimSpace(authorization.Control), control) &&
			strings.EqualFold(strings.TrimSpace(authorization.Mode), mode) {
			return authorization, true
		}
	}
	return SafetyBypassAuthorization{}, false
}

func loadWorkspaceAuthorization(path string) (AuthorizationConfig, bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return AuthorizationConfig{}, false, nil
		}
		return AuthorizationConfig{}, false, err
	}
	config, err := LoadAuthorizationConfig(path)
	if err != nil {
		return AuthorizationConfig{}, true, err
	}
	if err := ValidateAuthorizationConfig(config); err != nil {
		return AuthorizationConfig{}, true, err
	}
	return config, true, nil
}

func LoadAuthorizationConfig(path string) (AuthorizationConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AuthorizationConfig{}, err
	}
	var config AuthorizationConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return AuthorizationConfig{}, err
	}
	return config, nil
}

func ValidateAuthorizationConfig(config AuthorizationConfig) error {
	if config.Version == 0 {
		return errors.New("version is required")
	}
	for i, authorization := range config.SafetyBypasses {
		control := strings.TrimSpace(authorization.Control)
		mode := strings.TrimSpace(authorization.Mode)
		if control == "" {
			return fmt.Errorf("safety_bypasses[%d] control is required", i)
		}
		if control != "scope.target_policy" && control != "execution.isolation" {
			return fmt.Errorf("safety_bypasses[%d] control is unsupported: %s", i, control)
		}
		if mode == "" {
			return fmt.Errorf("safety_bypasses[%d] mode is required", i)
		}
		if mode != "disabled" {
			return fmt.Errorf("safety_bypasses[%d] mode is unsupported: %s", i, mode)
		}
		if strings.TrimSpace(authorization.AuthorizedBy) == "" {
			return fmt.Errorf("safety_bypasses[%d] authorized_by is required", i)
		}
		if strings.TrimSpace(authorization.Reason) == "" {
			return fmt.Errorf("safety_bypasses[%d] reason is required", i)
		}
		if expires := strings.TrimSpace(authorization.Expires); expires != "" {
			if _, err := parseSafetyBypassExpirationCutoff(expires); err != nil {
				return fmt.Errorf("safety_bypasses[%d] expires is invalid: %s", i, expires)
			}
		}
	}
	return nil
}

func parseSafetyBypassExpirationCutoff(expires string) (time.Time, error) {
	expires = strings.TrimSpace(expires)
	if expires == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse("2006-01-02", expires); err == nil {
		return parsed.AddDate(0, 0, 1).UTC(), nil
	}
	parsed, err := time.Parse(time.RFC3339, expires)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func LoadProviderConfig(path string) (ProviderConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProviderConfig{}, err
	}
	var config ProviderConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return ProviderConfig{}, err
	}
	return config, nil
}

func ValidateProviderConfig(config ProviderConfig) error {
	if config.Version == 0 {
		return errors.New("version is required")
	}
	if len(config.Providers) == 0 {
		return errors.New("at least one provider is required")
	}

	enabled := 0
	for i, provider := range config.Providers {
		if strings.TrimSpace(provider.Name) == "" {
			return fmt.Errorf("provider %d name is required", i)
		}
		if strings.TrimSpace(provider.Type) == "" {
			return fmt.Errorf("provider %s type is required", provider.Name)
		}
		if strings.TrimSpace(provider.Model) == "" {
			return fmt.Errorf("provider %s model is required", provider.Name)
		}
		if provider.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		return errors.New("at least one provider must be enabled")
	}
	return nil
}

func EnabledProvider(config ProviderConfig) (Provider, bool) {
	for _, provider := range config.Providers {
		if provider.Enabled {
			return provider, true
		}
	}
	return Provider{}, false
}

func writeFileIfMissing(path string, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func ensureWorkspaceToolchains(root string) error {
	for name, content := range workspaceToolchainManifestYAML {
		dir := filepath.Join(root, "toolchain", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create toolchain manifest directory %s: %w", name, err)
		}
		if err := writeFileIfMissing(filepath.Join(dir, "manifest.yaml"), content); err != nil {
			return err
		}
	}
	return nil
}

func validateWorkspaceToolchains(root string) []string {
	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		return []string{fmt.Sprintf("invalid toolchain manifests: %v", err)}
	}

	byName := map[string]bool{}
	for _, manifest := range manifests {
		byName[manifest.Name] = true
	}

	var errors []string
	for _, name := range requiredToolchainPacks {
		if !byName[name] {
			errors = append(errors, fmt.Sprintf("required toolchain manifest missing: %s", name))
		}
	}
	return errors
}

func validateYAMLFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var decoded any
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		return err
	}
	return nil
}

const defaultProviderConfigYAML = `version: 1
providers:
  - name: offline
    type: offline
    model: offline-model
    enabled: true
`

var workspaceToolchainManifestYAML = map[string]string{
	"web-api-pack": `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
  - web.route_enumeration
  - web.auth_state_tracking
  - web.candidate_generation
  - evidence.collection
runner: native
parser: web_api_parser
healthcheck: built_in
`,
	"infra-pack": `name: infra-pack
version: "0.1.0"
capabilities:
  - infra.host_discovery
  - infra.service_enumeration
  - infra.identity_graph
  - infra.path_hypothesis
  - evidence.collection
runner: native
parser: infra_parser
healthcheck: built_in
`,
	"ctf-pack": `name: ctf-pack
version: "0.1.0"
capabilities:
  - ctf.challenge_classification
  - ctf.artifact_analysis
  - ctf.fast_hypothesis
  - ctf.candidate_verification
  - ctf.result_proof
runner: native
parser: ctf_parser
healthcheck: built_in
`,
}
