package toolchain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type DependencyRunner interface {
	CheckDependency(Dependency) (DependencyResult, error)
}

type DependencyMetadataProvider interface {
	FetchDependencyMetadata(context.Context, Manifest, Dependency) (DependencyMetadata, error)
}

type DependencyMetadata struct {
	LatestVersion string `json:"latest_version" yaml:"latest_version"`
	ImageDigest   string `json:"image_digest" yaml:"image_digest"`
	Source        string `json:"source" yaml:"source"`
	Message       string `json:"message" yaml:"message"`
}

type DependencyResult struct {
	InstalledVersion string
	LatestVersion    string
	Missing          bool
	Message          string
}

type DependencyReport struct {
	Name              string
	Kind              string
	Required          bool
	State             HealthState
	InstalledVersion  string
	LatestVersion     string
	Source            string
	ImageDigest       string
	LatestImageDigest string
	MetadataSource    string
	DependsOn         []string
	Message           string
	InstallHint       string
	InstallCommand    []string
}

type DependencyInstallStep struct {
	Name        string
	Kind        string
	Required    bool
	State       HealthState
	Command     []string
	Hint        string
	Source      string
	ImageDigest string
	DependsOn   []string
}

type DefaultDependencyRunner struct {
	Timeout time.Duration
}

type DefaultMetadataProvider struct {
	Timeout time.Duration
	Client  *http.Client
}

func CheckDependencies(manifest Manifest, runner DependencyRunner) []DependencyReport {
	return CheckDependenciesWithMetadata(context.Background(), manifest, runner, nil)
}

func CheckDependenciesWithMetadata(ctx context.Context, manifest Manifest, runner DependencyRunner, metadataProvider DependencyMetadataProvider) []DependencyReport {
	reports := make([]DependencyReport, 0, len(manifest.Dependencies))
	for _, dependency := range manifest.Dependencies {
		result, err := runner.CheckDependency(dependency)
		metadata, metadataErr := fetchDependencyMetadata(ctx, manifest, dependency, metadataProvider)
		report := DependencyReport{
			Name:              strings.TrimSpace(dependency.Name),
			Kind:              dependencyKind(dependency),
			Required:          !dependency.Optional,
			InstalledVersion:  result.InstalledVersion,
			LatestVersion:     firstNonEmptyValue(metadata.LatestVersion, result.LatestVersion, dependency.LatestVersion),
			Source:            firstNonEmptyValue(metadata.Source, dependency.Source),
			ImageDigest:       strings.TrimSpace(dependency.ImageDigest),
			LatestImageDigest: strings.TrimSpace(metadata.ImageDigest),
			MetadataSource:    strings.TrimSpace(dependency.MetadataSource),
			DependsOn:         trimmedStrings(dependency.DependsOn),
			Message:           result.Message,
			InstallHint:       strings.TrimSpace(dependency.InstallHint),
			InstallCommand:    trimmedCommand(dependency.InstallCommand),
		}
		report.Message = appendDependencyMessage(report.Message, strings.TrimSpace(metadata.Message))
		if metadataErr != nil {
			report.Message = appendDependencyMessage(report.Message, "metadata fetch failed: "+metadataErr.Error())
		}
		switch {
		case err != nil:
			report.State = HealthFailed
			report.Message = err.Error()
		case result.Missing:
			report.State = HealthMissing
		case report.LatestVersion != "" && report.InstalledVersion != "" && report.LatestVersion != report.InstalledVersion:
			report.State = HealthUpdateAvailable
		case report.LatestImageDigest != "" && report.ImageDigest != "" && report.LatestImageDigest != report.ImageDigest:
			report.State = HealthUpdateAvailable
			report.Message = appendDependencyMessage(report.Message, "remote metadata image digest differs from pinned image_digest")
		default:
			report.State = HealthReady
		}
		reports = append(reports, report)
	}
	return reports
}

func fetchDependencyMetadata(ctx context.Context, manifest Manifest, dependency Dependency, provider DependencyMetadataProvider) (DependencyMetadata, error) {
	if strings.TrimSpace(dependency.MetadataSource) == "" || provider == nil {
		return DependencyMetadata{}, nil
	}
	return provider.FetchDependencyMetadata(ctx, manifest, dependency)
}

func PlanDependencyInstalls(manifest Manifest, dependencies []DependencyReport) []DependencyInstallStep {
	reportsByDependency := map[string]DependencyReport{}
	reportsByName := map[string]DependencyReport{}
	for _, report := range dependencies {
		reportsByDependency[dependencyReportKey(report.Name, report.Kind)] = report
		reportsByName[strings.TrimSpace(report.Name)] = report
	}
	unresolved := map[string]bool{}
	for _, report := range dependencies {
		if report.State == HealthMissing || report.State == HealthFailed {
			unresolved[strings.TrimSpace(report.Name)] = true
		}
	}

	steps := []DependencyInstallStep{}
	for _, dependency := range manifest.Dependencies {
		report, ok := reportsByDependency[dependencyReportKey(dependency.Name, dependencyKind(dependency))]
		if !ok {
			continue
		}
		if report.State != HealthMissing && report.State != HealthFailed {
			continue
		}
		command := report.InstallCommand
		if len(command) == 0 {
			command = trimmedCommand(dependency.InstallCommand)
		}
		hint := strings.TrimSpace(report.InstallHint)
		if hint == "" {
			hint = strings.TrimSpace(dependency.InstallHint)
		}
		if len(command) == 0 && hint == "" {
			continue
		}
		dependsOn := report.DependsOn
		if len(dependsOn) == 0 {
			dependsOn = dependency.DependsOn
		}
		steps = append(steps, DependencyInstallStep{
			Name:        report.Name,
			Kind:        report.Kind,
			Required:    report.Required,
			State:       report.State,
			Command:     append([]string(nil), command...),
			Hint:        hint,
			Source:      report.Source,
			ImageDigest: report.ImageDigest,
			DependsOn:   unresolvedDependsOn(dependsOn, unresolved, reportsByName),
		})
	}
	return orderDependencyInstallSteps(steps)
}

func ApplyDependencyHealth(health HealthReport, dependencies []DependencyReport) HealthReport {
	for _, dependency := range dependencies {
		if !dependency.Required {
			continue
		}
		if dependency.State == HealthFailed {
			health.State = HealthFailed
			health.Message = fmt.Sprintf("dependency %s failed: %s", dependency.Name, dependency.Message)
			return health
		}
		if dependency.State == HealthMissing {
			health.State = HealthMissing
			health.Message = fmt.Sprintf("dependency %s missing: %s", dependency.Name, dependency.Message)
			if strings.TrimSpace(dependency.InstallHint) != "" {
				health.Message += "; " + dependency.InstallHint
			}
			return health
		}
	}
	return health
}

func (r DefaultDependencyRunner) CheckDependency(dependency Dependency) (DependencyResult, error) {
	switch dependencyKind(dependency) {
	case "binary":
		return r.checkBinary(dependency)
	case "container_image":
		return r.checkContainerImage(dependency)
	default:
		return DependencyResult{}, fmt.Errorf("unsupported dependency kind: %s", dependency.Kind)
	}
}

func (r DefaultDependencyRunner) checkBinary(dependency Dependency) (DependencyResult, error) {
	command := strings.TrimSpace(dependency.Name)
	args := []string{}
	if len(dependency.HealthCommand) > 0 {
		command = strings.TrimSpace(dependency.HealthCommand[0])
		args = dependency.HealthCommand[1:]
	}
	path, err := exec.LookPath(command)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return DependencyResult{Missing: true, Message: err.Error()}, nil
		}
		return DependencyResult{}, err
	}
	if len(dependency.HealthCommand) == 0 {
		return DependencyResult{
			InstalledVersion: firstNonEmptyValue(dependency.Version, "present"),
			Message:          "binary dependency ready",
		}, nil
	}
	return r.runDependencyHealthCommand(path, args)
}

func (r DefaultDependencyRunner) checkContainerImage(dependency Dependency) (DependencyResult, error) {
	if len(dependency.HealthCommand) == 0 {
		return DependencyResult{
			InstalledVersion: firstNonEmptyValue(dependency.Version, dependency.ImageDigest, "declared"),
			LatestVersion:    strings.TrimSpace(dependency.LatestVersion),
			Message:          "container image provenance locked",
		}, nil
	}
	command := strings.TrimSpace(dependency.HealthCommand[0])
	path, err := exec.LookPath(command)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return DependencyResult{Missing: true, Message: err.Error()}, nil
		}
		return DependencyResult{}, err
	}
	result, err := r.runDependencyHealthCommand(path, dependency.HealthCommand[1:])
	if err != nil {
		return result, err
	}
	return verifyContainerImageDigest(dependency, result)
}

func (r DefaultDependencyRunner) runDependencyHealthCommand(path string, args []string) (DependencyResult, error) {
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, args...)
	output, err := cmd.CombinedOutput()
	text := firstNonEmptyLine(string(output))
	if ctx.Err() != nil {
		return DependencyResult{Message: "dependency healthcheck timed out"}, ctx.Err()
	}
	if err != nil {
		if text != "" {
			return DependencyResult{Message: text}, fmt.Errorf("dependency healthcheck failed: %w: %s", err, text)
		}
		return DependencyResult{}, fmt.Errorf("dependency healthcheck failed: %w", err)
	}
	if text == "" {
		text = "present"
	}
	return DependencyResult{
		InstalledVersion: text,
		Message:          "dependency ready",
	}, nil
}

func (p DefaultMetadataProvider) FetchDependencyMetadata(ctx context.Context, manifest Manifest, dependency Dependency) (DependencyMetadata, error) {
	source := strings.TrimSpace(dependency.MetadataSource)
	if source == "" {
		return DependencyMetadata{}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := p.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()

	parsed, err := url.Parse(source)
	if err != nil {
		return DependencyMetadata{}, fmt.Errorf("parse metadata_source: %w", err)
	}
	switch parsed.Scheme {
	case "http", "https":
		return p.fetchHTTPMetadata(ctx, source)
	case "file":
		return readDependencyMetadataFile(filepath.FromSlash(parsed.Path))
	case "":
		return readDependencyMetadataFile(resolveMetadataPath(manifest, source))
	default:
		return DependencyMetadata{}, fmt.Errorf("unsupported metadata_source scheme: %s", parsed.Scheme)
	}
}

func (p DefaultMetadataProvider) fetchHTTPMetadata(ctx context.Context, source string) (DependencyMetadata, error) {
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return DependencyMetadata{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return DependencyMetadata{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return DependencyMetadata{}, fmt.Errorf("metadata fetch returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return DependencyMetadata{}, err
	}
	return decodeDependencyMetadata(data)
}

func readDependencyMetadataFile(path string) (DependencyMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DependencyMetadata{}, err
	}
	return decodeDependencyMetadata(data)
}

func decodeDependencyMetadata(data []byte) (DependencyMetadata, error) {
	var metadata DependencyMetadata
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return DependencyMetadata{}, err
	}
	if strings.TrimSpace(metadata.LatestVersion) == "" && strings.TrimSpace(metadata.ImageDigest) == "" && strings.TrimSpace(metadata.Source) == "" && strings.TrimSpace(metadata.Message) == "" {
		return DependencyMetadata{}, errors.New("metadata document is empty")
	}
	return metadata, nil
}

func resolveMetadataPath(manifest Manifest, source string) string {
	if filepath.IsAbs(source) || strings.TrimSpace(manifest.Path) == "" {
		return source
	}
	return filepath.Join(filepath.Dir(manifest.Path), source)
}

func dependencyKind(dependency Dependency) string {
	if kind := strings.TrimSpace(dependency.Kind); kind != "" {
		return kind
	}
	return "binary"
}

func verifyContainerImageDigest(dependency Dependency, result DependencyResult) (DependencyResult, error) {
	expected := strings.TrimSpace(dependency.ImageDigest)
	if expected == "" {
		return result, nil
	}
	observed := strings.TrimSpace(result.InstalledVersion)
	if !strings.Contains(observed, expected) {
		message := fmt.Sprintf("container image digest mismatch: expected %s, got %s", expected, firstNonEmptyValue(observed, "unknown"))
		result.Message = message
		return result, errors.New(message)
	}
	result.Message = "container image digest verified"
	return result, nil
}

func dependencyReportKey(name string, kind string) string {
	return strings.TrimSpace(name) + "\x00" + strings.TrimSpace(kind)
}

func unresolvedDependsOn(dependsOn []string, unresolved map[string]bool, reportsByName map[string]DependencyReport) []string {
	seen := map[string]bool{}
	filtered := []string{}
	for _, dependencyName := range dependsOn {
		dependencyName = strings.TrimSpace(dependencyName)
		if dependencyName == "" || seen[dependencyName] {
			continue
		}
		report, reportExists := reportsByName[dependencyName]
		if unresolved[dependencyName] || (reportExists && (report.State == HealthMissing || report.State == HealthFailed)) {
			filtered = append(filtered, dependencyName)
			seen[dependencyName] = true
		}
	}
	return filtered
}

func orderDependencyInstallSteps(steps []DependencyInstallStep) []DependencyInstallStep {
	if len(steps) < 2 {
		return steps
	}
	stepNames := map[string]bool{}
	for _, step := range steps {
		stepNames[strings.TrimSpace(step.Name)] = true
	}
	emitted := map[string]bool{}
	ordered := make([]DependencyInstallStep, 0, len(steps))
	for len(ordered) < len(steps) {
		progress := false
		for _, step := range steps {
			name := strings.TrimSpace(step.Name)
			if emitted[name] {
				continue
			}
			if installStepDependenciesSatisfied(step, stepNames, emitted) {
				ordered = append(ordered, step)
				emitted[name] = true
				progress = true
			}
		}
		if progress {
			continue
		}
		for _, step := range steps {
			name := strings.TrimSpace(step.Name)
			if !emitted[name] {
				ordered = append(ordered, step)
				emitted[name] = true
			}
		}
	}
	return ordered
}

func installStepDependenciesSatisfied(step DependencyInstallStep, stepNames map[string]bool, emitted map[string]bool) bool {
	for _, dependsOn := range step.DependsOn {
		dependsOn = strings.TrimSpace(dependsOn)
		if stepNames[dependsOn] && !emitted[dependsOn] {
			return false
		}
	}
	return true
}

func trimmedCommand(command []string) []string {
	if len(command) == 0 {
		return nil
	}
	trimmed := make([]string, 0, len(command))
	for _, part := range command {
		trimmed = append(trimmed, strings.TrimSpace(part))
	}
	return trimmed
}

func trimmedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		trimmed = append(trimmed, value)
	}
	return trimmed
}

func appendDependencyMessage(current string, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == "" {
		return next
	}
	if next == "" {
		return current
	}
	return current + "; " + next
}

func firstNonEmptyValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
