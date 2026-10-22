package toolchain

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHealthReady(t *testing.T) {
	report := CheckHealth(validManifest(), staticHealthRunner{
		result: HealthResult{InstalledVersion: "0.1.0"},
	})

	if report.State != HealthReady {
		t.Fatalf("expected ready, got %s", report.State)
	}
}

func TestHealthMissing(t *testing.T) {
	report := CheckHealth(validManifest(), staticHealthRunner{
		result: HealthResult{Missing: true},
	})

	if report.State != HealthMissing {
		t.Fatalf("expected missing, got %s", report.State)
	}
}

func TestHealthFailed(t *testing.T) {
	report := CheckHealth(validManifest(), staticHealthRunner{
		err: errors.New("healthcheck failed"),
	})

	if report.State != HealthFailed {
		t.Fatalf("expected failed, got %s", report.State)
	}
	if report.Message == "" {
		t.Fatalf("expected failure message")
	}
}

func TestHealthUpdateAvailable(t *testing.T) {
	report := CheckHealth(validManifest(), staticHealthRunner{
		result: HealthResult{InstalledVersion: "0.0.9", LatestVersion: "0.1.0"},
	})

	if report.State != HealthUpdateAvailable {
		t.Fatalf("expected update available, got %s", report.State)
	}
}

func TestDefaultHealthRunnerExecutesExternalProbe(t *testing.T) {
	skipOnWindowsProbe(t)
	probe := filepath.Join(t.TempDir(), "probe")
	if err := os.WriteFile(probe, []byte("#!/bin/sh\nprintf 'probe-1.2.3\\n'\n"), 0o755); err != nil {
		t.Fatalf("write probe: %v", err)
	}
	manifest := validManifest()
	manifest.Healthcheck = "exec"
	manifest.HealthCommand = []string{probe, "--version"}

	report := CheckHealth(manifest, DefaultHealthRunner{})

	if report.State != HealthReady {
		t.Fatalf("expected ready external probe, got %#v", report)
	}
	if report.InstalledVersion != "probe-1.2.3" {
		t.Fatalf("expected probe output as installed version, got %#v", report)
	}
}

func TestDefaultHealthRunnerReportsMissingExternalProbe(t *testing.T) {
	manifest := validManifest()
	manifest.Healthcheck = "exec"
	manifest.HealthCommand = []string{filepath.Join(t.TempDir(), "missing-probe")}

	report := CheckHealth(manifest, DefaultHealthRunner{})

	if report.State != HealthMissing {
		t.Fatalf("expected missing external probe, got %#v", report)
	}
}

type staticHealthRunner struct {
	result HealthResult
	err    error
}

func (r staticHealthRunner) Check(manifest Manifest) (HealthResult, error) {
	return r.result, r.err
}

func validManifest() Manifest {
	return Manifest{
		Name:         "web-api-pack",
		Version:      "0.1.0",
		Capabilities: []string{"web.discovery"},
		Runner:       "native",
		Parser:       "web_parser",
		Healthcheck:  "built_in",
	}
}
