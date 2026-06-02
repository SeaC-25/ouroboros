package toolchain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type HealthState string

const (
	HealthReady           HealthState = "ready"
	HealthMissing         HealthState = "missing"
	HealthFailed          HealthState = "failed"
	HealthUpdateAvailable HealthState = "update_available"
)

type HealthRunner interface {
	Check(manifest Manifest) (HealthResult, error)
}

type HealthResult struct {
	InstalledVersion string
	LatestVersion    string
	Missing          bool
	Message          string
}

type HealthReport struct {
	ManifestName     string
	State            HealthState
	InstalledVersion string
	LatestVersion    string
	Message          string
}

type DefaultHealthRunner struct {
	Timeout time.Duration
}

func (r DefaultHealthRunner) Check(manifest Manifest) (HealthResult, error) {
	switch strings.TrimSpace(manifest.Healthcheck) {
	case "built_in":
		return HealthResult{
			InstalledVersion: manifest.Version,
			LatestVersion:    manifest.Version,
			Message:          "built_in manifest ready",
		}, nil
	case "exec":
		return r.checkExec(manifest)
	default:
		return HealthResult{}, fmt.Errorf("unsupported healthcheck: %s", manifest.Healthcheck)
	}
}

func (r DefaultHealthRunner) checkExec(manifest Manifest) (HealthResult, error) {
	if len(manifest.HealthCommand) == 0 {
		return HealthResult{}, errors.New("exec healthcheck requires health_command")
	}
	command := strings.TrimSpace(manifest.HealthCommand[0])
	path, err := exec.LookPath(command)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return HealthResult{Missing: true, Message: err.Error()}, nil
		}
		return HealthResult{}, err
	}

	timeout := r.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, manifest.HealthCommand[1:]...)
	output, err := cmd.CombinedOutput()
	text := firstNonEmptyLine(string(output))
	if ctx.Err() != nil {
		return HealthResult{Message: "healthcheck timed out"}, ctx.Err()
	}
	if err != nil {
		if text != "" {
			return HealthResult{Message: text}, fmt.Errorf("healthcheck command failed: %w: %s", err, text)
		}
		return HealthResult{}, fmt.Errorf("healthcheck command failed: %w", err)
	}
	if text == "" {
		text = "ok"
	}
	return HealthResult{
		InstalledVersion: text,
		Message:          "exec healthcheck ready",
	}, nil
}

func firstNonEmptyLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func CheckHealth(manifest Manifest, runner HealthRunner) HealthReport {
	result, err := runner.Check(manifest)
	report := HealthReport{
		ManifestName:     manifest.Name,
		InstalledVersion: result.InstalledVersion,
		LatestVersion:    result.LatestVersion,
		Message:          result.Message,
	}
	if err != nil {
		report.State = HealthFailed
		report.Message = err.Error()
		return report
	}
	if result.Missing {
		report.State = HealthMissing
		return report
	}
	if result.LatestVersion != "" && result.InstalledVersion != "" && result.LatestVersion != result.InstalledVersion {
		report.State = HealthUpdateAvailable
		return report
	}
	report.State = HealthReady
	return report
}
