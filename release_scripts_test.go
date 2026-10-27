package ouroboros_test

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/app"
)

func TestMain(m *testing.M) {
	if runtime.GOOS == "windows" {
		// These release / install / smoke / production-acceptance / completion-audit
		// tests exercise POSIX shell scripts (scripts/*.sh) that reset PATH to a
		// trusted /usr/local/sbin:... path and rely on readlink/symlink semantics.
		// They are Linux-only and cannot run on Windows, so skip the whole package
		// rather than falsify their assertions.
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestReleaseScriptCreatesArchiveAndChecksums(t *testing.T) {
	dist := t.TempDir()
	version := "0.1.0-test"
	releaseTestBundle := "ouro-release-test-tool"
	writeReleaseScriptManagedBundleFixture(t, releaseTestBundle)
	env := append(os.Environ(),
		"GOPROXY=https://goproxy.cn,direct",
		"DIST_DIR="+dist,
		"VERSION="+version,
		"GOOS="+runtime.GOOS,
		"GOARCH="+runtime.GOARCH,
	)
	cmd := exec.Command("sh", "./scripts/release.sh")
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("release script failed: %v\n%s", err, output)
	}

	archiveName := "ouro_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	archivePath := filepath.Join(dist, archiveName)
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("expected release archive %s: %v\noutput:\n%s", archivePath, err, output)
	}
	sumBytes, err := os.ReadFile(filepath.Join(dist, "SHA256SUMS"))
	if err != nil {
		t.Fatalf("read SHA256SUMS: %v", err)
	}
	sumText := string(sumBytes)
	if !strings.Contains(sumText, archiveName) {
		t.Fatalf("expected SHA256SUMS to include archive name %q, got %q", archiveName, sumText)
	}

	members := tarGzMembers(t, archivePath)
	for _, want := range []string{
		"ouro_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + "/ouro",
		"ouro_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + "/README.md",
		"ouro_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + "/install.sh",
		"ouro_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + "/toolchains/managed-bundles/" + releaseTestBundle + "/bundle.yaml",
		"ouro_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + "/toolchains/managed-bundles/" + releaseTestBundle + "/payload/" + releaseTestBundle,
	} {
		if !members[want] {
			t.Fatalf("expected archive member %q in %#v", want, members)
		}
	}
}

func TestReleaseScriptPackagesBuiltInManagedRuleBundle(t *testing.T) {
	dist := t.TempDir()
	version := "0.1.0-rule-bundle-test"
	env := append(os.Environ(),
		"GOPROXY=https://goproxy.cn,direct",
		"DIST_DIR="+dist,
		"VERSION="+version,
		"GOOS="+runtime.GOOS,
		"GOARCH="+runtime.GOARCH,
	)
	cmd := exec.Command("sh", "./scripts/release.sh")
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("release script failed: %v\n%s", err, output)
	}

	packageName := "ouro_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH
	archivePath := filepath.Join(dist, packageName+".tar.gz")
	members := tarGzMembers(t, archivePath)
	for _, want := range []string{
		packageName + "/toolchains/managed-bundles/nuclei-templates/bundle.yaml",
		packageName + "/toolchains/managed-bundles/nuclei-templates/payload/http/ouro-default-healthcheck.yaml",
	} {
		if !members[want] {
			t.Fatalf("expected built-in managed rule bundle member %q in %#v\noutput:\n%s", want, members, output)
		}
	}
	releaseReadme := tarGzFileContent(t, archivePath, packageName+"/README.md")
	for _, want := range []string{
		"offline corpus metadata",
		"corpus_status",
		"corpus_file_count",
		"corpus_digest_status",
		"corpus_digest_scope",
		"without downloading, syncing, or executing it",
	} {
		if !strings.Contains(releaseReadme, want) {
			t.Fatalf("expected release README to contain %q, got:\n%s", want, releaseReadme)
		}
	}

	prefix := filepath.Join(t.TempDir(), "prefix")
	installCmd := exec.Command("sh", "./scripts/install.sh", archivePath)
	installCmd.Env = append(os.Environ(), "PREFIX="+prefix)
	installOutput, err := installCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install release archive failed: %v\n%s", err, installOutput)
	}

	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(prefix, "share", "ouroboros")
	result, err := app.ExecuteReleaseManagedToolBundleInstall(root, releaseRoot, "web-api-pack", "nuclei-templates", app.ReleaseManagedToolBundleInstallOptions{Execute: true})
	if err != nil {
		t.Fatalf("install built-in managed rule bundle from release: %v", err)
	}
	if result.Kind != "rule_bundle" || result.Mode != "workspace-copy-link" {
		t.Fatalf("unexpected managed rule bundle install result: %#v", result)
	}
	installedRule := filepath.Join(root, "toolchain", "managed-bundles", "nuclei-templates", "payload", "http", "ouro-default-healthcheck.yaml")
	if _, err := os.Stat(installedRule); err != nil {
		t.Fatalf("expected installed managed rule payload %s: %v", installedRule, err)
	}
	linkInfo, err := os.Lstat(filepath.Join(root, "toolchain", "rules", "nuclei-templates"))
	if err != nil {
		t.Fatalf("expected managed rule bundle link: %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected managed rule bundle install to create symlink, mode=%v", linkInfo.Mode())
	}
}

func TestInstallScriptInstallsFromLocalArchiveWithoutNetwork(t *testing.T) {
	releaseRoot := t.TempDir()
	payloadDir := filepath.Join(releaseRoot, "ouro_0.1.0-test_linux_amd64")
	if err := os.MkdirAll(payloadDir, 0o755); err != nil {
		t.Fatalf("mkdir payload: %v", err)
	}
	binary := filepath.Join(payloadDir, "ouro")
	if err := os.WriteFile(binary, []byte("#!/usr/bin/env sh\nprintf 'fake ouro\\n'\n"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	bundleDir := filepath.Join(payloadDir, "toolchains", "managed-bundles", "httpx")
	if err := os.MkdirAll(filepath.Join(bundleDir, "payload"), 0o755); err != nil {
		t.Fatalf("mkdir fake bundle payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "bundle.yaml"), []byte("name: httpx\nkind: binary\nversion: \"1.3.0\"\nsource: test\npayload: payload/httpx\nsha256: 0000000000000000000000000000000000000000000000000000000000000000\n"), 0o644); err != nil {
		t.Fatalf("write fake bundle manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "payload", "httpx"), []byte("#!/usr/bin/env sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake bundle payload: %v", err)
	}
	archivePath := filepath.Join(releaseRoot, "ouro_0.1.0-test_linux_amd64.tar.gz")
	createTarGz(
		t,
		archivePath,
		releaseRoot,
		filepath.Base(payloadDir)+"/ouro",
		filepath.Base(payloadDir)+"/toolchains/managed-bundles/httpx/bundle.yaml",
		filepath.Base(payloadDir)+"/toolchains/managed-bundles/httpx/payload/httpx",
	)
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive for checksum: %v", err)
	}
	archiveDigest := sha256.Sum256(archiveBytes)
	if err := os.WriteFile(filepath.Join(releaseRoot, "SHA256SUMS"), []byte(hex.EncodeToString(archiveDigest[:])+"  "+filepath.Base(archivePath)+"\n"), 0o644); err != nil {
		t.Fatalf("write SHA256SUMS: %v", err)
	}

	prefix := filepath.Join(t.TempDir(), "prefix")
	cmd := exec.Command("sh", "./scripts/install.sh", archivePath)
	cmd.Env = append(os.Environ(), "PREFIX="+prefix)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install script failed: %v\n%s", err, output)
	}
	installed := filepath.Join(prefix, "bin", "ouro")
	info, err := os.Stat(installed)
	if err != nil {
		t.Fatalf("expected installed binary %s: %v\noutput:\n%s", installed, err, output)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("expected installed binary to be executable, mode=%s", info.Mode())
	}
	installedBundle := filepath.Join(prefix, "share", "ouroboros", "toolchains", "managed-bundles", "httpx", "bundle.yaml")
	if _, err := os.Stat(installedBundle); err != nil {
		t.Fatalf("expected installed managed bundle metadata %s: %v\noutput:\n%s", installedBundle, err, output)
	}
}

func TestInstallScriptRejectsArchiveChecksumMismatch(t *testing.T) {
	releaseRoot, archivePath := createInstallScriptArchive(t)
	wrongDigest := strings.Repeat("0", sha256.Size*2)
	if err := os.WriteFile(filepath.Join(releaseRoot, "SHA256SUMS"), []byte(wrongDigest+"  "+filepath.Base(archivePath)+"\n"), 0o644); err != nil {
		t.Fatalf("write SHA256SUMS: %v", err)
	}

	prefix := filepath.Join(t.TempDir(), "prefix")
	cmd := exec.Command("sh", "./scripts/install.sh", archivePath)
	cmd.Env = append(os.Environ(), "PREFIX="+prefix)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected checksum mismatch to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch error, got:\n%s", output)
	}
	if _, statErr := os.Stat(filepath.Join(prefix, "bin", "ouro")); !os.IsNotExist(statErr) {
		t.Fatalf("checksum mismatch must not install binary, stat err=%v output:\n%s", statErr, output)
	}
}

func TestInstallScriptRejectsArchiveChecksumMissingEntry(t *testing.T) {
	releaseRoot, archivePath := createInstallScriptArchive(t)
	digest := strings.Repeat("1", sha256.Size*2)
	if err := os.WriteFile(filepath.Join(releaseRoot, "SHA256SUMS"), []byte(digest+"  other_archive.tar.gz\n"), 0o644); err != nil {
		t.Fatalf("write SHA256SUMS: %v", err)
	}

	prefix := filepath.Join(t.TempDir(), "prefix")
	cmd := exec.Command("sh", "./scripts/install.sh", archivePath)
	cmd.Env = append(os.Environ(), "PREFIX="+prefix)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected missing checksum entry to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "checksum entry not found") {
		t.Fatalf("expected missing checksum entry error, got:\n%s", output)
	}
	if _, statErr := os.Stat(filepath.Join(prefix, "bin", "ouro")); !os.IsNotExist(statErr) {
		t.Fatalf("missing checksum entry must not install binary, stat err=%v output:\n%s", statErr, output)
	}
}

func TestInstallScriptRejectsArchiveChecksumWithoutChecksumTool(t *testing.T) {
	releaseRoot, archivePath := createInstallScriptArchive(t)
	digest := strings.Repeat("1", sha256.Size*2)
	if err := os.WriteFile(filepath.Join(releaseRoot, "SHA256SUMS"), []byte(digest+"  "+filepath.Base(archivePath)+"\n"), 0o644); err != nil {
		t.Fatalf("write SHA256SUMS: %v", err)
	}

	toolBin := t.TempDir()
	for _, name := range []string{"awk", "basename", "dirname"} {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		if err := os.Symlink(path, filepath.Join(toolBin, name)); err != nil {
			t.Fatalf("link %s: %v", name, err)
		}
	}

	prefix := filepath.Join(t.TempDir(), "prefix")
	cmd := exec.Command("/bin/sh", "./scripts/install.sh", archivePath)
	cmd.Env = append(os.Environ(), "PREFIX="+prefix, "PATH="+toolBin)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected missing checksum tool to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "sha256sum or shasum is required to verify checksums") {
		t.Fatalf("expected missing checksum tool error, got:\n%s", output)
	}
	if _, statErr := os.Stat(filepath.Join(prefix, "bin", "ouro")); !os.IsNotExist(statErr) {
		t.Fatalf("missing checksum tool must not install binary, stat err=%v output:\n%s", statErr, output)
	}
}

func TestReleaseMakefileAndCIEntrypoints(t *testing.T) {
	makefileBytes, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(makefileBytes)
	for _, want := range []string{
		".PHONY:",
		"test",
		"build",
		"doctor:",
		"doctor-strict:",
		"smoke-ide:",
		"smoke-workflow:",
		"smoke-direct-use:",
		"smoke-ai-provider:",
		"smoke-workflow-closure:",
		"smoke-release-managed:",
		"smoke-managed-payloads:",
		"smoke-production-acceptance:",
		"smoke-final-evidence-fixture:",
		"audit-completion:",
		"release:",
		"clean:",
		"--require-safety-authorization",
		"./scripts/smoke_ide.sh",
		"./scripts/smoke_workflow.sh",
		"./scripts/smoke_direct_use.sh",
		"./scripts/smoke_ai_provider.sh",
		"./scripts/smoke_workflow_closure.sh",
		"./scripts/smoke_release_managed.sh",
		"./scripts/smoke_managed_payloads.sh",
		"./scripts/smoke_production_acceptance.sh",
		"./scripts/smoke_final_evidence_fixture.sh",
		"./scripts/audit_completion.sh",
		"./scripts/release.sh",
	} {
		if !strings.Contains(makefile, want) {
			t.Fatalf("expected Makefile to contain %q, got:\n%s", want, makefile)
		}
	}

	workflowBytes, err := os.ReadFile(filepath.Join(".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	workflow := string(workflowBytes)
	for _, want := range []string{"make test", "make doctor-strict", "make build", "make release"} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("expected CI workflow to contain %q, got:\n%s", want, workflow)
		}
	}
}

func TestFinalEvidenceFixtureSmokeScriptExercisesProductionAcceptanceAndCompletionAudit(t *testing.T) {
	cmd := exec.Command("sh", "./scripts/smoke_final_evidence_fixture.sh")
	cmd.Env = append(os.Environ(),
		"GOPROXY=https://goproxy.cn,direct",
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
		"GOTMPDIR="+t.TempDir(),
		"TMPDIR="+t.TempDir(),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("final evidence fixture smoke script failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, want := range []string{
		"final-evidence-fixture: root ",
		"final-evidence-fixture: mode=fixture-only claim_100_ready=false",
		"production-acceptance-smoke: ok",
		"completion-audit: ok",
		"completion-status: status=ready-for-final-review",
		"final_review_status=fixture_evidence_not_acceptable",
		"claim_100_ready=false",
		"fixture_evidence_detected=true",
		"final-evidence-fixture: completion-accept fixture-rejected",
		"final-evidence-fixture: completion-log-dir ",
		"final-evidence-fixture: ok",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected final evidence fixture smoke output %q, got:\n%s", want, text)
		}
	}
	if strings.Contains(text, "claim_100_ready=true") {
		t.Fatalf("fixture smoke must not claim final 100 readiness:\n%s", text)
	}
}

func TestFinalEvidenceRunbookDocumentsRequiredFields(t *testing.T) {
	for _, docPath := range []string{"README.md", filepath.Join("docs", "development.md")} {
		docBytes, err := os.ReadFile(docPath)
		if err != nil {
			t.Fatalf("read %s: %v", docPath, err)
		}
		doc := string(docBytes)
		for _, want := range []string{
			"Final Evidence Runbook",
			"absolute paths",
			"must not be symlinks",
			"Payload paths declared in the inventory must also be absolute and must not be symlinks",
			"payload-inventory.env",
			"deployment-policy.env",
			"completion-attestation.env",
			"schema=ouroboros.completion_audit.attestation.v1",
			"attestation_status=approved",
			"production_summary_sha256=",
			"production_evidence=operator_owned",
			"fixture_evidence_allowed=false",
			"scope_reviewed=true",
			"safety_boundaries_reviewed=true",
			"release_managed_reviewed=true",
			"managed_payloads_reviewed=true",
			"workflow_closure_reviewed=true",
			"documentation_reviewed=true",
			"OURO_COMPLETION_AUDIT_LOG_DIR=/abs/path/completion-audit-logs",
			"inventory_sha256",
			"policy_sha256",
			"trusted shell/path metadata",
			"stage script SHA-256s",
			"stage workspace paths",
			"stage workspace artifacts",
			"ide-smoke: ok",
			"release-managed-smoke: ok",
			"managed-payloads-smoke: ok",
			"workflow-closure-smoke: ok",
			"make smoke-production-acceptance",
			"make audit-completion",
			"completion-final-acceptance.txt",
			"completion-accept",
			"fixture_evidence_accepted=false",
			"real_operator_evidence_reviewed=true",
			"claim_100_ready=true",
			"claim_100_after_real_operator_evidence_review",
			"must not be symlinks",
		} {
			if !strings.Contains(doc, want) {
				t.Fatalf("expected %s to contain %q", docPath, want)
			}
		}
	}
}

func TestFinalEvidenceRunbookBashBlocksAreCopyPasteSafe(t *testing.T) {
	for _, docPath := range []string{"README.md", filepath.Join("docs", "development.md")} {
		docBytes, err := os.ReadFile(docPath)
		if err != nil {
			t.Fatalf("read %s: %v", docPath, err)
		}
		inBash := false
		for i, line := range strings.Split(string(docBytes), "\n") {
			switch strings.TrimSpace(line) {
			case "```bash":
				inBash = true
				continue
			case "```":
				inBash = false
				continue
			}
			if !inBash {
				continue
			}
			for _, prosePrefix := range []string{
				"The production ",
				"The audit ",
				"Do not ",
				"Completion audit ",
			} {
				if strings.HasPrefix(line, prosePrefix) {
					t.Fatalf("%s:%d has prose inside a bash block: %s", docPath, i+1, line)
				}
			}
		}
		if inBash {
			t.Fatalf("%s has an unclosed bash block", docPath)
		}
	}
}

func TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck(t *testing.T) {
	for _, docPath := range []string{"README.md", filepath.Join("docs", "development.md")} {
		docBytes, err := os.ReadFile(docPath)
		if err != nil {
			t.Fatalf("read %s: %v", docPath, err)
		}
		blocks := finalEvidenceRunbookBashBlocksForTest(string(docBytes))
		if len(blocks) != 3 {
			t.Fatalf("expected %s final evidence runbook to contain 3 bash blocks, got %d", docPath, len(blocks))
		}
		for i, block := range blocks {
			cmd := exec.Command("sh", "-n")
			cmd.Stdin = strings.NewReader(block)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s final evidence bash block %d failed sh -n: %v\n%s\nblock:\n%s", docPath, i+1, err, output, block)
			}
		}
	}
}

func TestDocumentationBashBlocksPassShellSyntaxCheck(t *testing.T) {
	for _, docPath := range []string{"README.md", filepath.Join("docs", "development.md")} {
		docBytes, err := os.ReadFile(docPath)
		if err != nil {
			t.Fatalf("read %s: %v", docPath, err)
		}
		blocks := bashBlocksForTest(string(docBytes))
		if len(blocks) == 0 {
			t.Fatalf("expected %s to contain bash blocks", docPath)
		}
		for i, block := range blocks {
			cmd := exec.Command("sh", "-n")
			cmd.Stdin = strings.NewReader(block)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s bash block %d failed sh -n: %v\n%s\nblock:\n%s", docPath, i+1, err, output, block)
			}
		}
	}
}

func TestFinalEvidenceRunbookDocumentsStageProvenanceArtifactBindings(t *testing.T) {
	for _, docPath := range []string{"README.md", filepath.Join("docs", "development.md")} {
		docBytes, err := os.ReadFile(docPath)
		if err != nil {
			t.Fatalf("read %s: %v", docPath, err)
		}
		doc := string(docBytes)
		for _, want := range []string{
			"trusted_shell",
			"trusted_path",
			"completion_audit_trusted_shell",
			"completion_audit_trusted_path",
			"completion_audit_started_at",
			"completion_audit_completed_at",
			"attestation_reviewed_by",
			"attestation_reviewed_at",
			"completion_audit_script",
			"completion_audit_script_sha256",
			"production_acceptance_script",
			"production_acceptance_script_sha256",
			"ide_smoke_script",
			"ide_smoke_script_sha256",
			"ide_direct_use_script",
			"ide_direct_use_script_sha256",
			"ide_ai_provider_script",
			"ide_ai_provider_script_sha256",
			"release_managed_script",
			"release_managed_script_sha256",
			"managed_payloads_script",
			"managed_payloads_script_sha256",
			"workflow_closure_script",
			"workflow_closure_script_sha256",
			"binary_name",
			"binary_expected_sha256",
			"rule_bundle_name",
			"rule_bundle_expected_sha256",
			"ide_smoke_workspace",
			"release_managed_workspace",
			"managed_payloads_workspace",
			"workflow_closure_workspace",
			"completion-evidence-manifest.txt",
			"completion-evidence-manifest.sha256",
			"completion-audit.txt",
			"evidence_manifest",
			"evidence_manifest_digest",
			"evidence_manifest_sha256",
			"result_sha256",
			"policy_accepted_at",
			"referenced_binary_payload",
			"referenced_binary_payload_sha256",
			"referenced_rule_bundle_payload",
			"referenced_rule_bundle_payload_sha256",
			"referenced_rule_bundle_payload_digest_scope=directory_canonical_records",
			"accepted_at must not be after started_at",
			"completed_at must not be before started_at",
			"selected_at must not be before started_at",
			"selected_at must not be after completed_at",
			"created_at must not be after selected_at",
			"executed_at must not be after selected_at",
			"reviewed_at must not be after selected_at",
			"reviewed_at must not be before completed_at",
			"reviewed_at must not be after audit_started_at",
			"completion audit log directory",
			"output files must not be symlinks",
			"regular files or absent",
			"same directory as the production summary",
			"ide-smoke.txt",
			"ide-smoke-logs/direct-use.txt",
			"ide-smoke-logs/ai-provider.txt",
			"direct-use-smoke: ok",
			"chat: available",
			"ai-provider-smoke: ok",
			"chat-prompt: preview=created stored=false",
			"ide_direct_use_log",
			"ide_direct_use_log_sha256",
			"ide_ai_provider_log",
			"ide_ai_provider_log_sha256",
			"release-managed.txt",
			"managed-payloads.txt",
			"workflow-closure.txt",
			"toolchain/rules/<rule_bundle_name>",
			"toolchain/managed-bundles/<rule_bundle_name>/payload",
			"rule_bundle_expected_sha256",
			"release_managed_rule_target",
			"release_managed_rule_payload_sha256",
			"release_managed_rule_payload_digest_scope=directory_canonical_records",
			"toolchain/managed-bundles/<binary_name>/payload/<binary_name>",
			"reports/report-projection-selection.json",
			"workflow_report_selection_sha256",
			"workflow_report_execution_sha256",
			"workflow_report_dispatch_sha256",
			"workflow_report_sha256",
			"workflow_report_dispatch_created_at",
			"workflow_report_execution_executed_at",
			"workflow_report_execution_reviewed_at",
			"every status value must be `current`",
		} {
			if !strings.Contains(doc, want) {
				t.Fatalf("expected %s final evidence runbook to document %q", docPath, want)
			}
		}
	}
}

func TestCompletionAuditScriptUsesUnpredictableOutputTempFiles(t *testing.T) {
	scriptBytes, err := os.ReadFile(filepath.Join("scripts", "audit_completion.sh"))
	if err != nil {
		t.Fatalf("read completion audit script: %v", err)
	}
	script := string(scriptBytes)
	for _, forbidden := range []string{
		"$RESULT_PATH.tmp.$$",
		"$MANIFEST_PATH.tmp.$$",
		"$MANIFEST_DIGEST_PATH.tmp.$$",
		".tmp.$$",
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("completion audit output temp files must not use predictable suffix %q", forbidden)
		}
	}
	for _, want := range []string{
		`mktemp "$OURO_COMPLETION_AUDIT_LOG_DIR/.completion-audit.txt.XXXXXX"`,
		`mktemp "$OURO_COMPLETION_AUDIT_LOG_DIR/.completion-evidence-manifest.txt.XXXXXX"`,
		`mktemp "$OURO_COMPLETION_AUDIT_LOG_DIR/.completion-evidence-manifest.sha256.XXXXXX"`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("completion audit script must create output temp file with %q", want)
		}
	}
}

func TestCompletionAuditScriptPublishesSuccessOutputsThroughFailClosedMoves(t *testing.T) {
	scriptBytes, err := os.ReadFile(filepath.Join("scripts", "audit_completion.sh"))
	if err != nil {
		t.Fatalf("read completion audit script: %v", err)
	}
	script := string(scriptBytes)
	for _, want := range []string{
		`mv -f "$RESULT_TMP_PATH" "$RESULT_PATH" || fail "unable to publish completion audit result"`,
		`mv -f "$MANIFEST_TMP_PATH" "$MANIFEST_PATH" || fail "unable to publish completion audit manifest"`,
		`mv -f "$MANIFEST_DIGEST_TMP_PATH" "$MANIFEST_DIGEST_PATH" || fail "unable to publish completion audit manifest digest"`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("completion audit script must publish success output with fail-closed move %q", want)
		}
	}
}

func TestMakeDoctorKeepsSafetyBypassWarningsNonBlocking(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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

	cmd := exec.Command("make", "doctor")
	cmd.Env = append(os.Environ(), "GOPROXY=https://goproxy.cn,direct", "WORKSPACE="+root)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("default make doctor should remain warning-only: %v\n%s", err, output)
	}
	for _, want := range []string{
		"workspace: ready",
		"missing safety bypass authorization for scope.target_policy disabled",
		"missing safety bypass authorization for execution.isolation disabled",
	} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("expected default make doctor output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestMakeDoctorStrictRequiresSafetyBypassAuthorization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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

	cmd := exec.Command("make", "doctor-strict")
	cmd.Env = append(os.Environ(), "GOPROXY=https://goproxy.cn,direct", "WORKSPACE="+root)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected strict make doctor to fail without safety authorization, got output:\n%s", output)
	}
	for _, want := range []string{
		"missing required safety bypass authorization for scope.target_policy disabled",
		"missing required safety bypass authorization for execution.isolation disabled",
	} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("expected strict make doctor output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestMakeReleaseSupportsRelativeDistDir(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	relativeDist, err := filepath.Rel(cwd, filepath.Join(t.TempDir(), "dist-test-release-scripts"))
	if err != nil {
		t.Fatalf("create relative dist path: %v", err)
	}
	version := "0.1.0-make-test"
	cmd := exec.Command("make", "release")
	cmd.Env = append(os.Environ(),
		"GOPROXY=https://goproxy.cn,direct",
		"DIST_DIR="+relativeDist,
		"VERSION="+version,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make release failed: %v\n%s", err, output)
	}
	archiveName := "ouro_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	if _, err := os.Stat(filepath.Join(relativeDist, archiveName)); err != nil {
		t.Fatalf("expected relative dist archive: %v\noutput:\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(relativeDist, "SHA256SUMS")); err != nil {
		t.Fatalf("expected relative dist SHA256SUMS: %v\noutput:\n%s", err, output)
	}
}

func TestSmokeWorkflowScriptExercisesCoreIDEPath(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	logDir := filepath.Join(t.TempDir(), "logs")
	cmd := exec.Command("sh", "./scripts/smoke_workflow.sh")
	cmd.Env = append(os.Environ(),
		"GOPROXY=https://goproxy.cn,direct",
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
		"GOTMPDIR="+t.TempDir(),
		"WORKSPACE="+workspace,
		"OURO_SMOKE_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("smoke workflow script failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, want := range []string{
		"workflow-smoke: workspace " + workspace,
		"doctor: ready",
		"run: run-",
		"replay: ok",
		"workflow-status:",
		"workflow-smoke: ok",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected smoke workflow output %q, got:\n%s", want, text)
		}
	}
	if _, err := os.Stat(filepath.Join(logDir, "run.txt")); err != nil {
		t.Fatalf("expected run log: %v", err)
	}
	if _, err := os.Stat(filepath.Join(logDir, "replay.txt")); err != nil {
		t.Fatalf("expected replay log: %v", err)
	}
	if _, err := os.Stat(filepath.Join(logDir, "workflow-status.txt")); err != nil {
		t.Fatalf("expected workflow status log: %v", err)
	}
}

func writeFakeDirectUseOuro(t *testing.T, fakeOuro, fakeOuroLog string) {
	t.Helper()
	fakeScript := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$FAKE_OURO_LOG"
cmd="$1"
workspace="${2:-}"
run_id=run-direct-use-fake
report="$workspace/reports/$run_id.md"
review="$workspace/toolchain/compile-preview-reviews/direct-use-review.json"
selection="$workspace/toolchain/compile-preview-review-selection.json"
case "$cmd" in
init)
	mkdir -p "$workspace"
	printf 'initialized workspace: %s\n' "$workspace"
	;;
doctor)
	printf 'workspace: ready\n'
	;;
chat)
	if [ "${3:-}" = "--tui" ]; then
		printf 'Workspace\nMission Conversation\nAttack Graph\nRuntime Tabs\n'
		printf 'dialog: ai operator chat\n'
		printf 'AI setup: status=needs-provider\n'
		printf 'You > |\n'
	elif [ "${3:-}" = "--prompt" ]; then
		workspace="$2"
		preview_dir="$workspace/runtime/model-tool-plan-previews"
		mkdir -p "$preview_dir"
		printf '{"schema":"ouroboros.model_tool_plan_preview.v1","status":"created","action":"enumerate_paths","stored":false}\n' > "$preview_dir/preview-chat-fake.json"
		printf 'chat: status=completed provider=smoke-openai model=gpt-smoke tokens=12 stored=false\n'
		printf 'chat-text: semantic_action: enumerate_paths\n'
		printf 'chat-preview: status=created action=enumerate_paths marker=semantic_action: enumerate_paths artifact_created=true stored=false\n'
	else
		printf 'Launch the workspace AI chat IDE\n\n'
		printf 'Usage:\n'
		printf '  ouro chat <workspace>\n'
	fi
	;;
run)
	mkdir -p "$(dirname "$report")" "$(dirname "$review")"
	printf '# fake direct use report\n' > "$report"
	printf '{"status":"ready"}\n' > "$review"
	printf '{"status":"current"}\n' > "$selection"
	printf 'run: %s\n' "$run_id"
	printf 'events: 69\n'
	printf 'report: %s\n' "$report"
	printf 'Workspace\nMission Conversation\nAttack Graph\nRuntime Tabs\n'
	;;
replay)
	printf 'replay: %s\n' "${3:-$run_id}"
	printf 'events: 69\n'
	printf 'model: offline-model\n'
	printf 'tools: ready\n'
	printf 'Workspace\nMission Conversation\nAttack Graph\nRuntime Tabs\n'
	;;
workflow-status)
	printf 'workflow-status: status=ready mode=read-only stage=model-tool-plan next_gate=create-compile-preview-review-execution-dispatch\n'
	printf 'workflow-links: review=%s selection=%s\n' "$review" "$selection"
	;;
*)
	printf 'unexpected fake ouro command: %s\n' "$*" >&2
	exit 1
	;;
esac
`
	if err := os.WriteFile(fakeOuro, []byte(fakeScript), 0o755); err != nil {
		t.Fatalf("write fake ouro: %v", err)
	}
	_ = fakeOuroLog
}

func writeFakeAIProviderSmokeOuro(t *testing.T, fakeOuro, fakeOuroLog string) {
	t.Helper()
	fakeScript := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$FAKE_OURO_LOG"
cmd="$1"
workspace="${3:-}"
case "$cmd" in
init)
	workspace="$2"
	mkdir -p "$workspace"
	printf 'initialized workspace: %s\n' "$workspace"
	;;
provider)
	subcmd="$2"
	case "$subcmd" in
	configure-openai-compatible)
		workspace="$3"
		mkdir -p "$workspace"
		cat > "$workspace/providers.yaml" <<EOF
version: 1
providers:
  - name: smoke-openai
    type: openai_compatible
    model: gpt-smoke
    base_url: http://127.0.0.1:1/v1
    api_key_env: OURO_AI_PROVIDER_SMOKE_API_KEY
    enabled: true
  - name: offline
    type: offline
    model: offline-model
    enabled: false
EOF
		printf 'provider-configure: status=written provider=smoke-openai type=openai_compatible model=gpt-smoke enabled=true\n'
		printf 'providers_yaml=%s/providers.yaml\n' "$workspace"
		printf 'api_key_env=OURO_AI_PROVIDER_SMOKE_API_KEY\n'
		printf 'api_key_value_stored=false\n'
		printf 'routing_strategy=enabled_order_failover\n'
		;;
	generate)
		workspace="$3"
		usage_dir="$workspace/runtime/model-usage"
		mkdir -p "$usage_dir"
		usage_record="$usage_dir/provider-generate-smoke-openai-gpt-smoke.json"
		printf '{"source":"provider-generate","provider_name":"smoke-openai","model":"gpt-smoke"}\n' > "$usage_record"
		printf 'provider-generate: status=completed\n'
		printf 'provider=smoke-openai\n'
		printf 'model=gpt-smoke\n'
		printf 'input_tokens=7\n'
		printf 'output_tokens=5\n'
		printf 'total_tokens=12\n'
		printf 'capabilities=chat|tool_planning\n'
		printf 'text=semantic_action: enumerate_paths\n'
		printf 'usage_record=%s\n' "$usage_record"
		;;
	usage)
		printf 'provider-usage-summary:\n'
		printf 'records=1\n'
		printf 'providers=smoke-openai\n'
		printf 'models=gpt-smoke\n'
		printf 'total_tokens=12\n'
		;;
	*)
		printf 'unexpected fake ouro provider command: %s\n' "$*" >&2
		exit 1
		;;
	esac
	;;
chat)
	workspace="$2"
	preview_dir="$workspace/runtime/model-tool-plan-previews"
	mkdir -p "$preview_dir"
	printf '{"source":"tui-ai-dialog","action":"enumerate_paths","valid":true,"status":"completed"}\n' > "$preview_dir/chat-prompt-preview.json"
	printf 'chat: status=completed provider=smoke-openai model=gpt-smoke tokens=12 stored=false\n'
	printf 'chat-text: semantic_action: enumerate_paths\n'
	printf 'chat-preview: status=created action=enumerate_paths marker=semantic_action: enumerate_paths artifact_created=true stored=false\n'
	;;
doctor)
	printf 'workspace: ready\n'
	;;
*)
	printf 'unexpected fake ouro command: %s\n' "$*" >&2
	exit 1
	;;
esac
`
	if err := os.WriteFile(fakeOuro, []byte(fakeScript), 0o755); err != nil {
		t.Fatalf("write fake ouro: %v", err)
	}
	_ = fakeOuroLog
}

func writeFakeIDESmokeOuro(t *testing.T, fakeOuro, fakeOuroLog string) {
	t.Helper()
	fakeScript := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$FAKE_OURO_LOG"
cmd="$1"
case "$cmd" in
init)
	workspace="$2"
	mkdir -p "$workspace"
	printf 'initialized workspace: %s\n' "$workspace"
	;;
doctor)
	printf 'workspace: ready\n'
	;;
chat)
	if [ "${3:-}" = "--tui" ]; then
		printf 'Workspace\nMission Conversation\nAttack Graph\nRuntime Tabs\n'
		printf 'dialog: ai operator chat\n'
		printf 'AI setup: status=needs-provider\n'
		printf 'You > |\n'
	elif [ "${3:-}" = "--prompt" ]; then
		workspace="$2"
		preview_dir="$workspace/runtime/model-tool-plan-previews"
		mkdir -p "$preview_dir"
		printf '{"schema":"ouroboros.model_tool_plan_preview.v1","status":"created","action":"enumerate_paths","stored":false}\n' > "$preview_dir/preview-chat-fake.json"
		printf 'chat: status=completed provider=smoke-openai model=gpt-smoke tokens=12 stored=false\n'
		printf 'chat-text: semantic_action: enumerate_paths\n'
		printf 'chat-preview: status=created action=enumerate_paths marker=semantic_action: enumerate_paths artifact_created=true stored=false\n'
	else
		printf 'Launch the workspace AI chat IDE\n\n'
		printf 'Usage:\n'
		printf '  ouro chat <workspace>\n'
	fi
	;;
run)
	workspace="$2"
	run_id=run-ide-fake
	report="$workspace/reports/$run_id.md"
	review="$workspace/toolchain/compile-preview-reviews/ide-review.json"
	selection="$workspace/toolchain/compile-preview-review-selection.json"
	mkdir -p "$(dirname "$report")" "$(dirname "$review")"
	printf '# fake IDE report\n' > "$report"
	printf '{"status":"ready"}\n' > "$review"
	printf '{"status":"current"}\n' > "$selection"
	printf 'run: %s\n' "$run_id"
	printf 'events: 69\n'
	printf 'report: %s\n' "$report"
	printf 'Workspace\nMission Conversation\nAttack Graph\nRuntime Tabs\n'
	;;
replay)
	printf 'replay: %s\n' "${3:-run-ide-fake}"
	printf 'events: 69\n'
	printf 'model: offline-model\n'
	printf 'tools: ready\n'
	printf 'Workspace\nMission Conversation\nAttack Graph\nRuntime Tabs\n'
	;;
workflow-status)
	workspace="$2"
	review="$workspace/toolchain/compile-preview-reviews/ide-review.json"
	selection="$workspace/toolchain/compile-preview-review-selection.json"
	printf 'workflow-status: status=ready mode=read-only stage=model-tool-plan next_gate=create-compile-preview-review-execution-dispatch\n'
	printf 'workflow-links: review=%s selection=%s\n' "$review" "$selection"
	;;
provider)
	subcmd="$2"
	case "$subcmd" in
	configure-openai-compatible)
		workspace="$3"
		mkdir -p "$workspace"
		cat > "$workspace/providers.yaml" <<EOF
version: 1
providers:
  - name: smoke-openai
    type: openai_compatible
    model: gpt-smoke
    base_url: http://127.0.0.1:1/v1
    api_key_env: OURO_AI_PROVIDER_SMOKE_API_KEY
    enabled: true
  - name: offline
    type: offline
    model: offline-model
    enabled: false
EOF
		printf 'provider-configure: status=written provider=smoke-openai type=openai_compatible model=gpt-smoke enabled=true\n'
		printf 'providers_yaml=%s/providers.yaml\n' "$workspace"
		printf 'api_key_env=OURO_AI_PROVIDER_SMOKE_API_KEY\n'
		printf 'api_key_value_stored=false\n'
		printf 'routing_strategy=enabled_order_failover\n'
		;;
	generate)
		workspace="$3"
		usage_dir="$workspace/runtime/model-usage"
		mkdir -p "$usage_dir"
		usage_record="$usage_dir/provider-generate-smoke-openai-gpt-smoke.json"
		printf '{"source":"provider-generate","provider_name":"smoke-openai","model":"gpt-smoke"}\n' > "$usage_record"
		printf 'provider-generate: status=completed\n'
		printf 'provider=smoke-openai\n'
		printf 'model=gpt-smoke\n'
		printf 'input_tokens=7\n'
		printf 'output_tokens=5\n'
		printf 'total_tokens=12\n'
		printf 'capabilities=chat|tool_planning\n'
		printf 'text=semantic_action: enumerate_paths\n'
		printf 'usage_record=%s\n' "$usage_record"
		;;
	usage)
		printf 'provider-usage-summary:\n'
		printf 'records=1\n'
		printf 'providers=smoke-openai\n'
		printf 'models=gpt-smoke\n'
		printf 'total_tokens=12\n'
		;;
	*)
		printf 'unexpected fake ouro provider command: %s\n' "$*" >&2
		exit 1
		;;
	esac
	;;
*)
	printf 'unexpected fake ouro command: %s\n' "$*" >&2
	exit 1
	;;
esac
`
	if err := os.WriteFile(fakeOuro, []byte(fakeScript), 0o755); err != nil {
		t.Fatalf("write fake ouro: %v", err)
	}
	_ = fakeOuroLog
}

func TestIDESmokeScriptExercisesDirectUseAndAIProviderPath(t *testing.T) {
	workspaceRoot := filepath.Join(t.TempDir(), "ide workspace root")
	logDir := filepath.Join(t.TempDir(), "ide logs")
	fakeOuroLog := filepath.Join(t.TempDir(), "fake-ouro.log")
	fakeOuro := filepath.Join(t.TempDir(), "ouro")
	writeFakeIDESmokeOuro(t, fakeOuro, fakeOuroLog)

	cmd := exec.Command("sh", "./scripts/smoke_ide.sh")
	cmd.Env = append(os.Environ(),
		"FAKE_OURO_LOG="+fakeOuroLog,
		"WORKSPACE="+workspaceRoot,
		"OURO_IDE_SMOKE_BIN="+fakeOuro,
		"OURO_IDE_SMOKE_LOG_DIR="+logDir,
		"OURO_AI_PROVIDER_SMOKE_SKIP_FAKE_SERVER=1",
		"OURO_SMOKE_SKIP_BUILD=1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("IDE smoke script failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, want := range []string{
		"ide-smoke: workspace-root " + workspaceRoot,
		"binary: " + fakeOuro,
		"direct-use: ok",
		"ai-provider: ok",
		"ide-smoke: ok",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected IDE smoke output %q, got:\n%s", want, text)
		}
	}
	if strings.Contains(text, "smoke-secret-value") {
		t.Fatalf("IDE smoke output leaked API key value:\n%s", text)
	}
	logBytes, err := os.ReadFile(fakeOuroLog)
	if err != nil {
		t.Fatalf("read fake ouro log: %v", err)
	}
	logText := string(logBytes)
	for _, want := range []string{
		"init " + filepath.Join(workspaceRoot, "direct-use-workspace"),
		"chat --help",
		"chat " + filepath.Join(workspaceRoot, "direct-use-workspace") + " --tui",
		"run " + filepath.Join(workspaceRoot, "direct-use-workspace") + " --tui",
		"provider configure-openai-compatible " + filepath.Join(workspaceRoot, "ai-provider-workspace"),
		"provider generate " + filepath.Join(workspaceRoot, "ai-provider-workspace"),
		"provider usage " + filepath.Join(workspaceRoot, "ai-provider-workspace") + " --summary",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("expected fake ouro invocation %q, got:\n%s", want, logText)
		}
	}
	for _, name := range []string{"direct-use.txt", "ai-provider.txt"} {
		if _, err := os.Stat(filepath.Join(logDir, name)); err != nil {
			t.Fatalf("expected %s log: %v", name, err)
		}
	}
}

func TestDirectUseSmokeScriptExercisesBuiltBinaryPath(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace with spaces")
	logDir := filepath.Join(t.TempDir(), "logs")
	fakeOuroLog := filepath.Join(t.TempDir(), "fake-ouro.log")
	fakeOuro := filepath.Join(t.TempDir(), "ouro")
	writeFakeDirectUseOuro(t, fakeOuro, fakeOuroLog)

	cmd := exec.Command("sh", "./scripts/smoke_direct_use.sh")
	cmd.Env = append(os.Environ(),
		"FAKE_OURO_LOG="+fakeOuroLog,
		"WORKSPACE="+workspace,
		"OURO_DIRECT_USE_SMOKE_BIN="+fakeOuro,
		"OURO_DIRECT_USE_SMOKE_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("direct use smoke script failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, want := range []string{
		"direct-use-smoke: workspace " + workspace,
		"binary: " + fakeOuro,
		"doctor: ready",
		"chat: available",
		"chat: composer",
		"run: run-direct-use-fake",
		"events: 69",
		"replay: ok",
		"workflow-status: status=ready next_gate=create-compile-preview-review-execution-dispatch",
		"artifacts: report review selection",
		"direct-use-smoke: ok",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected direct use smoke output %q, got:\n%s", want, text)
		}
	}
	logBytes, err := os.ReadFile(fakeOuroLog)
	if err != nil {
		t.Fatalf("read fake ouro log: %v", err)
	}
	logText := string(logBytes)
	for _, want := range []string{
		"init " + workspace,
		"doctor " + workspace,
		"chat --help",
		"chat " + workspace + " --tui",
		"run " + workspace + " --tui",
		"replay " + workspace + " run-direct-use-fake --tui",
		"workflow-status " + workspace,
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("expected fake ouro invocation %q, got:\n%s", want, logText)
		}
	}
	for _, name := range []string{"init.txt", "doctor.txt", "chat-help.txt", "run.txt", "replay.txt", "workflow-status.txt"} {
		if _, err := os.Stat(filepath.Join(logDir, name)); err != nil {
			t.Fatalf("expected %s log: %v", name, err)
		}
	}
}

func TestAIProviderSmokeScriptExercisesConfigureAndGeneratePath(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace with spaces")
	logDir := filepath.Join(t.TempDir(), "logs")
	fakeOuroLog := filepath.Join(t.TempDir(), "fake-ouro.log")
	fakeOuro := filepath.Join(t.TempDir(), "ouro")
	writeFakeAIProviderSmokeOuro(t, fakeOuro, fakeOuroLog)

	cmd := exec.Command("sh", "./scripts/smoke_ai_provider.sh")
	cmd.Env = append(os.Environ(),
		"FAKE_OURO_LOG="+fakeOuroLog,
		"WORKSPACE="+workspace,
		"OURO_AI_PROVIDER_SMOKE_BIN="+fakeOuro,
		"OURO_AI_PROVIDER_SMOKE_LOG_DIR="+logDir,
		"OURO_AI_PROVIDER_SMOKE_SKIP_FAKE_SERVER=1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("AI provider smoke script failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, want := range []string{
		"ai-provider-smoke: workspace " + workspace,
		"binary: " + fakeOuro,
		"provider-configure: ok",
		"doctor: ready",
		"provider-generate: ok",
		"chat-prompt: preview=created stored=false",
		"usage-summary: records=1",
		"ai-provider-smoke: ok",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected AI provider smoke output %q, got:\n%s", want, text)
		}
	}
	if strings.Contains(text, "smoke-secret-value") {
		t.Fatalf("AI provider smoke output leaked API key value:\n%s", text)
	}
	logBytes, err := os.ReadFile(fakeOuroLog)
	if err != nil {
		t.Fatalf("read fake ouro log: %v", err)
	}
	logText := string(logBytes)
	for _, want := range []string{
		"init " + workspace,
		"provider configure-openai-compatible " + workspace,
		"doctor " + workspace,
		"provider generate " + workspace,
		"chat " + workspace + " --prompt Return one reviewed semantic action marker.",
		"provider usage " + workspace + " --summary",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("expected fake ouro invocation %q, got:\n%s", want, logText)
		}
	}
	if providers, err := os.ReadFile(filepath.Join(workspace, "providers.yaml")); err != nil {
		t.Fatalf("read providers.yaml: %v", err)
	} else if strings.Contains(string(providers), "smoke-secret-value") {
		t.Fatalf("providers.yaml leaked API key value:\n%s", providers)
	}
	for _, name := range []string{"init.txt", "configure-provider.txt", "doctor.txt", "provider-generate.txt", "chat-prompt.txt", "provider-usage-summary.txt"} {
		if _, err := os.Stat(filepath.Join(logDir, name)); err != nil {
			t.Fatalf("expected %s log: %v", name, err)
		}
	}
}

func TestDirectUseSmokeRejectsSymlinkPathComponents(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mutateEnv func(t *testing.T, env map[string]string)
		want      string
	}{
		{
			name: "direct use binary",
			mutateEnv: func(t *testing.T, env map[string]string) {
				targetDir := t.TempDir()
				linkDir := filepath.Join(t.TempDir(), "linked-bin")
				if err := os.Symlink(targetDir, linkDir); err != nil {
					t.Fatalf("symlink binary parent: %v", err)
				}
				fakeOuro := filepath.Join(targetDir, "ouro")
				writeFakeDirectUseOuro(t, fakeOuro, env["FAKE_OURO_LOG"])
				env["OURO_DIRECT_USE_SMOKE_BIN"] = filepath.Join(linkDir, "ouro")
			},
			want: "direct use binary path component must not be a symlink",
		},
		{
			name: "workspace",
			mutateEnv: func(t *testing.T, env map[string]string) {
				targetDir := t.TempDir()
				linkDir := filepath.Join(t.TempDir(), "linked-workspace-parent")
				if err := os.Symlink(targetDir, linkDir); err != nil {
					t.Fatalf("symlink workspace parent: %v", err)
				}
				env["WORKSPACE"] = filepath.Join(linkDir, "workspace")
			},
			want: "workspace path component must not be a symlink",
		},
		{
			name: "direct use log dir",
			mutateEnv: func(t *testing.T, env map[string]string) {
				targetDir := t.TempDir()
				linkDir := filepath.Join(t.TempDir(), "linked-log-parent")
				if err := os.Symlink(targetDir, linkDir); err != nil {
					t.Fatalf("symlink log parent: %v", err)
				}
				env["OURO_DIRECT_USE_SMOKE_LOG_DIR"] = filepath.Join(linkDir, "logs")
			},
			want: "direct use log dir path component must not be a symlink",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeOuroLog := filepath.Join(t.TempDir(), "fake-ouro.log")
			fakeOuro := filepath.Join(t.TempDir(), "ouro")
			writeFakeDirectUseOuro(t, fakeOuro, fakeOuroLog)
			env := map[string]string{
				"FAKE_OURO_LOG":                 fakeOuroLog,
				"OURO_DIRECT_USE_SMOKE_BIN":     fakeOuro,
				"OURO_DIRECT_USE_SMOKE_LOG_DIR": filepath.Join(t.TempDir(), "logs"),
				"WORKSPACE":                     filepath.Join(t.TempDir(), "workspace"),
			}
			tc.mutateEnv(t, env)

			cmd := exec.Command("sh", "./scripts/smoke_direct_use.sh")
			cmd.Env = append(os.Environ(),
				"FAKE_OURO_LOG="+env["FAKE_OURO_LOG"],
				"OURO_DIRECT_USE_SMOKE_BIN="+env["OURO_DIRECT_USE_SMOKE_BIN"],
				"OURO_DIRECT_USE_SMOKE_LOG_DIR="+env["OURO_DIRECT_USE_SMOKE_LOG_DIR"],
				"WORKSPACE="+env["WORKSPACE"],
			)
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("expected symlink component path to fail, got output:\n%s", output)
			}
			if !strings.Contains(string(output), tc.want) {
				t.Fatalf("expected %q, got:\n%s", tc.want, output)
			}
		})
	}
}

func TestSmokeWorkflowClosureScriptExercisesOperatorGateClosure(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace with spaces")
	logDir := filepath.Join(t.TempDir(), "logs")
	cmd := exec.Command("sh", "./scripts/smoke_workflow_closure.sh")
	cmd.Env = append(os.Environ(),
		"GOPROXY=https://goproxy.cn,direct",
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
		"GOTMPDIR="+t.TempDir(),
		"WORKSPACE="+workspace,
		"OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("workflow closure smoke script failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, want := range []string{
		"workflow-closure-smoke: workspace " + workspace,
		"doctor: ready",
		"run: run-",
		"workflow-status: next_gate=create-compile-preview-review-execution-dispatch",
		"compile-dispatch: status=dry-run mode=metadata-only",
		"workflow-status: next_gate=execute-compile-preview-review-execution-dispatch",
		"compile-execution: status=completed mode=execute",
		"workflow-status: next_gate=create-report-projection-dispatch",
		"report-dispatch: status=dry-run mode=metadata-only",
		"workflow-status: next_gate=execute-report-projection-dispatch",
		"report-execution: status=completed written=true",
		"workflow-status: next_gate=review-report-projection-execution",
		"report-annotation: decision=accepted",
		"workflow-status: next_gate=select-report-projection-execution",
		"report-selection: status=current",
		"workflow-status: next_gate=review-current-report-projection",
		"replay: ok",
		"workflow-closure-smoke: ok",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected workflow closure smoke output %q, got:\n%s", want, text)
		}
	}
	for _, logName := range []string{
		"init.txt",
		"doctor.txt",
		"run.txt",
		"workflow-status-create-compile-dispatch.txt",
		"compile-dispatch.txt",
		"workflow-status-execute-compile-dispatch.txt",
		"compile-execute.txt",
		"workflow-status-create-report-dispatch.txt",
		"report-dispatch.txt",
		"workflow-status-execute-report-dispatch.txt",
		"report-execute.txt",
		"workflow-status-review-report-execution.txt",
		"report-annotate.txt",
		"workflow-status-select-report-execution.txt",
		"report-select.txt",
		"workflow-status-review-current-report.txt",
		"report-selection.txt",
		"replay.txt",
	} {
		if _, err := os.Stat(filepath.Join(logDir, logName)); err != nil {
			t.Fatalf("expected workflow closure smoke log %s: %v", logName, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "reports", "report-projection-selection.json")); err != nil {
		t.Fatalf("expected final report projection selection marker: %v", err)
	}
	if matches, err := filepath.Glob(filepath.Join(workspace, "toolchain", "compile-preview-review-execution-dispatches", "*.json")); err != nil || len(matches) != 1 {
		t.Fatalf("expected one compile dispatch artifact, matches=%#v err=%v", matches, err)
	}
	if matches, err := filepath.Glob(filepath.Join(workspace, "toolchain", "compile-preview-review-executions", "*.json")); err != nil || len(matches) != 1 {
		t.Fatalf("expected one compile execution artifact, matches=%#v err=%v", matches, err)
	}
	if matches, err := filepath.Glob(filepath.Join(workspace, "reports", "report-projection-dispatches", "*.json")); err != nil || len(matches) != 1 {
		t.Fatalf("expected one report dispatch artifact, matches=%#v err=%v", matches, err)
	}
	if matches, err := filepath.Glob(filepath.Join(workspace, "reports", "report-projection-dispatch-executions", "*.json")); err != nil || len(matches) != 1 {
		t.Fatalf("expected one report execution artifact, matches=%#v err=%v", matches, err)
	}
}

func TestReleaseManagedSmokeScriptExercisesInstalledReleaseManagedPath(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace with spaces")
	prefix := filepath.Join(t.TempDir(), "prefix with spaces")
	dist := filepath.Join(t.TempDir(), "dist with spaces")
	logDir := filepath.Join(t.TempDir(), "logs")
	cmd := exec.Command("sh", "./scripts/smoke_release_managed.sh")
	cmd.Env = append(os.Environ(),
		"GOPROXY=https://goproxy.cn,direct",
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
		"GOTMPDIR="+t.TempDir(),
		"WORKSPACE="+workspace,
		"PREFIX="+prefix,
		"DIST_DIR="+dist,
		"VERSION=0.1.0-release-managed-smoke-test",
		"OURO_RELEASE_MANAGED_SMOKE_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("release-managed smoke script failed: %v\n%s", err, output)
	}
	text := string(output)
	releaseRoot := filepath.Join(prefix, "share", "ouroboros")
	for _, want := range []string{
		"release-managed-smoke: workspace " + workspace,
		"release-managed-smoke: release-root " + releaseRoot,
		"release: ok",
		"install: ok",
		"doctor: ready",
		"release-managed-bundles: nuclei-templates",
		"release-managed-status: import_status=not_imported link_status=missing",
		"expected_sha256_status=matched",
		"release-managed-status-source: status=current expected_sha256_status=matched",
		"release-managed-install-plan: status=ready",
		"release-managed-install-without-execute: rejected",
		"release-managed-install: status=installed expected_sha256_status=matched",
		"release-managed-status: import_status=linked link_status=linked",
		"tui: ok",
		"release-managed-smoke: ok",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected release-managed smoke output %q, got:\n%s", want, text)
		}
	}
	for _, logName := range []string{
		"release.txt",
		"install.txt",
		"init.txt",
		"doctor.txt",
		"release-managed-bundles.txt",
		"release-managed-status-before.txt",
		"release-managed-status-source-set.txt",
		"release-managed-status-source.txt",
		"release-managed-install-plan.txt",
		"release-managed-install-without-execute.txt",
		"release-managed-install.txt",
		"release-managed-status-after.txt",
		"run-tui.txt",
	} {
		if _, err := os.Stat(filepath.Join(logDir, logName)); err != nil {
			t.Fatalf("expected release-managed smoke log %s: %v", logName, err)
		}
	}
	installedRule := filepath.Join(workspace, "toolchain", "managed-bundles", "nuclei-templates", "payload", "http", "ouro-default-healthcheck.yaml")
	if _, err := os.Stat(installedRule); err != nil {
		t.Fatalf("expected smoke to install bundled rule payload %s: %v", installedRule, err)
	}
	linkInfo, err := os.Lstat(filepath.Join(workspace, "toolchain", "rules", "nuclei-templates"))
	if err != nil {
		t.Fatalf("expected smoke to create managed rule link: %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected managed rule link to be a symlink, mode=%v", linkInfo.Mode())
	}
}

func TestManagedPayloadsSmokeScriptExercisesOperatorSuppliedPayloads(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace with spaces")
	logDir := filepath.Join(t.TempDir(), "logs")
	fixtureRoot := filepath.Join(t.TempDir(), "operator payloads")
	binaryMarker := filepath.Join(t.TempDir(), "managed-binary-ran")
	binaryBody := "#!/usr/bin/env sh\nprintf 'httpx smoke fixture\\n'\ntouch " + binaryMarker + "\n"
	binaryPayload := filepath.Join(fixtureRoot, "bin", "httpx")
	if err := os.MkdirAll(filepath.Dir(binaryPayload), 0o755); err != nil {
		t.Fatalf("create binary payload dir: %v", err)
	}
	if err := os.WriteFile(binaryPayload, []byte(binaryBody), 0o755); err != nil {
		t.Fatalf("write binary payload: %v", err)
	}
	binaryDigest := sha256.Sum256([]byte(binaryBody))
	binarySHA := hex.EncodeToString(binaryDigest[:])
	rulePayload := writeManagedPayloadsSmokeRuleBundleFixture(t, filepath.Join(fixtureRoot, "rules"), map[string]string{
		"http/example.yaml": "id: ouro-smoke-http\ninfo:\n  name: Ouro smoke HTTP\n  severity: info\n",
		"dns/example.yaml":  "id: ouro-smoke-dns\ninfo:\n  name: Ouro smoke DNS\n  severity: low\n",
	})
	ruleSHA := digestManagedPayloadsSmokeDirectory(t, rulePayload)

	cmd := exec.Command("sh", "./scripts/smoke_managed_payloads.sh")
	cmd.Env = append(os.Environ(),
		"GOPROXY=https://goproxy.cn,direct",
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
		"GOTMPDIR="+t.TempDir(),
		"WORKSPACE="+workspace,
		"OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR="+logDir,
		"OURO_MANAGED_BINARY_NAME=httpx",
		"OURO_MANAGED_BINARY_PAYLOAD="+binaryPayload,
		"OURO_MANAGED_BINARY_EXPECTED_SHA256="+strings.ToUpper(binarySHA),
		"OURO_MANAGED_BINARY_VERSION=smoke-httpx-1",
		"OURO_MANAGED_BINARY_SOURCE=operator-local/httpx",
		"OURO_MANAGED_RULE_BUNDLE_NAME=nuclei-templates",
		"OURO_MANAGED_RULE_BUNDLE_PAYLOAD="+rulePayload,
		"OURO_MANAGED_RULE_BUNDLE_EXPECTED_SHA256="+strings.ToUpper(ruleSHA),
		"OURO_MANAGED_RULE_BUNDLE_VERSION=smoke-rules-1",
		"OURO_MANAGED_RULE_BUNDLE_SOURCE=operator-local/nuclei-templates",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("managed payloads smoke script failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, want := range []string{
		"managed-payloads-smoke: workspace " + workspace,
		"managed-payloads-smoke: binary httpx",
		"managed-payloads-smoke: rule_bundle nuclei-templates",
		"doctor: ready",
		"managed-bundle-import: binary status=imported expected_sha256=" + binarySHA,
		"managed-bundle-import: rule_bundle status=imported expected_sha256=" + ruleSHA,
		"managed-install-plan: binary status=ready",
		"managed-install-plan: rule_bundle status=ready",
		"managed-install-without-execute: binary rejected",
		"managed-install-without-execute: rule_bundle rejected",
		"managed-install: binary status=installed",
		"managed-install: rule_bundle status=installed",
		"managed-bundles: binary linked",
		"managed-bundles: rule_bundle linked",
		"managed-payloads-smoke: ok",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected managed payloads smoke output %q, got:\n%s", want, text)
		}
	}
	for _, logName := range []string{
		"init.txt",
		"doctor.txt",
		"binary-import.txt",
		"rule-bundle-import.txt",
		"binary-install-plan.txt",
		"rule-bundle-install-plan.txt",
		"binary-install-without-execute.txt",
		"rule-bundle-install-without-execute.txt",
		"binary-install.txt",
		"rule-bundle-install.txt",
		"binary-managed-bundle.txt",
		"rule-bundle-managed-bundle.txt",
	} {
		if _, err := os.Stat(filepath.Join(logDir, logName)); err != nil {
			t.Fatalf("expected managed payloads smoke log %s: %v", logName, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "toolchain", "managed-bundles", "httpx", "payload", "httpx")); err != nil {
		t.Fatalf("expected imported binary payload: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "toolchain", "managed-bundles", "nuclei-templates", "payload", "http", "example.yaml")); err != nil {
		t.Fatalf("expected imported rule payload: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(workspace, "toolchain", "bin", "httpx")); err != nil {
		t.Fatalf("expected managed binary install link: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(workspace, "toolchain", "rules", "nuclei-templates")); err != nil {
		t.Fatalf("expected managed rule install link: %v", err)
	}
	binaryTarget, err := os.Readlink(filepath.Join(workspace, "toolchain", "bin", "httpx"))
	if err != nil {
		t.Fatalf("read managed binary link: %v", err)
	}
	if !strings.HasPrefix(binaryTarget, filepath.Join(workspace, "toolchain", "managed-bundles")+string(os.PathSeparator)) {
		t.Fatalf("managed binary link must point inside workspace managed-bundles, got %s", binaryTarget)
	}
	ruleTarget, err := os.Readlink(filepath.Join(workspace, "toolchain", "rules", "nuclei-templates"))
	if err != nil {
		t.Fatalf("read managed rule link: %v", err)
	}
	if !strings.HasPrefix(ruleTarget, filepath.Join(workspace, "toolchain", "managed-bundles")+string(os.PathSeparator)) {
		t.Fatalf("managed rule link must point inside workspace managed-bundles, got %s", ruleTarget)
	}
	if _, err := os.Stat(binaryMarker); !os.IsNotExist(err) {
		t.Fatalf("managed payload smoke must not execute the binary payload, marker stat err=%v", err)
	}
}

func TestManagedPayloadsSmokeRejectsRelativePayloadPath(t *testing.T) {
	fixtureRoot := filepath.Join(t.TempDir(), "operator payloads")
	binaryPayload := filepath.Join(fixtureRoot, "bin", "httpx")
	if err := os.MkdirAll(filepath.Dir(binaryPayload), 0o755); err != nil {
		t.Fatalf("create binary payload dir: %v", err)
	}
	writeTextFile(t, binaryPayload, "#!/usr/bin/env sh\nexit 0\n")
	rulePayload := writeManagedPayloadsSmokeRuleBundleFixture(t, filepath.Join(fixtureRoot, "rules"), map[string]string{
		"http/example.yaml": "id: ouro-smoke-http\ninfo:\n  name: Ouro smoke HTTP\n  severity: info\n",
	})

	output, fakeGoLog := runRejectedManagedPayloadsSmoke(t, relativePathForTest(t, binaryPayload), rulePayload)
	if !strings.Contains(string(output), "binary payload path must be absolute") {
		t.Fatalf("expected relative binary payload path refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("managed payload smoke must fail before invoking go, fake go log stat=%v", err)
	}
}

func TestManagedPayloadsSmokeRejectsSymlinkPayloadPath(t *testing.T) {
	fixtureRoot := filepath.Join(t.TempDir(), "operator payloads")
	binaryTarget := filepath.Join(fixtureRoot, "bin", "httpx-target")
	if err := os.MkdirAll(filepath.Dir(binaryTarget), 0o755); err != nil {
		t.Fatalf("create binary payload dir: %v", err)
	}
	writeTextFile(t, binaryTarget, "#!/usr/bin/env sh\nexit 0\n")
	binaryLink := filepath.Join(fixtureRoot, "bin", "httpx")
	symlinkFileForTest(t, binaryTarget, binaryLink)
	rulePayload := writeManagedPayloadsSmokeRuleBundleFixture(t, filepath.Join(fixtureRoot, "rules"), map[string]string{
		"http/example.yaml": "id: ouro-smoke-http\ninfo:\n  name: Ouro smoke HTTP\n  severity: info\n",
	})

	output, fakeGoLog := runRejectedManagedPayloadsSmoke(t, binaryLink, rulePayload)
	if !strings.Contains(string(output), "binary payload file must not be a symlink") {
		t.Fatalf("expected symlink binary payload refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("managed payload smoke must fail before invoking go, fake go log stat=%v", err)
	}
}

func TestProductionAcceptanceSmokeScriptExercisesOperatorInventoryAndPolicy(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "acceptance workspace with spaces")
	logDir := filepath.Join(t.TempDir(), "logs")
	fixtureRoot := filepath.Join(t.TempDir(), "operator payloads")
	binaryMarker := filepath.Join(t.TempDir(), "production-binary-ran")
	binaryBody := "#!/usr/bin/env sh\nprintf 'production httpx fixture\\n'\ntouch " + binaryMarker + "\n"
	binaryPayload := filepath.Join(fixtureRoot, "bin", "httpx")
	if err := os.MkdirAll(filepath.Dir(binaryPayload), 0o755); err != nil {
		t.Fatalf("create binary payload dir: %v", err)
	}
	if err := os.WriteFile(binaryPayload, []byte(binaryBody), 0o755); err != nil {
		t.Fatalf("write binary payload: %v", err)
	}
	binaryDigest := sha256.Sum256([]byte(binaryBody))
	binarySHA := hex.EncodeToString(binaryDigest[:])
	rulePayload := writeManagedPayloadsSmokeRuleBundleFixture(t, filepath.Join(fixtureRoot, "rules"), map[string]string{
		"http/production.yaml": "id: ouro-production-http\ninfo:\n  name: Ouro production HTTP\n  severity: info\n",
		"dns/production.yaml":  "id: ouro-production-dns\ninfo:\n  name: Ouro production DNS\n  severity: low\n",
	})
	ruleSHA := digestManagedPayloadsSmokeDirectory(t, rulePayload)

	evidenceDir := filepath.Join(t.TempDir(), "operator evidence")
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		t.Fatalf("create evidence dir: %v", err)
	}
	inventoryPath := filepath.Join(evidenceDir, "payload inventory.env")
	inventory := strings.Join([]string{
		"schema=ouroboros.production_acceptance.inventory.v1",
		"inventory_status=operator_owned",
		"binary_name=httpx",
		"binary_pack=web-api-pack",
		"binary_payload=" + binaryPayload,
		"binary_expected_sha256=" + strings.ToUpper(binarySHA),
		"binary_version=production-httpx-1",
		"binary_source=operator-inventory/httpx",
		"rule_bundle_name=nuclei-templates",
		"rule_bundle_pack=web-api-pack",
		"rule_bundle_payload=" + rulePayload,
		"rule_bundle_expected_sha256=" + strings.ToUpper(ruleSHA),
		"rule_bundle_version=production-rules-1",
		"rule_bundle_source=operator-inventory/nuclei-templates",
		"",
	}, "\n")
	if err := os.WriteFile(inventoryPath, []byte(inventory), 0o644); err != nil {
		t.Fatalf("write inventory: %v", err)
	}
	policyPath := filepath.Join(evidenceDir, "deployment policy.env")
	policy := strings.Join([]string{
		"schema=ouroboros.production_acceptance.policy.v1",
		"policy_status=approved",
		"deployment_environment=production",
		"accepted_by=operator",
		"accepted_at=2026-06-08T00:00:00Z",
		"authorization_scope=operator-owned-production-acceptance",
		"managed_payload_acceptance_required=true",
		"workflow_closure_required=true",
		"no_live_targets=true",
		"payload_execution_allowed=false",
		"network_download_allowed=false",
		"package_manager_allowed=false",
		"provider_calls_allowed=false",
		"worker_contact_allowed=false",
		"",
	}, "\n")
	if err := os.WriteFile(policyPath, []byte(policy), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
	cmd.Env = append(os.Environ(),
		"GOPROXY=https://goproxy.cn,direct",
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
		"GOTMPDIR="+t.TempDir(),
		"WORKSPACE="+workspace,
		"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+logDir,
		"OURO_PRODUCTION_ACCEPTANCE_INVENTORY="+inventoryPath,
		"OURO_PRODUCTION_ACCEPTANCE_POLICY="+policyPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("production acceptance smoke script failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, want := range []string{
		"production-acceptance-smoke: workspace " + workspace,
		"production-acceptance-smoke: inventory " + inventoryPath,
		"production-acceptance-smoke: policy " + policyPath,
		"inventory: status=operator_owned binary=httpx expected_sha256=" + binarySHA,
		"inventory: status=operator_owned rule_bundle=nuclei-templates expected_sha256=" + ruleSHA,
		"policy: status=approved environment=production accepted_by=operator",
		"ide-smoke: ok",
		"release-managed: ok",
		"managed-payloads: ok",
		"workflow-closure: ok",
		"production-acceptance: status=accepted mode=operator-owned",
		"production-acceptance-smoke: ok",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected production acceptance smoke output %q, got:\n%s", want, text)
		}
	}
	for _, logName := range []string{
		"ide-smoke.txt",
		"release-managed.txt",
		"managed-payloads.txt",
		"workflow-closure.txt",
		"summary.txt",
	} {
		if _, err := os.Stat(filepath.Join(logDir, logName)); err != nil {
			t.Fatalf("expected production acceptance smoke log %s: %v", logName, err)
		}
	}
	summaryBytes, err := os.ReadFile(filepath.Join(logDir, "summary.txt"))
	if err != nil {
		t.Fatalf("read production acceptance summary: %v", err)
	}
	summary := string(summaryBytes)
	for _, want := range []string{
		"schema=ouroboros.production_acceptance.audit.v1",
		"status=accepted",
		"mode=operator-owned",
		"inventory_sha256=" + fileSHA256ForTest(t, inventoryPath),
		"policy_sha256=" + fileSHA256ForTest(t, policyPath),
		"trusted_shell=",
		"trusted_path=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"production_acceptance_script=" + repoFilePathForTest(t, "scripts", "smoke_production_acceptance.sh"),
		"production_acceptance_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_production_acceptance.sh")),
		"ide_smoke_workspace=" + filepath.Join(workspace, "ide-smoke-workspace"),
		"ide_smoke_script=" + repoFilePathForTest(t, "scripts", "smoke_ide.sh"),
		"ide_smoke_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_ide.sh")),
		"ide_direct_use_script=" + repoFilePathForTest(t, "scripts", "smoke_direct_use.sh"),
		"ide_direct_use_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_direct_use.sh")),
		"ide_ai_provider_script=" + repoFilePathForTest(t, "scripts", "smoke_ai_provider.sh"),
		"ide_ai_provider_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_ai_provider.sh")),
		"release_managed_script=" + repoFilePathForTest(t, "scripts", "smoke_release_managed.sh"),
		"release_managed_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_release_managed.sh")),
		"managed_payloads_script=" + repoFilePathForTest(t, "scripts", "smoke_managed_payloads.sh"),
		"managed_payloads_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_managed_payloads.sh")),
		"workflow_closure_script=" + repoFilePathForTest(t, "scripts", "smoke_workflow_closure.sh"),
		"workflow_closure_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_workflow_closure.sh")),
		"binary_name=httpx",
		"binary_expected_sha256=" + fileSHA256ForTest(t, binaryPayload),
		"rule_bundle_name=nuclei-templates",
		"rule_bundle_expected_sha256=" + digestManagedPayloadsSmokeDirectory(t, rulePayload),
		"ide_smoke_acceptance=passed",
		"release_managed_acceptance=passed",
		"managed_payload_acceptance=passed",
		"workflow_closure_acceptance=passed",
		"ide_smoke_log=",
		"ide_direct_use_log=" + filepath.Join(logDir, "ide-smoke-logs", "direct-use.txt"),
		"ide_ai_provider_log=" + filepath.Join(logDir, "ide-smoke-logs", "ai-provider.txt"),
		"release_managed_log=",
		"managed_payloads_log=",
		"workflow_closure_log=",
		"ide_smoke_log_sha256=",
		"ide_direct_use_log_sha256=",
		"ide_ai_provider_log_sha256=",
		"release_managed_log_sha256=",
		"managed_payloads_log_sha256=",
		"workflow_closure_log_sha256=",
		"no_live_targets=true",
		"payload_execution_allowed=false",
		"network_download_allowed=false",
		"package_manager_allowed=false",
		"provider_calls_allowed=false",
		"worker_contact_allowed=false",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("expected production acceptance summary %q, got:\n%s", want, summary)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "managed-payloads-workspace", "toolchain", "managed-bundles", "httpx", "payload", "httpx")); err != nil {
		t.Fatalf("expected managed payload acceptance workspace binary: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "release-managed-workspace", "toolchain", "rules", "nuclei-templates")); err != nil {
		t.Fatalf("expected release-managed acceptance workspace rule link: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "workflow-closure-workspace", "reports", "report-projection-selection.json")); err != nil {
		t.Fatalf("expected workflow closure workspace selected report marker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "ide-smoke-workspace", "direct-use-workspace")); err != nil {
		t.Fatalf("expected IDE smoke direct-use workspace: %v", err)
	}
	directUseLog := readTextFileForTest(t, filepath.Join(logDir, "ide-smoke-logs", "direct-use.txt"))
	if !strings.Contains(directUseLog, "chat: available") {
		t.Fatalf("expected IDE direct-use child log to prove chat availability, got:\n%s", directUseLog)
	}
	if !strings.Contains(directUseLog, "chat: composer") {
		t.Fatalf("expected IDE direct-use child log to prove chat composer, got:\n%s", directUseLog)
	}
	aiProviderLog := readTextFileForTest(t, filepath.Join(logDir, "ide-smoke-logs", "ai-provider.txt"))
	if !strings.Contains(aiProviderLog, "ai-provider-smoke: ok") {
		t.Fatalf("expected IDE AI-provider child log to prove provider path, got:\n%s", aiProviderLog)
	}
	if !strings.Contains(aiProviderLog, "chat-prompt: preview=created stored=false") {
		t.Fatalf("expected IDE AI-provider child log to prove direct chat prompt path, got:\n%s", aiProviderLog)
	}
	if _, err := os.Stat(binaryMarker); !os.IsNotExist(err) {
		t.Fatalf("production acceptance smoke must not execute the binary payload, marker stat err=%v", err)
	}
}

func TestProductionAcceptanceSmokeUsesTrustedShellForChildStages(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "acceptance workspace with fake path shell")
	logDir := filepath.Join(t.TempDir(), "logs")
	fixtureRoot := filepath.Join(t.TempDir(), "operator payloads")
	binaryBody := "#!/usr/bin/env sh\nprintf 'production httpx fixture\\n'\n"
	binaryPayload := filepath.Join(fixtureRoot, "bin", "httpx")
	if err := os.MkdirAll(filepath.Dir(binaryPayload), 0o755); err != nil {
		t.Fatalf("create binary payload dir: %v", err)
	}
	if err := os.WriteFile(binaryPayload, []byte(binaryBody), 0o755); err != nil {
		t.Fatalf("write binary payload: %v", err)
	}
	binaryDigest := sha256.Sum256([]byte(binaryBody))
	binarySHA := hex.EncodeToString(binaryDigest[:])
	rulePayload := writeManagedPayloadsSmokeRuleBundleFixture(t, filepath.Join(fixtureRoot, "rules"), map[string]string{
		"http/production.yaml": "id: ouro-production-http\ninfo:\n  name: Ouro production HTTP\n  severity: info\n",
	})
	ruleSHA := digestManagedPayloadsSmokeDirectory(t, rulePayload)

	evidenceDir := filepath.Join(t.TempDir(), "operator evidence")
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		t.Fatalf("create evidence dir: %v", err)
	}
	inventoryPath := filepath.Join(evidenceDir, "payload-inventory.env")
	writeTextFile(t, inventoryPath, strings.Join([]string{
		"schema=ouroboros.production_acceptance.inventory.v1",
		"inventory_status=operator_owned",
		"binary_name=httpx",
		"binary_pack=web-api-pack",
		"binary_payload=" + binaryPayload,
		"binary_expected_sha256=" + binarySHA,
		"binary_version=production-httpx-1",
		"binary_source=operator-inventory/httpx",
		"rule_bundle_name=nuclei-templates",
		"rule_bundle_pack=web-api-pack",
		"rule_bundle_payload=" + rulePayload,
		"rule_bundle_expected_sha256=" + ruleSHA,
		"rule_bundle_version=production-rules-1",
		"rule_bundle_source=operator-inventory/nuclei-templates",
		"",
	}, "\n"))
	policyPath := filepath.Join(evidenceDir, "deployment-policy.env")
	writeTextFile(t, policyPath, validProductionAcceptancePolicyFixture())

	toolDir := t.TempDir()
	fakeShLog := filepath.Join(t.TempDir(), "fake-sh.log")
	fakeSh := filepath.Join(toolDir, "sh")
	writeTextFile(t, fakeSh, `#!/bin/sh
printf '%s\n' "$1" >> "$FAKE_SH_LOG"
case "$1" in
	*smoke_release_managed.sh) printf 'release-managed-smoke: ok\n' ;;
	*smoke_managed_payloads.sh) printf 'managed-payloads-smoke: ok\n' ;;
	*smoke_workflow_closure.sh) printf 'workflow-closure-smoke: ok\n' ;;
	*) printf 'fake-sh: unexpected child %s\n' "$1" ;;
esac
exit 0
`)
	if err := os.Chmod(fakeSh, 0o755); err != nil {
		t.Fatalf("chmod fake sh: %v", err)
	}

	cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
	cmd.Env = append(os.Environ(),
		"FAKE_SH_LOG="+fakeShLog,
		"GOPROXY=https://goproxy.cn,direct",
		"GOCACHE="+filepath.Join(t.TempDir(), "gocache"),
		"GOTMPDIR="+t.TempDir(),
		"WORKSPACE="+workspace,
		"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+logDir,
		"OURO_PRODUCTION_ACCEPTANCE_INVENTORY="+inventoryPath,
		"OURO_PRODUCTION_ACCEPTANCE_POLICY="+policyPath,
		"PATH="+toolDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("production acceptance smoke with fake PATH sh failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(fakeShLog); !os.IsNotExist(err) {
		t.Fatalf("production acceptance smoke must not invoke PATH sh, fake sh log stat=%v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "release-managed-workspace", "toolchain", "rules", "nuclei-templates")); err != nil {
		t.Fatalf("trusted shell run must create release-managed artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "managed-payloads-workspace", "toolchain", "managed-bundles", "httpx", "payload", "httpx")); err != nil {
		t.Fatalf("trusted shell run must create managed payload artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "workflow-closure-workspace", "reports", "report-projection-selection.json")); err != nil {
		t.Fatalf("trusted shell run must create workflow closure artifact: %v", err)
	}
}

func TestProductionAcceptanceSmokeDirectExecUsesTrustedShellAndPathBeforePrelude(t *testing.T) {
	toolDir := t.TempDir()
	fakeShLog := filepath.Join(t.TempDir(), "fake-sh.log")
	fakeSh := filepath.Join(toolDir, "sh")
	writeTextFile(t, fakeSh, "#!/bin/sh\nprintf 'fake sh invoked\\n' >> \"$FAKE_SH_LOG\"\nexit 42\n")
	if err := os.Chmod(fakeSh, 0o755); err != nil {
		t.Fatalf("chmod fake sh: %v", err)
	}
	fakeDirnameLog := filepath.Join(t.TempDir(), "fake-dirname.log")
	fakeDirname := filepath.Join(toolDir, "dirname")
	writeTextFile(t, fakeDirname, "#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$FAKE_DIRNAME_LOG\"\nprintf '.\\n'\n")
	if err := os.Chmod(fakeDirname, 0o755); err != nil {
		t.Fatalf("chmod fake dirname: %v", err)
	}

	cmd := exec.Command("./scripts/smoke_production_acceptance.sh")
	cmd.Env = append(os.Environ(),
		"FAKE_SH_LOG="+fakeShLog,
		"FAKE_DIRNAME_LOG="+fakeDirnameLog,
		"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+filepath.Join(t.TempDir(), "logs"),
		"PATH="+toolDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected production acceptance direct exec to fail for missing inventory, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "OURO_PRODUCTION_ACCEPTANCE_INVENTORY is required") {
		t.Fatalf("expected missing inventory failure from real script, got:\n%s", output)
	}
	if _, err := os.Stat(fakeShLog); !os.IsNotExist(err) {
		t.Fatalf("production acceptance direct exec must not invoke PATH sh, fake sh log stat=%v", err)
	}
	if _, err := os.Stat(fakeDirnameLog); !os.IsNotExist(err) {
		t.Fatalf("production acceptance prelude must not invoke PATH dirname, fake dirname log stat=%v", err)
	}
}

func TestProductionAcceptanceSmokeRefusesExistingConfiguredWorkspaceWithoutOptIn(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "scope.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	toolDir := t.TempDir()
	fakeGoLog := filepath.Join(t.TempDir(), "fake-go.log")
	fakeGo := filepath.Join(toolDir, "go")
	if err := os.WriteFile(fakeGo, []byte("#!/usr/bin/env sh\nprintf '%s\\n' \"$*\" >> \"$FAKE_GO_LOG\"\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake go: %v", err)
	}

	logDir := filepath.Join(t.TempDir(), "logs")
	cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
	cmd.Env = append(os.Environ(),
		"FAKE_GO_LOG="+fakeGoLog,
		"GOPROXY=https://goproxy.cn,direct",
		"WORKSPACE="+workspace,
		"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+logDir,
		"PATH="+toolDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected existing configured workspace to be refused, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "refusing existing non-empty workspace") {
		t.Fatalf("expected existing-workspace refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("production acceptance smoke must fail before invoking go, fake go log stat=%v", err)
	}
	if _, err := os.Stat(filepath.Join(logDir, "managed-payloads.txt")); !os.IsNotExist(err) {
		t.Fatalf("production acceptance smoke must not start managed payload smoke for refused workspace, stat=%v", err)
	}
}

func TestProductionAcceptanceSmokeRejectsUntrustedWorkspacePath(t *testing.T) {
	t.Run("relative", func(t *testing.T) {
		logDir := filepath.Join(t.TempDir(), "logs")
		cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
		cmd.Env = append(os.Environ(),
			"WORKSPACE=relative-workspace",
			"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+logDir,
		)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected relative production acceptance workspace to fail, got output:\n%s", output)
		}
		if !strings.Contains(string(output), "production acceptance workspace path must be absolute") {
			t.Fatalf("expected relative workspace refusal, got:\n%s", output)
		}
		assertProductionAcceptanceStageLogsAbsent(t, logDir)
	})

	t.Run("symlink leaf", func(t *testing.T) {
		tempDir := t.TempDir()
		realWorkspace := filepath.Join(tempDir, "real-workspace")
		if err := os.MkdirAll(realWorkspace, 0o755); err != nil {
			t.Fatalf("create real workspace: %v", err)
		}
		workspace := filepath.Join(tempDir, "workspace-link")
		if err := os.Symlink(realWorkspace, workspace); err != nil {
			t.Fatalf("symlink workspace: %v", err)
		}
		logDir := filepath.Join(t.TempDir(), "logs")
		cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
		cmd.Env = append(os.Environ(),
			"WORKSPACE="+workspace,
			"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+logDir,
		)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected symlink production acceptance workspace to fail, got output:\n%s", output)
		}
		if !strings.Contains(string(output), "production acceptance workspace directory must not be a symlink") {
			t.Fatalf("expected symlink workspace refusal, got:\n%s", output)
		}
		assertProductionAcceptanceStageLogsAbsent(t, logDir)
	})

	t.Run("symlink parent", func(t *testing.T) {
		tempDir := t.TempDir()
		outsideDir := filepath.Join(tempDir, "outside")
		if err := os.MkdirAll(outsideDir, 0o755); err != nil {
			t.Fatalf("create outside dir: %v", err)
		}
		linkParent := filepath.Join(tempDir, "link-parent")
		if err := os.Symlink(outsideDir, linkParent); err != nil {
			t.Fatalf("symlink workspace parent: %v", err)
		}
		workspace := filepath.Join(linkParent, "workspace")
		logDir := filepath.Join(t.TempDir(), "logs")
		cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
		cmd.Env = append(os.Environ(),
			"WORKSPACE="+workspace,
			"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+logDir,
		)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected symlink production acceptance workspace parent to fail, got output:\n%s", output)
		}
		if !strings.Contains(string(output), "production acceptance workspace path component must not be a symlink") {
			t.Fatalf("expected symlink workspace parent refusal, got:\n%s", output)
		}
		if _, err := os.Stat(filepath.Join(outsideDir, "workspace")); !os.IsNotExist(err) {
			t.Fatalf("production acceptance must not create workspace through symlink parent, stat=%v", err)
		}
		assertProductionAcceptanceStageLogsAbsent(t, logDir)
	})
}

func TestProductionAcceptanceSmokeAllowsDefaultWorkspaceUnderSymlinkTmpDir(t *testing.T) {
	tempDir := t.TempDir()
	realTmp := filepath.Join(tempDir, "real tmp")
	if err := os.MkdirAll(realTmp, 0o755); err != nil {
		t.Fatalf("create real tmp dir: %v", err)
	}
	tmpLink := filepath.Join(tempDir, "tmp-link")
	if err := os.Symlink(realTmp, tmpLink); err != nil {
		t.Fatalf("symlink tmp dir: %v", err)
	}
	logDir := filepath.Join(t.TempDir(), "logs")

	cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
	cmd.Env = append(os.Environ(),
		"TMPDIR="+tmpLink,
		"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected missing inventory failure with default workspace under symlink TMPDIR, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "OURO_PRODUCTION_ACCEPTANCE_INVENTORY is required") {
		t.Fatalf("expected default workspace under symlink TMPDIR to reach required inventory check, got:\n%s", output)
	}
	if strings.Contains(string(output), "production acceptance workspace path component must not be a symlink") {
		t.Fatalf("default workspace under symlink TMPDIR must use a physical path, got:\n%s", output)
	}
	assertProductionAcceptanceStageLogsAbsent(t, logDir)
	if _, err := os.Stat(filepath.Join(logDir, "summary.txt")); err != nil {
		t.Fatalf("expected failed summary for missing inventory with default workspace: %v", err)
	}
}

func TestProductionAcceptanceSmokeRejectsDuplicateInventoryFields(t *testing.T) {
	evidenceDir := t.TempDir()
	inventoryPath := filepath.Join(evidenceDir, "inventory.env")
	writeTextFile(t, inventoryPath, strings.Join([]string{
		"schema=ouroboros.production_acceptance.inventory.v1",
		"inventory_status=operator_owned",
		"inventory_status=fixture",
		"",
	}, "\n"))
	policyPath := filepath.Join(evidenceDir, "policy.env")
	writeTextFile(t, policyPath, validProductionAcceptancePolicyFixture())

	output, fakeGoLog := runRejectedProductionAcceptanceSmoke(t, inventoryPath, policyPath)
	if !strings.Contains(string(output), "inventory invalid: duplicate field inventory_status") {
		t.Fatalf("expected duplicate inventory field failure, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("production acceptance smoke must fail before invoking go, fake go log stat=%v", err)
	}
}

func TestProductionAcceptanceSmokeRejectsMalformedPolicyLines(t *testing.T) {
	evidenceDir := t.TempDir()
	inventoryPath := filepath.Join(evidenceDir, "inventory.env")
	writeTextFile(t, inventoryPath, strings.Join([]string{
		"schema=ouroboros.production_acceptance.inventory.v1",
		"inventory_status=operator_owned",
		"",
	}, "\n"))
	policyPath := filepath.Join(evidenceDir, "policy.env")
	writeTextFile(t, policyPath, validProductionAcceptancePolicyFixture()+"not a key value line\n")

	output, fakeGoLog := runRejectedProductionAcceptanceSmoke(t, inventoryPath, policyPath)
	if !strings.Contains(string(output), "policy invalid: malformed key=value line") {
		t.Fatalf("expected malformed policy line failure, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("production acceptance smoke must fail before invoking go, fake go log stat=%v", err)
	}
}

func TestProductionAcceptanceSmokeRejectsSymlinkInventoryFile(t *testing.T) {
	evidenceDir := t.TempDir()
	inventoryTarget := filepath.Join(evidenceDir, "inventory-target.env")
	writeTextFile(t, inventoryTarget, strings.Join([]string{
		"schema=ouroboros.production_acceptance.inventory.v1",
		"inventory_status=operator_owned",
		"",
	}, "\n"))
	inventoryLink := filepath.Join(evidenceDir, "inventory-link.env")
	symlinkFileForTest(t, inventoryTarget, inventoryLink)
	policyPath := filepath.Join(evidenceDir, "policy.env")
	writeTextFile(t, policyPath, validProductionAcceptancePolicyFixture())

	output, fakeGoLog := runRejectedProductionAcceptanceSmoke(t, inventoryLink, policyPath)
	if !strings.Contains(string(output), "inventory file must not be a symlink") {
		t.Fatalf("expected symlink inventory refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("production acceptance smoke must fail before invoking go, fake go log stat=%v", err)
	}
}

func TestProductionAcceptanceSmokeRejectsRelativeEvidencePaths(t *testing.T) {
	evidenceDir := t.TempDir()
	inventoryPath := filepath.Join(evidenceDir, "inventory.env")
	writeTextFile(t, inventoryPath, strings.Join([]string{
		"schema=ouroboros.production_acceptance.inventory.v1",
		"inventory_status=operator_owned",
		"",
	}, "\n"))
	policyPath := filepath.Join(evidenceDir, "policy.env")
	writeTextFile(t, policyPath, validProductionAcceptancePolicyFixture())

	output, fakeGoLog := runRejectedProductionAcceptanceSmoke(t, relativePathForTest(t, inventoryPath), policyPath)
	if !strings.Contains(string(output), "inventory path must be absolute") {
		t.Fatalf("expected relative inventory path refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("production acceptance smoke must fail before invoking go, fake go log stat=%v", err)
	}
}

func TestProductionAcceptanceSmokeRejectsEvidenceInputSymlinkPathComponents(t *testing.T) {
	cases := []struct {
		name          string
		symlinkedEnv  string
		expectedError string
	}{
		{
			name:          "inventory",
			symlinkedEnv:  "inventory",
			expectedError: "inventory path component must not be a symlink",
		},
		{
			name:          "deployment policy",
			symlinkedEnv:  "policy",
			expectedError: "deployment policy path component must not be a symlink",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidenceDir := t.TempDir()
			inventoryPath := filepath.Join(evidenceDir, "inventory.env")
			writeTextFile(t, inventoryPath, strings.Join([]string{
				"schema=ouroboros.production_acceptance.inventory.v1",
				"inventory_status=operator_owned",
				"",
			}, "\n"))
			policyPath := filepath.Join(evidenceDir, "policy.env")
			writeTextFile(t, policyPath, validProductionAcceptancePolicyFixture())
			switch tc.symlinkedEnv {
			case "inventory":
				inventoryPath = pathWithSymlinkAncestorForTest(t, evidenceDir, inventoryPath)
			case "policy":
				policyPath = pathWithSymlinkAncestorForTest(t, evidenceDir, policyPath)
			default:
				t.Fatalf("unhandled symlinked env %q", tc.symlinkedEnv)
			}

			output, fakeGoLog := runRejectedProductionAcceptanceSmoke(t, inventoryPath, policyPath)
			if !strings.Contains(string(output), tc.expectedError) {
				t.Fatalf("expected symlink component refusal %q, got:\n%s", tc.expectedError, output)
			}
			if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
				t.Fatalf("production acceptance smoke must fail before invoking go, fake go log stat=%v", err)
			}
		})
	}
}

func TestProductionAcceptanceSmokeRejectsSymlinkLogDir(t *testing.T) {
	tempDir := t.TempDir()
	outsideDir := filepath.Join(tempDir, "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("create outside dir: %v", err)
	}
	logDir := filepath.Join(tempDir, "logs")
	if err := os.Symlink(outsideDir, logDir); err != nil {
		t.Fatalf("symlink log dir: %v", err)
	}

	cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
	cmd.Env = append(os.Environ(),
		"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected symlink production acceptance log dir to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "production acceptance log dir directory must not be a symlink") {
		t.Fatalf("expected symlink log dir refusal, got:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "summary.txt")); !os.IsNotExist(err) {
		t.Fatalf("production acceptance must not write through symlink log dir, stat err=%v", err)
	}
}

func TestProductionAcceptanceSmokeRejectsSymlinkLogDirParent(t *testing.T) {
	tempDir := t.TempDir()
	outsideDir := filepath.Join(tempDir, "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("create outside dir: %v", err)
	}
	linkParent := filepath.Join(tempDir, "link-parent")
	if err := os.Symlink(outsideDir, linkParent); err != nil {
		t.Fatalf("symlink log dir parent: %v", err)
	}
	logDir := filepath.Join(linkParent, "logs")

	cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
	cmd.Env = append(os.Environ(),
		"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected symlink production acceptance log dir parent to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "production acceptance log dir path component must not be a symlink") {
		t.Fatalf("expected symlink log dir parent refusal, got:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "logs", "summary.txt")); !os.IsNotExist(err) {
		t.Fatalf("production acceptance must not write through symlink log dir parent, stat err=%v", err)
	}
}

func TestProductionAcceptanceSmokeAllowsDefaultLogDirUnderSymlinkTmpDir(t *testing.T) {
	tempDir := t.TempDir()
	realTmp := filepath.Join(tempDir, "real tmp")
	if err := os.MkdirAll(realTmp, 0o755); err != nil {
		t.Fatalf("create real tmp dir: %v", err)
	}
	tmpLink := filepath.Join(tempDir, "tmp-link")
	if err := os.Symlink(realTmp, tmpLink); err != nil {
		t.Fatalf("symlink tmp dir: %v", err)
	}

	cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
	cmd.Env = append(os.Environ(), "TMPDIR="+tmpLink)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected missing inventory failure under symlink TMPDIR, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "OURO_PRODUCTION_ACCEPTANCE_INVENTORY is required") {
		t.Fatalf("expected default log dir under symlink TMPDIR to reach required inventory check, got:\n%s", output)
	}
	if strings.Contains(string(output), "path component must not be a symlink") {
		t.Fatalf("default log dir under symlink TMPDIR must not fail path-component check, got:\n%s", output)
	}
}

func TestProductionAcceptanceSmokeRejectsSymlinkSummaryOutputPath(t *testing.T) {
	tempDir := t.TempDir()
	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("create log dir: %v", err)
	}
	outsideSummary := filepath.Join(tempDir, "outside-summary.txt")
	writeTextFile(t, outsideSummary, "outside sentinel\n")
	symlinkFileForTest(t, outsideSummary, filepath.Join(logDir, "summary.txt"))

	cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
	cmd.Env = append(os.Environ(),
		"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected symlink production acceptance summary path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "production acceptance summary file must not be a symlink") {
		t.Fatalf("expected symlink summary refusal, got:\n%s", output)
	}
	if got := readTextFileForTest(t, outsideSummary); got != "outside sentinel\n" {
		t.Fatalf("production acceptance must not overwrite outside summary through symlink, got:\n%s", got)
	}
}

func TestProductionAcceptanceSmokeRemovesStaleStageLogsWhenSummaryOutputRejected(t *testing.T) {
	tempDir := t.TempDir()
	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("create log dir: %v", err)
	}
	for _, name := range []string{"ide-smoke.txt", "release-managed.txt", "managed-payloads.txt", "workflow-closure.txt"} {
		writeTextFile(t, filepath.Join(logDir, name), "stale success log\n")
	}
	outsideSummary := filepath.Join(tempDir, "outside-summary.txt")
	writeTextFile(t, outsideSummary, "outside sentinel\n")
	symlinkFileForTest(t, outsideSummary, filepath.Join(logDir, "summary.txt"))

	cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
	cmd.Env = append(os.Environ(),
		"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected symlink production acceptance summary path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "production acceptance summary file must not be a symlink") {
		t.Fatalf("expected symlink summary refusal, got:\n%s", output)
	}
	if got := readTextFileForTest(t, outsideSummary); got != "outside sentinel\n" {
		t.Fatalf("production acceptance must not overwrite outside summary through symlink, got:\n%s", got)
	}
	for _, name := range []string{"ide-smoke.txt", "release-managed.txt", "managed-payloads.txt", "workflow-closure.txt"} {
		if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
			t.Fatalf("summary-output failure must remove stale stage log %s, stat err=%v", name, err)
		}
	}
}

func TestProductionAcceptanceSmokeRemovesStaleStageLogsWhenSummaryOutputDirectory(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(filepath.Join(logDir, "summary.txt"), 0o755); err != nil {
		t.Fatalf("create summary output directory: %v", err)
	}
	staleLogs := map[string]string{
		"ide-smoke.txt":        "ide-smoke: ok\n",
		"release-managed.txt":  "release-managed-smoke: ok\n",
		"managed-payloads.txt": "managed-payloads-smoke: ok\n",
		"workflow-closure.txt": "workflow-closure-smoke: ok\n",
	}
	for name, body := range staleLogs {
		writeTextFile(t, filepath.Join(logDir, name), body)
	}

	cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
	cmd.Env = append(os.Environ(),
		"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected directory production acceptance summary path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "production acceptance summary file must be a regular file or absent") {
		t.Fatalf("expected directory summary refusal, got:\n%s", output)
	}
	for name := range staleLogs {
		if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
			t.Fatalf("directory-summary failure must remove stale stage log %s, stat err=%v", name, err)
		}
	}
}

func TestProductionAcceptanceSmokeRejectsSymlinkStageLogOutputPath(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	tempDir := t.TempDir()
	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("create log dir: %v", err)
	}
	outsideStageLog := filepath.Join(tempDir, "outside-release-managed.txt")
	writeTextFile(t, outsideStageLog, "outside sentinel\n")
	symlinkFileForTest(t, outsideStageLog, filepath.Join(logDir, "release-managed.txt"))

	cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
	cmd.Env = append(os.Environ(),
		"GOPROXY=https://goproxy.cn,direct",
		"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+logDir,
		"OURO_PRODUCTION_ACCEPTANCE_INVENTORY="+evidence.InventoryPath,
		"OURO_PRODUCTION_ACCEPTANCE_POLICY="+evidence.PolicyPath,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected symlink stage log output path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "release managed log file must not be a symlink") {
		t.Fatalf("expected symlink stage log refusal, got:\n%s", output)
	}
	if got := readTextFileForTest(t, outsideStageLog); got != "outside sentinel\n" {
		t.Fatalf("production acceptance must not overwrite outside stage log through symlink, got:\n%s", got)
	}
}

func TestProductionAcceptanceSmokePreflightsStageLogOutputPaths(t *testing.T) {
	scriptBytes, err := os.ReadFile(filepath.Join("scripts", "smoke_production_acceptance.sh"))
	if err != nil {
		t.Fatalf("read production acceptance script: %v", err)
	}
	script := string(scriptBytes)
	for _, want := range []string{
		`assert_output_file_path "ide smoke log" "$IDE_SMOKE_LOG"`,
		`assert_output_file_path "release managed log" "$RELEASE_MANAGED_LOG"`,
		`assert_output_file_path "managed payloads log" "$MANAGED_PAYLOADS_LOG"`,
		`assert_output_file_path "workflow closure log" "$WORKFLOW_CLOSURE_LOG"`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("production acceptance script must preflight stage log output path with %q", want)
		}
	}
}

func TestProductionAcceptanceSmokePublishesStageLogsThroughTempFiles(t *testing.T) {
	scriptBytes, err := os.ReadFile(filepath.Join("scripts", "smoke_production_acceptance.sh"))
	if err != nil {
		t.Fatalf("read production acceptance script: %v", err)
	}
	script := string(scriptBytes)
	for _, forbidden := range []string{
		`> "$IDE_SMOKE_LOG" 2>&1`,
		`> "$RELEASE_MANAGED_LOG" 2>&1`,
		`> "$MANAGED_PAYLOADS_LOG" 2>&1`,
		`> "$WORKFLOW_CLOSURE_LOG" 2>&1`,
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("production acceptance stage logs must not redirect directly to final output %q", forbidden)
		}
	}
	for _, want := range []string{
		`IDE_SMOKE_LOG_TMP_PATH="$(mktemp "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/.ide-smoke.txt.XXXXXX")" || fail "unable to create IDE smoke log temporary file"`,
		`RELEASE_MANAGED_LOG_TMP_PATH="$(mktemp "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/.release-managed.txt.XXXXXX")" || fail "unable to create release managed log temporary file"`,
		`MANAGED_PAYLOADS_LOG_TMP_PATH="$(mktemp "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/.managed-payloads.txt.XXXXXX")" || fail "unable to create managed payloads log temporary file"`,
		`WORKFLOW_CLOSURE_LOG_TMP_PATH="$(mktemp "$OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR/.workflow-closure.txt.XXXXXX")" || fail "unable to create workflow closure log temporary file"`,
		`mv -f "$IDE_SMOKE_LOG_TMP_PATH" "$IDE_SMOKE_LOG" || fail "unable to publish IDE smoke log"`,
		`mv -f "$RELEASE_MANAGED_LOG_TMP_PATH" "$RELEASE_MANAGED_LOG" || fail "unable to publish release managed log"`,
		`mv -f "$MANAGED_PAYLOADS_LOG_TMP_PATH" "$MANAGED_PAYLOADS_LOG" || fail "unable to publish managed payloads log"`,
		`mv -f "$WORKFLOW_CLOSURE_LOG_TMP_PATH" "$WORKFLOW_CLOSURE_LOG" || fail "unable to publish workflow closure log"`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("production acceptance script must publish stage logs through temporary file with %q", want)
		}
	}
}

func TestProductionAcceptanceSmokeRemovesStaleUnpublishedStageLogsOnFailure(t *testing.T) {
	logDir := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("create log dir: %v", err)
	}
	for _, name := range []string{"ide-smoke.txt", "release-managed.txt", "managed-payloads.txt", "workflow-closure.txt"} {
		writeTextFile(t, filepath.Join(logDir, name), "stale success log\n")
	}

	cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
	cmd.Env = append(os.Environ(),
		"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected missing inventory failure, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "OURO_PRODUCTION_ACCEPTANCE_INVENTORY is required") {
		t.Fatalf("expected missing inventory failure, got:\n%s", output)
	}
	summary := readTextFileForTest(t, filepath.Join(logDir, "summary.txt"))
	if !strings.Contains(summary, "status=failed") {
		t.Fatalf("expected failed summary, got:\n%s", summary)
	}
	for _, name := range []string{"ide-smoke.txt", "release-managed.txt", "managed-payloads.txt", "workflow-closure.txt"} {
		if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
			t.Fatalf("expected stale unpublished stage log %s to be removed, stat err=%v", name, err)
		}
	}
}

func TestProductionAcceptanceSmokeRejectsRelativePayloadInventoryField(t *testing.T) {
	evidenceDir := t.TempDir()
	payloadDir := filepath.Join(t.TempDir(), "operator payloads")
	binaryPayload := filepath.Join(payloadDir, "bin", "httpx")
	if err := os.MkdirAll(filepath.Dir(binaryPayload), 0o755); err != nil {
		t.Fatalf("create binary payload dir: %v", err)
	}
	writeTextFile(t, binaryPayload, "#!/usr/bin/env sh\nexit 0\n")
	rulePayload := writeManagedPayloadsSmokeRuleBundleFixture(t, filepath.Join(payloadDir, "rules"), map[string]string{
		"http/example.yaml": "id: ouro-smoke-http\ninfo:\n  name: Ouro smoke HTTP\n  severity: info\n",
	})
	inventoryPath := filepath.Join(evidenceDir, "inventory.env")
	writeTextFile(t, inventoryPath, strings.Join([]string{
		"schema=ouroboros.production_acceptance.inventory.v1",
		"inventory_status=operator_owned",
		"binary_name=httpx",
		"binary_pack=web-api-pack",
		"binary_payload=" + relativePathForTest(t, binaryPayload),
		"binary_expected_sha256=" + fileSHA256ForTest(t, binaryPayload),
		"binary_version=operator-test",
		"binary_source=operator-local",
		"rule_bundle_name=nuclei-templates",
		"rule_bundle_pack=web-api-pack",
		"rule_bundle_payload=" + rulePayload,
		"rule_bundle_expected_sha256=" + digestManagedPayloadsSmokeDirectory(t, rulePayload),
		"rule_bundle_version=operator-test",
		"rule_bundle_source=operator-local",
		"",
	}, "\n"))
	policyPath := filepath.Join(evidenceDir, "policy.env")
	writeTextFile(t, policyPath, validProductionAcceptancePolicyFixture())

	output, fakeGoLog := runRejectedProductionAcceptanceSmoke(t, inventoryPath, policyPath)
	if !strings.Contains(string(output), "binary payload path must be absolute") {
		t.Fatalf("expected relative payload path refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("production acceptance smoke must fail before invoking go, fake go log stat=%v", err)
	}
}

func TestProductionAcceptanceSmokeRejectsPayloadSymlinkPathComponents(t *testing.T) {
	cases := []struct {
		name          string
		field         string
		expectedError string
	}{
		{
			name:          "binary payload",
			field:         "binary_payload",
			expectedError: "binary payload path component must not be a symlink",
		},
		{
			name:          "rule bundle payload",
			field:         "rule_bundle_payload",
			expectedError: "rule bundle payload path component must not be a symlink",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			evidenceDir := filepath.Dir(evidence.SummaryPath)
			inventoryPath := evidence.InventoryPath
			originalPayload := readEvidenceFieldForTest(t, inventoryPath, tc.field)
			symlinkedPayload := pathWithSymlinkAncestorForTest(t, evidenceDir, originalPayload)
			replaceTextFile(t, inventoryPath, tc.field+"="+originalPayload, tc.field+"="+symlinkedPayload)

			logDir := filepath.Join(t.TempDir(), "logs")
			if err := os.MkdirAll(filepath.Join(logDir, "release-managed.txt"), 0o755); err != nil {
				t.Fatalf("create release managed log directory tripwire: %v", err)
			}
			cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
			cmd.Env = append(os.Environ(),
				"GOPROXY=https://goproxy.cn,direct",
				"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+logDir,
				"OURO_PRODUCTION_ACCEPTANCE_INVENTORY="+inventoryPath,
				"OURO_PRODUCTION_ACCEPTANCE_POLICY="+evidence.PolicyPath,
			)
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("expected symlink component payload to fail, got output:\n%s", output)
			}
			if !strings.Contains(string(output), tc.expectedError) {
				t.Fatalf("expected symlink component refusal %q, got:\n%s", tc.expectedError, output)
			}
		})
	}
}

func TestProductionAcceptanceSmokeRejectsInvalidAcceptedAtTimestamp(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	writeTextFile(t, evidence.PolicyPath, strings.ReplaceAll(
		readTextFileForTest(t, evidence.PolicyPath),
		"accepted_at=2026-06-08T00:00:00Z",
		"accepted_at=not-rfc3339",
	))

	output, fakeGoLog := runRejectedProductionAcceptanceSmoke(t, evidence.InventoryPath, evidence.PolicyPath)
	if !strings.Contains(string(output), "accepted_at must be RFC3339") {
		t.Fatalf("expected invalid accepted_at timestamp refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("production acceptance smoke must fail before invoking go, fake go log stat=%v", err)
	}
}

func TestCompletionAuditScriptValidatesProductionSummaryAndAttestation(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})

	logDir := filepath.Join(t.TempDir(), "logs")
	resultPath := filepath.Join(logDir, "completion-audit.txt")
	manifestPath := filepath.Join(logDir, "completion-evidence-manifest.txt")
	manifestDigestPath := filepath.Join(logDir, "completion-evidence-manifest.sha256")
	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("completion audit script failed: %v\n%s", err, output)
	}
	text := string(output)
	for _, want := range []string{
		"completion-audit: production-summary " + evidence.SummaryPath,
		"completion-audit: attestation " + evidence.AttestationPath,
		"completion-audit: production_acceptance=accepted",
		"completion-audit: operator_attestation=approved",
		"completion-audit: status=ready-for-final-review",
		"completion-audit: result " + resultPath,
		"completion-audit: evidence-manifest " + manifestPath,
		"completion-audit: evidence-manifest-digest " + manifestDigestPath,
		"completion-audit: ok",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected completion audit output %q, got:\n%s", want, text)
		}
	}
	resultBytes, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read completion audit result: %v", err)
	}
	result := string(resultBytes)
	completionAuditScript := repoFilePathForTest(t, "scripts", "audit_completion.sh")
	for _, want := range []string{
		"schema=ouroboros.completion_audit.result.v1",
		"status=ready-for-final-review",
		"completion_audit_trusted_shell=" + trustedShellForTest(t),
		"completion_audit_trusted_path=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"completion_audit_script=" + completionAuditScript,
		"completion_audit_script_sha256=" + fileSHA256ForTest(t, completionAuditScript),
		"production_summary_sha256=",
		"attestation_sha256=",
		"evidence_manifest=" + manifestPath,
		"evidence_manifest_digest=" + manifestDigestPath,
		"inventory_sha256=",
		"policy_sha256=",
		"ide_smoke_acceptance=passed",
		"release_managed_acceptance=passed",
		"managed_payload_acceptance=passed",
		"workflow_closure_acceptance=passed",
		"ide_direct_use_script=" + repoFilePathForTest(t, "scripts", "smoke_direct_use.sh"),
		"ide_direct_use_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_direct_use.sh")),
		"ide_ai_provider_script=" + repoFilePathForTest(t, "scripts", "smoke_ai_provider.sh"),
		"ide_ai_provider_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_ai_provider.sh")),
		"ide_smoke_log=",
		"ide_smoke_log_sha256=",
		"ide_direct_use_log=",
		"ide_direct_use_log_sha256=",
		"ide_ai_provider_log=",
		"ide_ai_provider_log_sha256=",
		"remaining_manual_step=claim_100_after_real_operator_evidence_review",
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("expected completion audit result %q, got:\n%s", want, result)
		}
	}
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read completion audit evidence manifest: %v", err)
	}
	manifest := string(manifestBytes)
	workflowSelection := readTextFileForTest(t, evidence.WorkflowSelectionPath)
	workflowDispatch := readTextFileForTest(t, readWorkflowSelectionFieldForTest(t, workflowSelection, "dispatch_path"))
	workflowExecution := readTextFileForTest(t, readWorkflowSelectionFieldForTest(t, workflowSelection, "execution_path"))
	for _, want := range []string{
		"schema=ouroboros.completion_audit.evidence_manifest.v1",
		"status=ready-for-final-review",
		"result=" + resultPath,
		"result_sha256=" + fileSHA256ForTest(t, resultPath),
		"completion_audit_started_at=" + readEvidenceFieldForTest(t, resultPath, "started_at"),
		"completion_audit_completed_at=" + readEvidenceFieldForTest(t, resultPath, "completed_at"),
		"completion_audit_trusted_shell=" + trustedShellForTest(t),
		"completion_audit_trusted_path=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"completion_audit_script=" + completionAuditScript,
		"completion_audit_script_sha256=" + fileSHA256ForTest(t, completionAuditScript),
		"production_summary=" + evidence.SummaryPath,
		"production_summary_sha256=" + fileSHA256ForTest(t, evidence.SummaryPath),
		"production_summary_started_at=" + readEvidenceFieldForTest(t, evidence.SummaryPath, "started_at"),
		"production_summary_completed_at=" + readEvidenceFieldForTest(t, evidence.SummaryPath, "completed_at"),
		"attestation=" + evidence.AttestationPath,
		"attestation_sha256=" + fileSHA256ForTest(t, evidence.AttestationPath),
		"attestation_reviewed_by=" + readEvidenceFieldForTest(t, evidence.AttestationPath, "reviewed_by"),
		"attestation_reviewed_at=" + readEvidenceFieldForTest(t, evidence.AttestationPath, "reviewed_at"),
		"inventory=" + evidence.InventoryPath,
		"inventory_sha256=" + fileSHA256ForTest(t, evidence.InventoryPath),
		"policy=" + evidence.PolicyPath,
		"policy_sha256=" + fileSHA256ForTest(t, evidence.PolicyPath),
		"policy_accepted_at=" + readEvidenceFieldForTest(t, evidence.PolicyPath, "accepted_at"),
		"referenced_binary_payload=" + readEvidenceFieldForTest(t, evidence.InventoryPath, "binary_payload"),
		"referenced_binary_payload_sha256=" + readEvidenceFieldForTest(t, evidence.InventoryPath, "binary_expected_sha256"),
		"referenced_rule_bundle_payload=" + readEvidenceFieldForTest(t, evidence.InventoryPath, "rule_bundle_payload"),
		"referenced_rule_bundle_payload_sha256=" + readEvidenceFieldForTest(t, evidence.InventoryPath, "rule_bundle_expected_sha256"),
		"referenced_rule_bundle_payload_digest_scope=directory_canonical_records",
		"production_workspace=" + filepath.Dir(evidence.SummaryPath),
		"stage_workspace_relationship=fixed_production_children",
		"ide_smoke_workspace=" + filepath.Join(filepath.Dir(evidence.SummaryPath), "ide-smoke-workspace"),
		"ide_smoke_log=" + evidence.IdeLogPath,
		"ide_smoke_log_sha256=" + fileSHA256ForTest(t, evidence.IdeLogPath),
		"ide_direct_use_script=" + repoFilePathForTest(t, "scripts", "smoke_direct_use.sh"),
		"ide_direct_use_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_direct_use.sh")),
		"ide_direct_use_log=" + evidence.IdeDirectUseLogPath,
		"ide_direct_use_log_sha256=" + fileSHA256ForTest(t, evidence.IdeDirectUseLogPath),
		"ide_ai_provider_script=" + repoFilePathForTest(t, "scripts", "smoke_ai_provider.sh"),
		"ide_ai_provider_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_ai_provider.sh")),
		"ide_ai_provider_log=" + evidence.IdeAIProviderLogPath,
		"ide_ai_provider_log_sha256=" + fileSHA256ForTest(t, evidence.IdeAIProviderLogPath),
		"release_managed_workspace=" + filepath.Join(filepath.Dir(evidence.SummaryPath), "release-managed-workspace"),
		"release_managed_log=" + evidence.ReleaseLogPath,
		"release_managed_log_sha256=" + fileSHA256ForTest(t, evidence.ReleaseLogPath),
		"managed_payloads_workspace=" + filepath.Join(filepath.Dir(evidence.SummaryPath), "managed-payloads-workspace"),
		"managed_payloads_log=" + evidence.ManagedLogPath,
		"managed_payloads_log_sha256=" + fileSHA256ForTest(t, evidence.ManagedLogPath),
		"workflow_closure_workspace=" + filepath.Join(filepath.Dir(evidence.SummaryPath), "workflow-closure-workspace"),
		"workflow_closure_log=" + evidence.WorkflowLogPath,
		"workflow_closure_log_sha256=" + fileSHA256ForTest(t, evidence.WorkflowLogPath),
		"release_managed_rule_link=" + evidence.ReleaseRuleLinkPath,
		"release_managed_rule_target=" + evidence.ReleaseRulePayloadPath,
		"release_managed_rule_payload_sha256=" + digestManagedPayloadsSmokeDirectory(t, evidence.ReleaseRulePayloadPath),
		"release_managed_rule_payload_digest_scope=directory_canonical_records",
		"managed_payload_binary=" + evidence.ManagedBinaryPath,
		"managed_payload_binary_sha256=" + fileSHA256ForTest(t, evidence.ManagedBinaryPath),
		"workflow_report_selection=" + evidence.WorkflowSelectionPath,
		"workflow_report_selection_sha256=" + fileSHA256ForTest(t, evidence.WorkflowSelectionPath),
		"workflow_report_execution=" + readWorkflowSelectionFieldForTest(t, workflowSelection, "execution_path"),
		"workflow_report_execution_sha256=" + fileSHA256ForTest(t, readWorkflowSelectionFieldForTest(t, workflowSelection, "execution_path")),
		"workflow_report_dispatch=" + readWorkflowSelectionFieldForTest(t, workflowSelection, "dispatch_path"),
		"workflow_report_dispatch_sha256=" + fileSHA256ForTest(t, readWorkflowSelectionFieldForTest(t, workflowSelection, "dispatch_path")),
		"workflow_report_run_id=" + readWorkflowSelectionFieldForTest(t, workflowSelection, "run_id"),
		"workflow_report_path=" + readWorkflowSelectionFieldForTest(t, workflowSelection, "report_path"),
		"workflow_report_sha256=" + fileSHA256ForTest(t, readWorkflowSelectionFieldForTest(t, workflowSelection, "report_path")),
		"workflow_report_event_count=3",
		"workflow_report_dispatch_created_at=" + readWorkflowSelectionFieldForTest(t, workflowDispatch, "created_at"),
		"workflow_report_execution_executed_at=" + readWorkflowSelectionFieldForTest(t, workflowExecution, "executed_at"),
		"workflow_report_execution_reviewed_at=" + readWorkflowSelectionFieldForTest(t, workflowExecution, "reviewed_at"),
		"workflow_report_selected_at=" + readWorkflowSelectionFieldForTest(t, workflowSelection, "selected_at"),
		"remaining_manual_step=claim_100_after_real_operator_evidence_review",
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("expected completion audit evidence manifest %q, got:\n%s", want, manifest)
		}
	}
	manifestDigestBytes, err := os.ReadFile(manifestDigestPath)
	if err != nil {
		t.Fatalf("read completion audit evidence manifest digest: %v", err)
	}
	manifestDigest := string(manifestDigestBytes)
	for _, want := range []string{
		"schema=ouroboros.completion_audit.evidence_manifest_digest.v1",
		"status=ready-for-final-review",
		"result=" + resultPath,
		"result_sha256=" + fileSHA256ForTest(t, resultPath),
		"evidence_manifest=" + manifestPath,
		"evidence_manifest_sha256=" + fileSHA256ForTest(t, manifestPath),
		"completion_audit_started_at=" + readEvidenceFieldForTest(t, resultPath, "started_at"),
		"completion_audit_completed_at=" + readEvidenceFieldForTest(t, resultPath, "completed_at"),
		"attestation_reviewed_by=" + readEvidenceFieldForTest(t, evidence.AttestationPath, "reviewed_by"),
		"attestation_reviewed_at=" + readEvidenceFieldForTest(t, evidence.AttestationPath, "reviewed_at"),
		"completion_audit_trusted_shell=" + trustedShellForTest(t),
		"completion_audit_trusted_path=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"completion_audit_script=" + completionAuditScript,
		"completion_audit_script_sha256=" + fileSHA256ForTest(t, completionAuditScript),
		"remaining_manual_step=claim_100_after_real_operator_evidence_review",
	} {
		if !strings.Contains(manifestDigest, want) {
			t.Fatalf("expected completion audit evidence manifest digest %q, got:\n%s", want, manifestDigest)
		}
	}
}

func TestCompletionAuditScriptAllowsWorkflowDispatchEventCountBeforeSelection(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	workflowSelection := readTextFileForTest(t, evidence.WorkflowSelectionPath)
	dispatchPath := readWorkflowSelectionFieldForTest(t, workflowSelection, "dispatch_path")
	replaceTextFile(t, dispatchPath, `"event_count": 3,`, `"event_count": 2,`)

	output, err := runCompletionAuditScript(t, evidence)
	if err != nil {
		t.Fatalf("expected completion audit to accept earlier dispatch event count, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "completion-audit: ok") {
		t.Fatalf("expected successful completion audit output, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsWorkflowDispatchEventCountAfterSelection(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	workflowSelection := readTextFileForTest(t, evidence.WorkflowSelectionPath)
	dispatchPath := readWorkflowSelectionFieldForTest(t, workflowSelection, "dispatch_path")
	replaceTextFile(t, dispatchPath, `"event_count": 3,`, `"event_count": 4,`)

	logDir := filepath.Join(t.TempDir(), "logs")
	output, err := runCompletionAuditScriptWithLogDir(t, evidence, logDir)
	if err == nil {
		t.Fatalf("expected completion audit to reject dispatch event count after selection, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "workflow closure workspace report selection dispatch event_count must not exceed selection event_count") {
		t.Fatalf("expected dispatch event count refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsSymlinkLogDir(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	tempDir := t.TempDir()
	outsideDir := filepath.Join(tempDir, "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("create outside dir: %v", err)
	}
	logDir := filepath.Join(tempDir, "logs")
	if err := os.Symlink(outsideDir, logDir); err != nil {
		t.Fatalf("symlink log dir: %v", err)
	}

	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected symlink log dir to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "completion audit log dir directory must not be a symlink") {
		t.Fatalf("expected symlink log dir refusal, got:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "completion-audit.txt")); !os.IsNotExist(err) {
		t.Fatalf("completion audit must not write through symlink log dir, stat err=%v", err)
	}
}

func TestCompletionAuditScriptRejectsSymlinkLogDirParent(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	tempDir := t.TempDir()
	outsideDir := filepath.Join(tempDir, "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("create outside dir: %v", err)
	}
	linkParent := filepath.Join(tempDir, "link-parent")
	if err := os.Symlink(outsideDir, linkParent); err != nil {
		t.Fatalf("symlink log dir parent: %v", err)
	}
	logDir := filepath.Join(linkParent, "logs")

	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected symlink log dir parent to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "completion audit log dir path component must not be a symlink") {
		t.Fatalf("expected symlink log dir parent refusal, got:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "logs", "completion-audit.txt")); !os.IsNotExist(err) {
		t.Fatalf("completion audit must not write through symlink log dir parent, stat err=%v", err)
	}
}

func TestCompletionAuditScriptAllowsDefaultLogDirUnderSymlinkTmpDir(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	tempDir := t.TempDir()
	realTmp := filepath.Join(tempDir, "real tmp")
	if err := os.MkdirAll(realTmp, 0o755); err != nil {
		t.Fatalf("create real tmp dir: %v", err)
	}
	tmpLink := filepath.Join(tempDir, "tmp-link")
	if err := os.Symlink(realTmp, tmpLink); err != nil {
		t.Fatalf("symlink tmp dir: %v", err)
	}

	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"TMPDIR="+tmpLink,
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected default completion audit log dir under symlink TMPDIR to succeed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "completion-audit: ok") {
		t.Fatalf("expected completion audit success under symlink TMPDIR, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsSymlinkResultOutputPath(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	tempDir := t.TempDir()
	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("create log dir: %v", err)
	}
	outsideResult := filepath.Join(tempDir, "outside-result.txt")
	writeTextFile(t, outsideResult, "outside sentinel\n")
	symlinkFileForTest(t, outsideResult, filepath.Join(logDir, "completion-audit.txt"))

	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected symlink result output path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "completion audit result file must not be a symlink") {
		t.Fatalf("expected symlink result refusal, got:\n%s", output)
	}
	if got := readTextFileForTest(t, outsideResult); got != "outside sentinel\n" {
		t.Fatalf("completion audit must not overwrite outside result through symlink, got:\n%s", got)
	}
}

func TestCompletionAuditScriptRemovesStaleManifestWhenResultOutputRejected(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	tempDir := t.TempDir()
	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("create log dir: %v", err)
	}
	manifestPath := filepath.Join(logDir, "completion-evidence-manifest.txt")
	writeTextFile(t, manifestPath, strings.Join([]string{
		"schema=ouroboros.completion_audit.evidence_manifest.v1",
		"status=ready-for-final-review",
		"stale=true",
		"",
	}, "\n"))
	manifestDigestPath := filepath.Join(logDir, "completion-evidence-manifest.sha256")
	writeTextFile(t, manifestDigestPath, strings.Join([]string{
		"schema=ouroboros.completion_audit.evidence_manifest_digest.v1",
		"status=ready-for-final-review",
		"stale=true",
		"",
	}, "\n"))
	outsideResult := filepath.Join(tempDir, "outside-result.txt")
	writeTextFile(t, outsideResult, "outside sentinel\n")
	symlinkFileForTest(t, outsideResult, filepath.Join(logDir, "completion-audit.txt"))

	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected symlink result output path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "completion audit result file must not be a symlink") {
		t.Fatalf("expected symlink result refusal, got:\n%s", output)
	}
	if got := readTextFileForTest(t, outsideResult); got != "outside sentinel\n" {
		t.Fatalf("completion audit must not overwrite outside result through symlink, got:\n%s", got)
	}
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatalf("completion audit result-output failure must remove stale evidence manifest, stat err=%v", err)
	}
	if _, err := os.Stat(manifestDigestPath); !os.IsNotExist(err) {
		t.Fatalf("completion audit result-output failure must remove stale evidence manifest digest, stat err=%v", err)
	}
}

func TestCompletionAuditScriptRejectsDirectoryResultOutputPath(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	logDir := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(filepath.Join(logDir, "completion-audit.txt"), 0o755); err != nil {
		t.Fatalf("create result output directory: %v", err)
	}
	manifestPath := filepath.Join(logDir, "completion-evidence-manifest.txt")
	writeTextFile(t, manifestPath, strings.Join([]string{
		"schema=ouroboros.completion_audit.evidence_manifest.v1",
		"status=ready-for-final-review",
		"stale=true",
		"",
	}, "\n"))
	manifestDigestPath := filepath.Join(logDir, "completion-evidence-manifest.sha256")
	writeTextFile(t, manifestDigestPath, strings.Join([]string{
		"schema=ouroboros.completion_audit.evidence_manifest_digest.v1",
		"status=ready-for-final-review",
		"stale=true",
		"",
	}, "\n"))

	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected directory result output path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "completion audit result file must be a regular file or absent") {
		t.Fatalf("expected directory result refusal, got:\n%s", output)
	}
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatalf("completion audit directory-result failure must remove stale evidence manifest, stat err=%v", err)
	}
	if _, err := os.Stat(manifestDigestPath); !os.IsNotExist(err) {
		t.Fatalf("completion audit directory-result failure must remove stale evidence manifest digest, stat err=%v", err)
	}
}

func TestCompletionAuditScriptRejectsSymlinkManifestOutputPath(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	tempDir := t.TempDir()
	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("create log dir: %v", err)
	}
	outsideManifest := filepath.Join(tempDir, "outside-manifest.txt")
	writeTextFile(t, outsideManifest, "outside sentinel\n")
	symlinkFileForTest(t, outsideManifest, filepath.Join(logDir, "completion-evidence-manifest.txt"))

	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected symlink manifest output path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "completion audit manifest file must not be a symlink") {
		t.Fatalf("expected symlink manifest refusal, got:\n%s", output)
	}
	resultBytes, readErr := os.ReadFile(filepath.Join(logDir, "completion-audit.txt"))
	if readErr != nil {
		t.Fatalf("expected failed completion audit result for symlink manifest: %v", readErr)
	}
	if !strings.Contains(string(resultBytes), "status=failed") {
		t.Fatalf("expected failed result for symlink manifest, got:\n%s", resultBytes)
	}
	if got := readTextFileForTest(t, outsideManifest); got != "outside sentinel\n" {
		t.Fatalf("completion audit must not overwrite outside manifest through symlink, got:\n%s", got)
	}
}

func TestCompletionAuditScriptRejectsDirectoryManifestOutputPath(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	logDir := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(filepath.Join(logDir, "completion-evidence-manifest.txt"), 0o755); err != nil {
		t.Fatalf("create manifest output directory: %v", err)
	}

	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected directory manifest output path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "completion audit manifest file must be a regular file or absent") {
		t.Fatalf("expected directory manifest refusal, got:\n%s", output)
	}
	resultBytes, readErr := os.ReadFile(filepath.Join(logDir, "completion-audit.txt"))
	if readErr != nil {
		t.Fatalf("expected failed completion audit result for directory manifest: %v", readErr)
	}
	if !strings.Contains(string(resultBytes), "status=failed") {
		t.Fatalf("expected failed result for directory manifest, got:\n%s", resultBytes)
	}
}

func TestCompletionAuditScriptRejectsSymlinkManifestDigestOutputPath(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	tempDir := t.TempDir()
	logDir := filepath.Join(tempDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("create log dir: %v", err)
	}
	outsideDigest := filepath.Join(tempDir, "outside-manifest-digest.txt")
	writeTextFile(t, outsideDigest, "outside sentinel\n")
	symlinkFileForTest(t, outsideDigest, filepath.Join(logDir, "completion-evidence-manifest.sha256"))

	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected symlink manifest digest output path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "completion audit manifest digest file must not be a symlink") {
		t.Fatalf("expected symlink manifest digest refusal, got:\n%s", output)
	}
	resultBytes, readErr := os.ReadFile(filepath.Join(logDir, "completion-audit.txt"))
	if readErr != nil {
		t.Fatalf("expected failed completion audit result for symlink manifest digest: %v", readErr)
	}
	if !strings.Contains(string(resultBytes), "status=failed") {
		t.Fatalf("expected failed result for symlink manifest digest, got:\n%s", resultBytes)
	}
	if got := readTextFileForTest(t, outsideDigest); got != "outside sentinel\n" {
		t.Fatalf("completion audit must not overwrite outside manifest digest through symlink, got:\n%s", got)
	}
}

func TestCompletionAuditScriptRejectsDirectoryManifestDigestOutputPath(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	logDir := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(filepath.Join(logDir, "completion-evidence-manifest.sha256"), 0o755); err != nil {
		t.Fatalf("create manifest digest output directory: %v", err)
	}

	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected directory manifest digest output path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "completion audit manifest digest file must be a regular file or absent") {
		t.Fatalf("expected directory manifest digest refusal, got:\n%s", output)
	}
	resultBytes, readErr := os.ReadFile(filepath.Join(logDir, "completion-audit.txt"))
	if readErr != nil {
		t.Fatalf("expected failed completion audit result for directory manifest digest: %v", readErr)
	}
	if !strings.Contains(string(resultBytes), "status=failed") {
		t.Fatalf("expected failed result for directory manifest digest, got:\n%s", resultBytes)
	}
}

func TestCompletionAuditScriptFailsClosedForFixtureAttestation(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	if err := os.WriteFile(evidence.AttestationPath, []byte(strings.Join([]string{
		"schema=ouroboros.completion_audit.attestation.v1",
		"attestation_status=approved",
		"reviewed_by=operator",
		"reviewed_at=2026-06-08T00:02:00Z",
		"production_summary_sha256=" + fileSHA256ForTest(t, evidence.SummaryPath),
		"production_evidence=operator_owned",
		"fixture_evidence_allowed=true",
		"scope_reviewed=true",
		"safety_boundaries_reviewed=true",
		"release_managed_reviewed=true",
		"managed_payloads_reviewed=true",
		"workflow_closure_reviewed=true",
		"documentation_reviewed=true",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write attestation: %v", err)
	}

	logDir := filepath.Join(t.TempDir(), "logs")
	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected fixture attestation to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "fixture_evidence_allowed expected false got true") {
		t.Fatalf("expected fixture evidence refusal, got:\n%s", output)
	}
	resultBytes, readErr := os.ReadFile(filepath.Join(logDir, "completion-audit.txt"))
	if readErr != nil {
		t.Fatalf("expected failed completion audit result: %v", readErr)
	}
	if !strings.Contains(string(resultBytes), "status=failed") {
		t.Fatalf("expected failed result, got:\n%s", resultBytes)
	}
	if _, err := os.Stat(filepath.Join(logDir, "completion-evidence-manifest.txt")); !os.IsNotExist(err) {
		t.Fatalf("completion audit must not write evidence manifest on failure, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(logDir, "completion-evidence-manifest.sha256")); !os.IsNotExist(err) {
		t.Fatalf("completion audit must not write evidence manifest digest on failure, stat err=%v", err)
	}
}

func TestCompletionAuditScriptRemovesStaleManifestOnFailure(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	replaceTextFile(t, evidence.AttestationPath, "fixture_evidence_allowed=false", "fixture_evidence_allowed=true")

	logDir := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("create completion audit log dir: %v", err)
	}
	manifestPath := filepath.Join(logDir, "completion-evidence-manifest.txt")
	writeTextFile(t, manifestPath, strings.Join([]string{
		"schema=ouroboros.completion_audit.evidence_manifest.v1",
		"status=ready-for-final-review",
		"stale=true",
		"",
	}, "\n"))
	manifestDigestPath := filepath.Join(logDir, "completion-evidence-manifest.sha256")
	writeTextFile(t, manifestDigestPath, strings.Join([]string{
		"schema=ouroboros.completion_audit.evidence_manifest_digest.v1",
		"status=ready-for-final-review",
		"stale=true",
		"",
	}, "\n"))

	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+logDir,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected fixture attestation to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "fixture_evidence_allowed expected false got true") {
		t.Fatalf("expected fixture evidence refusal, got:\n%s", output)
	}
	resultBytes, readErr := os.ReadFile(filepath.Join(logDir, "completion-audit.txt"))
	if readErr != nil {
		t.Fatalf("expected failed completion audit result: %v", readErr)
	}
	if !strings.Contains(string(resultBytes), "status=failed") {
		t.Fatalf("expected failed result, got:\n%s", resultBytes)
	}
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatalf("completion audit failure must remove stale evidence manifest, stat err=%v", err)
	}
	if _, err := os.Stat(manifestDigestPath); !os.IsNotExist(err) {
		t.Fatalf("completion audit failure must remove stale evidence manifest digest, stat err=%v", err)
	}
}

func TestCompletionAuditScriptRejectsMismatchedStageLogDigest(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{
		ReleaseLogDigestOverride: strings.Repeat("a", 64),
	})
	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+filepath.Join(t.TempDir(), "logs"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected mismatched log digest to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "release_managed_log_sha256 mismatch") {
		t.Fatalf("expected release log digest mismatch, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsIDEChildDirectUseLogWithoutChatEntrypointMarker(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	oldDigest := fileSHA256ForTest(t, evidence.IdeDirectUseLogPath)
	writeTextFile(t, evidence.IdeDirectUseLogPath, "direct-use-smoke: ok\n")
	newDigest := fileSHA256ForTest(t, evidence.IdeDirectUseLogPath)
	replaceTextFile(t, evidence.SummaryPath, "ide_direct_use_log_sha256="+oldDigest, "ide_direct_use_log_sha256="+newDigest)
	updateCompletionAuditAttestationSummaryDigest(t, evidence)

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected missing direct chat marker to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "ide_direct_use_log missing required marker: chat: available") {
		t.Fatalf("expected missing chat marker refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsIDEChildDirectUseLogWithoutChatComposerMarker(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	oldDigest := fileSHA256ForTest(t, evidence.IdeDirectUseLogPath)
	writeTextFile(t, evidence.IdeDirectUseLogPath, "chat: available\ndirect-use-smoke: ok\n")
	newDigest := fileSHA256ForTest(t, evidence.IdeDirectUseLogPath)
	replaceTextFile(t, evidence.SummaryPath, "ide_direct_use_log_sha256="+oldDigest, "ide_direct_use_log_sha256="+newDigest)
	updateCompletionAuditAttestationSummaryDigest(t, evidence)

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected missing direct chat composer marker to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "ide_direct_use_log missing required marker: chat: composer") {
		t.Fatalf("expected missing chat composer marker refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsStageLogWithoutOKSentinel(t *testing.T) {
	cases := []struct {
		name        string
		logPath     func(completionAuditEvidence) string
		summaryKey  string
		requiredOK  string
		expectedErr string
	}{
		{
			name:        "IDE smoke",
			logPath:     func(e completionAuditEvidence) string { return e.IdeLogPath },
			summaryKey:  "ide_smoke_log_sha256",
			requiredOK:  "ide-smoke: ok",
			expectedErr: "ide_smoke_log missing required marker",
		},
		{
			name:        "IDE direct-use child",
			logPath:     func(e completionAuditEvidence) string { return e.IdeDirectUseLogPath },
			summaryKey:  "ide_direct_use_log_sha256",
			requiredOK:  "direct-use-smoke: ok",
			expectedErr: "ide_direct_use_log missing required marker",
		},
		{
			name:        "IDE AI-provider child",
			logPath:     func(e completionAuditEvidence) string { return e.IdeAIProviderLogPath },
			summaryKey:  "ide_ai_provider_log_sha256",
			requiredOK:  "chat-prompt: preview=created stored=false",
			expectedErr: "ide_ai_provider_log missing required marker",
		},
		{
			name:        "release managed",
			logPath:     func(e completionAuditEvidence) string { return e.ReleaseLogPath },
			summaryKey:  "release_managed_log_sha256",
			requiredOK:  "release-managed-smoke: ok",
			expectedErr: "release_managed_log missing required marker",
		},
		{
			name:        "managed payloads",
			logPath:     func(e completionAuditEvidence) string { return e.ManagedLogPath },
			summaryKey:  "managed_payloads_log_sha256",
			requiredOK:  "managed-payloads-smoke: ok",
			expectedErr: "managed_payloads_log missing required marker",
		},
		{
			name:        "workflow closure",
			logPath:     func(e completionAuditEvidence) string { return e.WorkflowLogPath },
			summaryKey:  "workflow_closure_log_sha256",
			requiredOK:  "workflow-closure-smoke: ok",
			expectedErr: "workflow_closure_log missing required marker",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			logPath := tc.logPath(evidence)
			oldDigest := fileSHA256ForTest(t, logPath)
			writeTextFile(t, logPath, "stage ended without the expected sentinel\n")
			newDigest := fileSHA256ForTest(t, logPath)
			replaceTextFile(t, evidence.SummaryPath, tc.summaryKey+"="+oldDigest, tc.summaryKey+"="+newDigest)
			updateCompletionAuditAttestationSummaryDigest(t, evidence)

			output, err := runCompletionAuditScript(t, evidence)
			if err == nil {
				t.Fatalf("expected stage log without %q to fail, got output:\n%s", tc.requiredOK, output)
			}
			if !strings.Contains(string(output), tc.expectedErr) {
				t.Fatalf("expected missing marker failure %q, got:\n%s", tc.expectedErr, output)
			}
		})
	}
}

func TestCompletionAuditScriptRequiresCanonicalStageLogPaths(t *testing.T) {
	cases := []struct {
		name        string
		field       string
		logPath     func(completionAuditEvidence) string
		fileName    string
		expectedErr string
	}{
		{
			name:        "IDE smoke",
			field:       "ide_smoke_log",
			logPath:     func(e completionAuditEvidence) string { return e.IdeLogPath },
			fileName:    "ide-smoke.txt",
			expectedErr: "ide_smoke_log must reference expected production acceptance log",
		},
		{
			name:        "release managed",
			field:       "release_managed_log",
			logPath:     func(e completionAuditEvidence) string { return e.ReleaseLogPath },
			fileName:    "release-managed.txt",
			expectedErr: "release_managed_log must reference expected production acceptance log",
		},
		{
			name:        "managed payloads",
			field:       "managed_payloads_log",
			logPath:     func(e completionAuditEvidence) string { return e.ManagedLogPath },
			fileName:    "managed-payloads.txt",
			expectedErr: "managed_payloads_log must reference expected production acceptance log",
		},
		{
			name:        "workflow closure",
			field:       "workflow_closure_log",
			logPath:     func(e completionAuditEvidence) string { return e.WorkflowLogPath },
			fileName:    "workflow-closure.txt",
			expectedErr: "workflow_closure_log must reference expected production acceptance log",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			originalLog := tc.logPath(evidence)
			outsideLog := filepath.Join(t.TempDir(), tc.fileName)
			writeTextFile(t, outsideLog, readTextFileForTest(t, originalLog))
			replaceTextFile(t, evidence.SummaryPath, tc.field+"="+originalLog, tc.field+"="+outsideLog)
			updateCompletionAuditAttestationSummaryDigest(t, evidence)

			output, err := runCompletionAuditScript(t, evidence)
			if err == nil {
				t.Fatalf("expected non-canonical stage log path to fail, got output:\n%s", output)
			}
			if !strings.Contains(string(output), tc.expectedErr) {
				t.Fatalf("expected canonical stage log path failure %q, got:\n%s", tc.expectedErr, output)
			}
		})
	}
}

func TestCompletionAuditScriptRequiresCanonicalIDEChildLogPaths(t *testing.T) {
	cases := []struct {
		name        string
		field       string
		logPath     func(completionAuditEvidence) string
		expectedErr string
	}{
		{
			name:        "IDE direct-use child",
			field:       "ide_direct_use_log",
			logPath:     func(e completionAuditEvidence) string { return e.IdeDirectUseLogPath },
			expectedErr: "ide_direct_use_log must reference expected production acceptance log",
		},
		{
			name:        "IDE AI-provider child",
			field:       "ide_ai_provider_log",
			logPath:     func(e completionAuditEvidence) string { return e.IdeAIProviderLogPath },
			expectedErr: "ide_ai_provider_log must reference expected production acceptance log",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			originalLog := tc.logPath(evidence)
			outsideLog := filepath.Join(t.TempDir(), filepath.Base(originalLog))
			writeTextFile(t, outsideLog, readTextFileForTest(t, originalLog))
			replaceTextFile(t, evidence.SummaryPath, tc.field+"="+originalLog, tc.field+"="+outsideLog)
			updateCompletionAuditAttestationSummaryDigest(t, evidence)

			output, err := runCompletionAuditScript(t, evidence)
			if err == nil {
				t.Fatalf("expected non-canonical IDE child log path to fail, got output:\n%s", output)
			}
			if !strings.Contains(string(output), tc.expectedErr) {
				t.Fatalf("expected canonical IDE child log path failure %q, got:\n%s", tc.expectedErr, output)
			}
		})
	}
}

func TestCompletionAuditScriptRejectsNonCanonicalStageLogFileNames(t *testing.T) {
	cases := []struct {
		name        string
		field       string
		logPath     func(completionAuditEvidence) string
		fileName    string
		expectedErr string
	}{
		{
			name:        "IDE smoke",
			field:       "ide_smoke_log",
			logPath:     func(e completionAuditEvidence) string { return e.IdeLogPath },
			fileName:    "ide-smoke-copy.txt",
			expectedErr: "ide_smoke_log must reference expected production acceptance log",
		},
		{
			name:        "release managed",
			field:       "release_managed_log",
			logPath:     func(e completionAuditEvidence) string { return e.ReleaseLogPath },
			fileName:    "release-managed-copy.txt",
			expectedErr: "release_managed_log must reference expected production acceptance log",
		},
		{
			name:        "managed payloads",
			field:       "managed_payloads_log",
			logPath:     func(e completionAuditEvidence) string { return e.ManagedLogPath },
			fileName:    "managed-payloads-copy.txt",
			expectedErr: "managed_payloads_log must reference expected production acceptance log",
		},
		{
			name:        "workflow closure",
			field:       "workflow_closure_log",
			logPath:     func(e completionAuditEvidence) string { return e.WorkflowLogPath },
			fileName:    "workflow-closure-copy.txt",
			expectedErr: "workflow_closure_log must reference expected production acceptance log",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			originalLog := tc.logPath(evidence)
			alternateLog := filepath.Join(filepath.Dir(evidence.SummaryPath), tc.fileName)
			writeTextFile(t, alternateLog, readTextFileForTest(t, originalLog))
			replaceTextFile(t, evidence.SummaryPath, tc.field+"="+originalLog, tc.field+"="+alternateLog)
			updateCompletionAuditAttestationSummaryDigest(t, evidence)

			output, err := runCompletionAuditScript(t, evidence)
			if err == nil {
				t.Fatalf("expected non-canonical stage log filename to fail, got output:\n%s", output)
			}
			if !strings.Contains(string(output), tc.expectedErr) {
				t.Fatalf("expected canonical stage log path failure %q, got:\n%s", tc.expectedErr, output)
			}
		})
	}
}

func TestCompletionAuditScriptRequiresNoLiveTargetsSummaryFlag(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{
		OmitNoLiveTargets: true,
	})
	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+filepath.Join(t.TempDir(), "logs"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected missing no_live_targets to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "missing required field no_live_targets") {
		t.Fatalf("expected missing no_live_targets failure, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRequiresAttestationToBindProductionSummary(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{
		AttestationSummaryDigestOverride: strings.Repeat("b", 64),
	})
	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+filepath.Join(t.TempDir(), "logs"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected unbound attestation to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "production_summary_sha256 expected") {
		t.Fatalf("expected production summary binding failure, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsDuplicateProductionSummaryFields(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	appendTextFile(t, evidence.SummaryPath, "status=failed\n")
	updateCompletionAuditAttestationSummaryDigest(t, evidence)

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected duplicate summary field to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "production summary invalid: duplicate field status") {
		t.Fatalf("expected duplicate summary field failure, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsDuplicateAttestationFields(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	appendTextFile(t, evidence.AttestationPath, "fixture_evidence_allowed=true\n")

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected duplicate attestation field to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "completion attestation invalid: duplicate field fixture_evidence_allowed") {
		t.Fatalf("expected duplicate attestation field failure, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsMalformedEvidenceLines(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	appendTextFile(t, evidence.AttestationPath, "not a key value line\n")

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected malformed attestation line to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "completion attestation invalid: malformed key=value line") {
		t.Fatalf("expected malformed attestation line failure, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsSymlinkProductionSummary(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	summaryLink := filepath.Join(filepath.Dir(evidence.SummaryPath), "summary-link.txt")
	symlinkFileForTest(t, evidence.SummaryPath, summaryLink)
	evidence.SummaryPath = summaryLink

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected symlink summary to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "production summary file must not be a symlink") {
		t.Fatalf("expected symlink summary refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsSymlinkStageLog(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	releaseLogLink := filepath.Join(filepath.Dir(evidence.ReleaseLogPath), "release-managed-link.txt")
	symlinkFileForTest(t, evidence.ReleaseLogPath, releaseLogLink)
	replaceTextFile(t, evidence.SummaryPath, "release_managed_log="+evidence.ReleaseLogPath, "release_managed_log="+releaseLogLink)
	updateCompletionAuditAttestationSummaryDigest(t, evidence)

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected symlink stage log to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "release_managed_log_sha256 file must not be a symlink") {
		t.Fatalf("expected symlink stage log refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsRelativeProductionSummaryPath(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	evidence.SummaryPath = relativePathForTest(t, evidence.SummaryPath)

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected relative summary path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "production summary path must be absolute") {
		t.Fatalf("expected relative summary path refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsTopLevelEvidenceSymlinkPathComponents(t *testing.T) {
	cases := []struct {
		name          string
		apply         func(t *testing.T, evidence *completionAuditEvidence)
		expectedError string
	}{
		{
			name: "production summary",
			apply: func(t *testing.T, evidence *completionAuditEvidence) {
				evidenceDir := filepath.Dir(evidence.SummaryPath)
				evidence.SummaryPath = pathWithSymlinkAncestorForTest(t, evidenceDir, evidence.SummaryPath)
			},
			expectedError: "production summary path component must not be a symlink",
		},
		{
			name: "completion attestation",
			apply: func(t *testing.T, evidence *completionAuditEvidence) {
				evidenceDir := filepath.Dir(evidence.AttestationPath)
				evidence.AttestationPath = pathWithSymlinkAncestorForTest(t, evidenceDir, evidence.AttestationPath)
			},
			expectedError: "completion attestation path component must not be a symlink",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			tc.apply(t, &evidence)

			output, err := runCompletionAuditScript(t, evidence)
			if err == nil {
				t.Fatalf("expected top-level evidence symlink component to fail, got output:\n%s", output)
			}
			if !strings.Contains(string(output), tc.expectedError) {
				t.Fatalf("expected symlink component refusal %q, got:\n%s", tc.expectedError, output)
			}
		})
	}
}

func TestCompletionAuditScriptRejectsRelativeStageLogPath(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	relativeReleaseLog := relativePathForTest(t, evidence.ReleaseLogPath)
	replaceTextFile(t, evidence.SummaryPath, "release_managed_log="+evidence.ReleaseLogPath, "release_managed_log="+relativeReleaseLog)
	updateCompletionAuditAttestationSummaryDigest(t, evidence)

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected relative stage log path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "release_managed_log_sha256 path must be absolute") {
		t.Fatalf("expected relative stage log path refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsRelativeReferencedPayloadPath(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	inventoryPath := filepath.Join(filepath.Dir(evidence.SummaryPath), "inventory.env")
	relativeBinary := relativePathForTest(t, filepath.Join(filepath.Dir(evidence.SummaryPath), "httpx"))
	replaceTextFile(t, inventoryPath, "binary_payload="+filepath.Join(filepath.Dir(evidence.SummaryPath), "httpx"), "binary_payload="+relativeBinary)

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected relative referenced payload path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "referenced binary payload path must be absolute") {
		t.Fatalf("expected relative referenced payload refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsReferencedEvidenceSymlinkPathComponents(t *testing.T) {
	cases := []struct {
		name          string
		field         string
		path          func(completionAuditEvidence) string
		expectedError string
	}{
		{
			name:          "inventory",
			field:         "inventory",
			path:          func(e completionAuditEvidence) string { return e.InventoryPath },
			expectedError: "referenced inventory path component must not be a symlink",
		},
		{
			name:          "policy",
			field:         "policy",
			path:          func(e completionAuditEvidence) string { return e.PolicyPath },
			expectedError: "referenced policy path component must not be a symlink",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			evidenceDir := filepath.Dir(evidence.SummaryPath)
			originalPath := tc.path(evidence)
			symlinkedPath := pathWithSymlinkAncestorForTest(t, evidenceDir, originalPath)
			replaceTextFile(t, evidence.SummaryPath, tc.field+"="+originalPath, tc.field+"="+symlinkedPath)
			updateCompletionAuditAttestationSummaryDigest(t, evidence)

			output, err := runCompletionAuditScript(t, evidence)
			if err == nil {
				t.Fatalf("expected referenced evidence symlink component to fail, got output:\n%s", output)
			}
			if !strings.Contains(string(output), tc.expectedError) {
				t.Fatalf("expected symlink component refusal %q, got:\n%s", tc.expectedError, output)
			}
		})
	}
}

func TestCompletionAuditScriptRejectsReferencedPayloadSymlinkPathComponents(t *testing.T) {
	cases := []struct {
		name          string
		field         string
		expectedError string
	}{
		{
			name:          "binary payload",
			field:         "binary_payload",
			expectedError: "referenced binary payload path component must not be a symlink",
		},
		{
			name:          "rule bundle payload",
			field:         "rule_bundle_payload",
			expectedError: "referenced rule bundle payload path component must not be a symlink",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			evidenceDir := filepath.Dir(evidence.SummaryPath)
			originalPayload := readEvidenceFieldForTest(t, evidence.InventoryPath, tc.field)
			symlinkedPayload := pathWithSymlinkAncestorForTest(t, evidenceDir, originalPayload)
			replaceTextFile(t, evidence.InventoryPath, tc.field+"="+originalPayload, tc.field+"="+symlinkedPayload)
			updateCompletionAuditSummaryDigestField(t, evidence, "inventory_sha256", evidence.InventoryPath)

			output, err := runCompletionAuditScript(t, evidence)
			if err == nil {
				t.Fatalf("expected referenced payload symlink component to fail, got output:\n%s", output)
			}
			if !strings.Contains(string(output), tc.expectedError) {
				t.Fatalf("expected symlink component refusal %q, got:\n%s", tc.expectedError, output)
			}
		})
	}
}

func TestCompletionAuditScriptRejectsReferencedOperatorPayloadDigestMismatch(t *testing.T) {
	cases := []struct {
		name          string
		tamper        func(t *testing.T, evidence completionAuditEvidence)
		expectedError string
	}{
		{
			name: "binary payload",
			tamper: func(t *testing.T, evidence completionAuditEvidence) {
				t.Helper()
				binaryPayload := readEvidenceFieldForTest(t, evidence.InventoryPath, "binary_payload")
				writeTextFile(t, binaryPayload, "tampered operator binary payload\n")
			},
			expectedError: "referenced binary payload mismatch",
		},
		{
			name: "rule bundle payload",
			tamper: func(t *testing.T, evidence completionAuditEvidence) {
				t.Helper()
				rulePayload := readEvidenceFieldForTest(t, evidence.InventoryPath, "rule_bundle_payload")
				writeTextFile(t, filepath.Join(rulePayload, "template.yaml"), "id: tampered-operator-template\n")
			},
			expectedError: "referenced rule bundle payload mismatch",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			tc.tamper(t, evidence)

			logDir := filepath.Join(t.TempDir(), "logs")
			output, err := runCompletionAuditScriptWithLogDir(t, evidence, logDir)
			if err == nil {
				t.Fatalf("expected referenced operator payload digest mismatch to fail, got output:\n%s", output)
			}
			if !strings.Contains(string(output), tc.expectedError) {
				t.Fatalf("expected referenced operator payload digest refusal %q, got:\n%s", tc.expectedError, output)
			}
			for _, name := range []string{"completion-evidence-manifest.txt", "completion-evidence-manifest.sha256"} {
				if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
					t.Fatalf("referenced operator payload digest failure must not leave %s, stat=%v", name, err)
				}
			}
		})
	}
}

func TestCompletionAuditScriptRejectsSummaryPayloadFieldsMismatch(t *testing.T) {
	cases := []struct {
		name          string
		field         string
		originalValue func(t *testing.T, evidence completionAuditEvidence) string
		mismatchValue func(original string) string
		expectedError string
	}{
		{
			name:  "binary payload name",
			field: "binary_name",
			originalValue: func(t *testing.T, evidence completionAuditEvidence) string {
				t.Helper()
				return readEvidenceFieldForTest(t, evidence.InventoryPath, "binary_name")
			},
			mismatchValue: func(original string) string {
				return original + "-tampered"
			},
			expectedError: "binary_name expected",
		},
		{
			name:  "binary payload digest",
			field: "binary_expected_sha256",
			originalValue: func(t *testing.T, evidence completionAuditEvidence) string {
				t.Helper()
				return fileSHA256ForTest(t, readEvidenceFieldForTest(t, evidence.InventoryPath, "binary_payload"))
			},
			mismatchValue: func(original string) string {
				mismatched := strings.Repeat("0", 64)
				if original == mismatched {
					mismatched = strings.Repeat("1", 64)
				}
				return mismatched
			},
			expectedError: "binary_expected_sha256 expected",
		},
		{
			name:  "rule bundle payload name",
			field: "rule_bundle_name",
			originalValue: func(t *testing.T, evidence completionAuditEvidence) string {
				t.Helper()
				return readEvidenceFieldForTest(t, evidence.InventoryPath, "rule_bundle_name")
			},
			mismatchValue: func(original string) string {
				return original + "-tampered"
			},
			expectedError: "rule_bundle_name expected",
		},
		{
			name:  "rule bundle payload digest",
			field: "rule_bundle_expected_sha256",
			originalValue: func(t *testing.T, evidence completionAuditEvidence) string {
				t.Helper()
				return digestManagedPayloadsSmokeDirectory(t, readEvidenceFieldForTest(t, evidence.InventoryPath, "rule_bundle_payload"))
			},
			mismatchValue: func(original string) string {
				mismatched := strings.Repeat("0", 64)
				if original == mismatched {
					mismatched = strings.Repeat("1", 64)
				}
				return mismatched
			},
			expectedError: "rule_bundle_expected_sha256 expected",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			original := tc.originalValue(t, evidence)
			mismatched := tc.mismatchValue(original)
			replaceTextFile(t, evidence.SummaryPath, tc.field+"="+original, tc.field+"="+mismatched)
			updateCompletionAuditAttestationSummaryDigest(t, evidence)

			logDir := filepath.Join(t.TempDir(), "logs")
			output, err := runCompletionAuditScriptWithLogDir(t, evidence, logDir)
			if err == nil {
				t.Fatalf("expected summary payload digest mismatch to fail, got output:\n%s", output)
			}
			if !strings.Contains(string(output), tc.expectedError) {
				t.Fatalf("expected summary payload digest refusal %q, got:\n%s", tc.expectedError, output)
			}
			for _, name := range []string{"completion-evidence-manifest.txt", "completion-evidence-manifest.sha256"} {
				if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
					t.Fatalf("summary payload digest failure must not leave %s, stat=%v", name, err)
				}
			}
		})
	}
}

func TestCompletionAuditScriptRejectsStageLogSymlinkPathComponents(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	evidenceDir := filepath.Dir(evidence.SummaryPath)
	symlinkedReleaseLog := pathWithSymlinkAncestorForTest(t, evidenceDir, evidence.ReleaseLogPath)
	replaceTextFile(t, evidence.SummaryPath, "release_managed_log="+evidence.ReleaseLogPath, "release_managed_log="+symlinkedReleaseLog)
	updateCompletionAuditAttestationSummaryDigest(t, evidence)

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected stage log symlink component to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "release_managed_log_sha256 path component must not be a symlink") {
		t.Fatalf("expected stage log symlink component refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsRecordedScriptSymlinkPathComponents(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	symlinkedAuditScript := repoPathWithSymlinkAncestorForTest(t, repoFilePathForTest(t, "scripts", "audit_completion.sh"))
	symlinkedRepoRoot := filepath.Dir(filepath.Dir(symlinkedAuditScript))
	originalProductionScript := repoFilePathForTest(t, "scripts", "smoke_production_acceptance.sh")
	symlinkedProductionScript := filepath.Join(symlinkedRepoRoot, "scripts", "smoke_production_acceptance.sh")
	replaceTextFile(t, evidence.SummaryPath, "production_acceptance_script="+originalProductionScript, "production_acceptance_script="+symlinkedProductionScript)
	updateCompletionAuditAttestationSummaryDigest(t, evidence)

	cmd := exec.Command("sh", symlinkedAuditScript)
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+filepath.Join(t.TempDir(), "logs"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected recorded script symlink component to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "production_acceptance_script path component must not be a symlink") {
		t.Fatalf("expected recorded script symlink component refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsStageArtifactSymlinkPathComponents(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	reportsDir := filepath.Dir(evidence.WorkflowSelectionPath)
	realReportsDir := filepath.Join(filepath.Dir(reportsDir), "reports-real")
	if err := os.Rename(reportsDir, realReportsDir); err != nil {
		t.Fatalf("rename workflow reports dir: %v", err)
	}
	if err := os.Symlink(realReportsDir, reportsDir); err != nil {
		t.Fatalf("symlink workflow reports dir: %v", err)
	}

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected stage artifact symlink component to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "workflow closure workspace report selection path component must not be a symlink") {
		t.Fatalf("expected stage artifact symlink component refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsStageWorkspaceSymlinkPathComponents(t *testing.T) {
	cases := []struct {
		name          string
		field         string
		originalPath  func(completionAuditEvidence) string
		symlinkedPath func(linkParent, evidenceDir string) string
	}{
		{
			name:         "production workspace",
			field:        "workspace",
			originalPath: func(e completionAuditEvidence) string { return filepath.Dir(e.SummaryPath) },
			symlinkedPath: func(linkParent, evidenceDir string) string {
				return filepath.Join(linkParent, filepath.Base(evidenceDir))
			},
		},
		{
			name:  "release managed workspace",
			field: "release_managed_workspace",
			originalPath: func(e completionAuditEvidence) string {
				return filepath.Join(filepath.Dir(e.SummaryPath), "release-managed-workspace")
			},
			symlinkedPath: func(linkParent, evidenceDir string) string {
				return filepath.Join(linkParent, filepath.Base(evidenceDir), "release-managed-workspace")
			},
		},
		{
			name:  "managed payloads workspace",
			field: "managed_payloads_workspace",
			originalPath: func(e completionAuditEvidence) string {
				return filepath.Join(filepath.Dir(e.SummaryPath), "managed-payloads-workspace")
			},
			symlinkedPath: func(linkParent, evidenceDir string) string {
				return filepath.Join(linkParent, filepath.Base(evidenceDir), "managed-payloads-workspace")
			},
		},
		{
			name:  "workflow closure workspace",
			field: "workflow_closure_workspace",
			originalPath: func(e completionAuditEvidence) string {
				return filepath.Join(filepath.Dir(e.SummaryPath), "workflow-closure-workspace")
			},
			symlinkedPath: func(linkParent, evidenceDir string) string {
				return filepath.Join(linkParent, filepath.Base(evidenceDir), "workflow-closure-workspace")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			evidenceDir := filepath.Dir(evidence.SummaryPath)
			linkParent := filepath.Join(filepath.Dir(evidenceDir), "evidence-parent-link")
			if err := os.Symlink(filepath.Dir(evidenceDir), linkParent); err != nil {
				t.Fatalf("symlink evidence parent: %v", err)
			}
			originalPath := tc.originalPath(evidence)
			symlinkedPath := tc.symlinkedPath(linkParent, evidenceDir)
			replaceTextFile(t, evidence.SummaryPath, tc.field+"="+originalPath, tc.field+"="+symlinkedPath)
			updateCompletionAuditAttestationSummaryDigest(t, evidence)

			logDir := filepath.Join(t.TempDir(), "logs")
			output, err := runCompletionAuditScriptWithLogDir(t, evidence, logDir)
			if err == nil {
				t.Fatalf("expected stage workspace symlink component to fail, got output:\n%s", output)
			}
			if !strings.Contains(string(output), tc.field+" path component must not be a symlink") {
				t.Fatalf("expected symlink component refusal for %s, got:\n%s", tc.field, output)
			}
			result := readTextFileForTest(t, filepath.Join(logDir, "completion-audit.txt"))
			if !strings.Contains(result, "status=failed") {
				t.Fatalf("expected failed completion audit result for %s, got:\n%s", tc.field, result)
			}
			for _, name := range []string{"completion-evidence-manifest.txt", "completion-evidence-manifest.sha256"} {
				if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
					t.Fatalf("stage workspace symlink component failure must not leave %s, stat=%v", name, err)
				}
			}
		})
	}
}

func TestCompletionAuditScriptRejectsStageWorkspacesOutsideProductionWorkspace(t *testing.T) {
	cases := []struct {
		name      string
		field     string
		dirName   string
		afterMove func(t *testing.T, movedWorkspace string)
	}{
		{
			name:    "IDE smoke workspace",
			field:   "ide_smoke_workspace",
			dirName: "ide-smoke-workspace",
		},
		{
			name:    "release managed workspace",
			field:   "release_managed_workspace",
			dirName: "release-managed-workspace",
			afterMove: func(t *testing.T, movedWorkspace string) {
				t.Helper()
				linkPath := filepath.Join(movedWorkspace, "toolchain", "rules", "nuclei-templates")
				if err := os.Remove(linkPath); err != nil {
					t.Fatalf("remove moved release managed rule link: %v", err)
				}
				targetPath := filepath.Join(movedWorkspace, "toolchain", "managed-bundles", "nuclei-templates", "payload")
				if err := os.Symlink(targetPath, linkPath); err != nil {
					t.Fatalf("relink moved release managed rule payload: %v", err)
				}
			},
		},
		{
			name:    "managed payloads workspace",
			field:   "managed_payloads_workspace",
			dirName: "managed-payloads-workspace",
		},
		{
			name:    "workflow closure workspace",
			field:   "workflow_closure_workspace",
			dirName: "workflow-closure-workspace",
			afterMove: func(t *testing.T, movedWorkspace string) {
				t.Helper()
				originalWorkspace := filepath.Join(filepath.Dir(movedWorkspace), "operator evidence", "workflow-closure-workspace")
				for _, path := range []string{
					filepath.Join(movedWorkspace, "reports", "report-projection-selection.json"),
					filepath.Join(movedWorkspace, "reports", "report-projection-dispatches", "report-projection-dispatch-workflow-closure-run.json"),
					filepath.Join(movedWorkspace, "reports", "report-projection-dispatch-executions", "report-projection-dispatch-execution-workflow-closure-run.json"),
				} {
					writeTextFile(t, path, strings.ReplaceAll(readTextFileForTest(t, path), originalWorkspace, movedWorkspace))
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			evidenceDir := filepath.Dir(evidence.SummaryPath)
			originalWorkspace := filepath.Join(evidenceDir, tc.dirName)
			movedWorkspace := filepath.Join(filepath.Dir(evidenceDir), "outside-"+tc.dirName)
			if err := os.Rename(originalWorkspace, movedWorkspace); err != nil {
				t.Fatalf("move %s outside production workspace: %v", tc.field, err)
			}
			if tc.afterMove != nil {
				tc.afterMove(t, movedWorkspace)
			}
			replaceTextFile(t, evidence.SummaryPath, tc.field+"="+originalWorkspace, tc.field+"="+movedWorkspace)
			updateCompletionAuditAttestationSummaryDigest(t, evidence)

			logDir := filepath.Join(t.TempDir(), "logs")
			output, err := runCompletionAuditScriptWithLogDir(t, evidence, logDir)
			if err == nil {
				t.Fatalf("expected outside stage workspace to fail for %s, got output:\n%s", tc.field, output)
			}
			if !strings.Contains(string(output), tc.field+" must be under production workspace") {
				t.Fatalf("expected outside stage workspace refusal for %s, got:\n%s", tc.field, output)
			}
			for _, name := range []string{"completion-evidence-manifest.txt", "completion-evidence-manifest.sha256"} {
				if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
					t.Fatalf("outside stage workspace failure must not leave %s, stat=%v", name, err)
				}
			}
		})
	}
}

func TestCompletionAuditScriptRejectsStageWorkspacesWithUnexpectedProductionChildName(t *testing.T) {
	cases := []struct {
		name      string
		field     string
		dirName   string
		movedName string
		afterMove func(t *testing.T, originalWorkspace, movedWorkspace string)
	}{
		{
			name:      "IDE smoke workspace",
			field:     "ide_smoke_workspace",
			dirName:   "ide-smoke-workspace",
			movedName: "other-ide-smoke-workspace",
		},
		{
			name:      "release managed workspace",
			field:     "release_managed_workspace",
			dirName:   "release-managed-workspace",
			movedName: "other-release-managed-workspace",
			afterMove: func(t *testing.T, _ string, movedWorkspace string) {
				t.Helper()
				linkPath := filepath.Join(movedWorkspace, "toolchain", "rules", "nuclei-templates")
				if err := os.Remove(linkPath); err != nil {
					t.Fatalf("remove moved release managed rule link: %v", err)
				}
				targetPath := filepath.Join(movedWorkspace, "toolchain", "managed-bundles", "nuclei-templates", "payload")
				if err := os.Symlink(targetPath, linkPath); err != nil {
					t.Fatalf("relink moved release managed rule payload: %v", err)
				}
			},
		},
		{
			name:      "managed payloads workspace",
			field:     "managed_payloads_workspace",
			dirName:   "managed-payloads-workspace",
			movedName: "other-managed-payloads-workspace",
		},
		{
			name:      "workflow closure workspace",
			field:     "workflow_closure_workspace",
			dirName:   "workflow-closure-workspace",
			movedName: "other-workflow-closure-workspace",
			afterMove: func(t *testing.T, originalWorkspace, movedWorkspace string) {
				t.Helper()
				for _, path := range []string{
					filepath.Join(movedWorkspace, "reports", "report-projection-selection.json"),
					filepath.Join(movedWorkspace, "reports", "report-projection-dispatches", "report-projection-dispatch-workflow-closure-run.json"),
					filepath.Join(movedWorkspace, "reports", "report-projection-dispatch-executions", "report-projection-dispatch-execution-workflow-closure-run.json"),
				} {
					writeTextFile(t, path, strings.ReplaceAll(readTextFileForTest(t, path), originalWorkspace, movedWorkspace))
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			evidenceDir := filepath.Dir(evidence.SummaryPath)
			originalWorkspace := filepath.Join(evidenceDir, tc.dirName)
			movedWorkspace := filepath.Join(evidenceDir, tc.movedName)
			if err := os.Rename(originalWorkspace, movedWorkspace); err != nil {
				t.Fatalf("move %s to unexpected production child name: %v", tc.field, err)
			}
			if tc.afterMove != nil {
				tc.afterMove(t, originalWorkspace, movedWorkspace)
			}
			replaceTextFile(t, evidence.SummaryPath, tc.field+"="+originalWorkspace, tc.field+"="+movedWorkspace)
			updateCompletionAuditAttestationSummaryDigest(t, evidence)

			logDir := filepath.Join(t.TempDir(), "logs")
			output, err := runCompletionAuditScriptWithLogDir(t, evidence, logDir)
			if err == nil {
				t.Fatalf("expected unexpected stage workspace child to fail for %s, got output:\n%s", tc.field, output)
			}
			if !strings.Contains(string(output), tc.field+" must be under production workspace") {
				t.Fatalf("expected unexpected stage workspace child refusal for %s, got:\n%s", tc.field, output)
			}
			for _, name := range []string{"completion-evidence-manifest.txt", "completion-evidence-manifest.sha256"} {
				if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
					t.Fatalf("unexpected stage workspace child failure must not leave %s, stat=%v", name, err)
				}
			}
		})
	}
}

func TestCompletionAuditScriptRejectsChangedReferencedInventoryDigest(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	inventoryPath := filepath.Join(filepath.Dir(evidence.SummaryPath), "inventory.env")
	replaceTextFile(t, inventoryPath, "binary_source=operator-local", "binary_source=operator-local-replaced")

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected changed referenced inventory digest to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "inventory_sha256 mismatch") {
		t.Fatalf("expected inventory digest mismatch, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsChangedReferencedPolicyDigest(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	policyPath := filepath.Join(filepath.Dir(evidence.SummaryPath), "policy.env")
	replaceTextFile(t, policyPath, "accepted_by=operator", "accepted_by=operator-replaced")

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected changed referenced policy digest to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "policy_sha256 mismatch") {
		t.Fatalf("expected policy digest mismatch, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsInvalidReferencedPolicyAcceptedAtTimestamp(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	oldPolicyDigest := fileSHA256ForTest(t, evidence.PolicyPath)
	writeTextFile(t, evidence.PolicyPath, strings.ReplaceAll(
		readTextFileForTest(t, evidence.PolicyPath),
		"accepted_at=2026-06-08T00:00:00Z",
		"accepted_at=not-rfc3339",
	))
	newPolicyDigest := fileSHA256ForTest(t, evidence.PolicyPath)
	writeTextFile(t, evidence.SummaryPath, strings.ReplaceAll(
		readTextFileForTest(t, evidence.SummaryPath),
		"policy_sha256="+oldPolicyDigest,
		"policy_sha256="+newPolicyDigest,
	))
	updateCompletionAuditAttestationSummaryDigest(t, evidence)

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected invalid referenced policy accepted_at to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "accepted_at must be RFC3339") {
		t.Fatalf("expected invalid policy accepted_at refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsPolicyAcceptedAfterProductionStarted(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	replaceTextFile(t, evidence.PolicyPath,
		"accepted_at=2026-06-08T00:00:00Z",
		"accepted_at=2026-06-08T00:00:30Z")
	updateCompletionAuditSummaryDigestField(t, evidence, "policy_sha256", evidence.PolicyPath)

	logDir := filepath.Join(t.TempDir(), "logs")
	output, err := runCompletionAuditScriptWithLogDir(t, evidence, logDir)
	if err == nil {
		t.Fatalf("expected policy accepted_at after production started_at to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "accepted_at must not be after started_at") {
		t.Fatalf("expected policy accepted_at production window refusal, got:\n%s", output)
	}
	for _, name := range []string{"completion-evidence-manifest.txt", "completion-evidence-manifest.sha256"} {
		if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
			t.Fatalf("policy accepted_at failure must not leave %s, stat=%v", name, err)
		}
	}
}

func TestCompletionAuditScriptRejectsInvalidProductionSummaryTimestamps(t *testing.T) {
	cases := []struct {
		name          string
		field         string
		originalValue string
	}{
		{
			name:          "started_at",
			field:         "started_at",
			originalValue: "2026-06-08T00:00:00Z",
		},
		{
			name:          "completed_at",
			field:         "completed_at",
			originalValue: "2026-06-08T00:01:00Z",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			replaceTextFile(t, evidence.SummaryPath, tc.field+"="+tc.originalValue, tc.field+"=not-rfc3339")
			updateCompletionAuditAttestationSummaryDigest(t, evidence)

			output, err := runCompletionAuditScript(t, evidence)
			if err == nil {
				t.Fatalf("expected invalid production summary %s timestamp to fail, got output:\n%s", tc.field, output)
			}
			if !strings.Contains(string(output), tc.field+" must be RFC3339") {
				t.Fatalf("expected invalid production summary %s timestamp refusal, got:\n%s", tc.field, output)
			}
		})
	}
}

func TestCompletionAuditScriptRejectsProductionSummaryCompletedBeforeStarted(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	replaceTextFile(t, evidence.SummaryPath, "started_at=2026-06-08T00:00:00Z", "started_at=2026-06-08T00:02:00Z")
	updateCompletionAuditAttestationSummaryDigest(t, evidence)

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected production summary completed_at before started_at to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "completed_at must not be before started_at") {
		t.Fatalf("expected production summary timestamp order refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsInvalidReviewedAtTimestamp(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	writeTextFile(t, evidence.AttestationPath, strings.ReplaceAll(
		readTextFileForTest(t, evidence.AttestationPath),
		"reviewed_at=2026-06-08T00:02:00Z",
		"reviewed_at=not-rfc3339",
	))

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected invalid reviewed_at timestamp to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "reviewed_at must be RFC3339") {
		t.Fatalf("expected invalid reviewed_at refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsAttestationReviewedBeforeProductionCompleted(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	replaceTextFile(t, evidence.AttestationPath, "reviewed_at=2026-06-08T00:02:00Z", "reviewed_at=2026-06-08T00:00:30Z")

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected attestation reviewed_at before production completed_at to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "reviewed_at must not be before completed_at") {
		t.Fatalf("expected attestation review timestamp order refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsAttestationReviewedAfterAuditStarted(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	replaceTextFile(t, evidence.AttestationPath, "reviewed_at=2026-06-08T00:02:00Z", "reviewed_at=2999-06-08T00:02:00Z")

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected attestation reviewed_at after audit started_at to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "reviewed_at must not be after audit_started_at") {
		t.Fatalf("expected future attestation review timestamp refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptUsesTrustedPathForDigestChecks(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	releaseDigest := fileSHA256ForTest(t, evidence.ReleaseLogPath)
	appendTextFile(t, evidence.ReleaseLogPath, "release-managed-smoke: ok after tamper\n")

	inventoryPath := readEvidenceFieldForTest(t, evidence.SummaryPath, "inventory")
	policyPath := readEvidenceFieldForTest(t, evidence.SummaryPath, "policy")
	toolDir := t.TempDir()
	fakeShaLog := filepath.Join(t.TempDir(), "fake-sha256sum.log")
	fakeSha := filepath.Join(toolDir, "sha256sum")
	writeTextFile(t, fakeSha, `#!/bin/sh
printf '%s\n' "$1" >> "$FAKE_SHA_LOG"
case "$1" in
	"$FAKE_SHA_SUMMARY_PATH") printf '%s  %s\n' "$FAKE_SHA_SUMMARY_DIGEST" "$1" ;;
	"$FAKE_SHA_ATTESTATION_PATH") printf '%s  %s\n' "$FAKE_SHA_ATTESTATION_DIGEST" "$1" ;;
	"$FAKE_SHA_RELEASE_LOG_PATH") printf '%s  %s\n' "$FAKE_SHA_RELEASE_LOG_DIGEST" "$1" ;;
	"$FAKE_SHA_MANAGED_LOG_PATH") printf '%s  %s\n' "$FAKE_SHA_MANAGED_LOG_DIGEST" "$1" ;;
	"$FAKE_SHA_WORKFLOW_LOG_PATH") printf '%s  %s\n' "$FAKE_SHA_WORKFLOW_LOG_DIGEST" "$1" ;;
	"$FAKE_SHA_INVENTORY_PATH") printf '%s  %s\n' "$FAKE_SHA_INVENTORY_DIGEST" "$1" ;;
	"$FAKE_SHA_POLICY_PATH") printf '%s  %s\n' "$FAKE_SHA_POLICY_DIGEST" "$1" ;;
	*) printf 'unexpected fake sha path: %s\n' "$1" >&2; exit 2 ;;
esac
`)
	if err := os.Chmod(fakeSha, 0o755); err != nil {
		t.Fatalf("chmod fake sha256sum: %v", err)
	}

	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"FAKE_SHA_LOG="+fakeShaLog,
		"FAKE_SHA_SUMMARY_PATH="+evidence.SummaryPath,
		"FAKE_SHA_SUMMARY_DIGEST="+fileSHA256ForTest(t, evidence.SummaryPath),
		"FAKE_SHA_ATTESTATION_PATH="+evidence.AttestationPath,
		"FAKE_SHA_ATTESTATION_DIGEST="+fileSHA256ForTest(t, evidence.AttestationPath),
		"FAKE_SHA_RELEASE_LOG_PATH="+evidence.ReleaseLogPath,
		"FAKE_SHA_RELEASE_LOG_DIGEST="+releaseDigest,
		"FAKE_SHA_MANAGED_LOG_PATH="+evidence.ManagedLogPath,
		"FAKE_SHA_MANAGED_LOG_DIGEST="+fileSHA256ForTest(t, evidence.ManagedLogPath),
		"FAKE_SHA_WORKFLOW_LOG_PATH="+evidence.WorkflowLogPath,
		"FAKE_SHA_WORKFLOW_LOG_DIGEST="+fileSHA256ForTest(t, evidence.WorkflowLogPath),
		"FAKE_SHA_INVENTORY_PATH="+inventoryPath,
		"FAKE_SHA_INVENTORY_DIGEST="+fileSHA256ForTest(t, inventoryPath),
		"FAKE_SHA_POLICY_PATH="+policyPath,
		"FAKE_SHA_POLICY_DIGEST="+fileSHA256ForTest(t, policyPath),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+filepath.Join(t.TempDir(), "logs"),
		"PATH="+toolDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected trusted digest check to reject tampered stage log despite fake sha256sum, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "release_managed_log_sha256 mismatch") {
		t.Fatalf("expected real release log digest mismatch, got:\n%s", output)
	}
	if _, err := os.Stat(fakeShaLog); !os.IsNotExist(err) {
		t.Fatalf("completion audit must not invoke PATH sha256sum, fake sha log stat=%v", err)
	}
}

func TestCompletionAuditScriptRejectsMismatchedTrustedPath(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	replaceTextFile(t, evidence.SummaryPath,
		"trusted_path=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"trusted_path=/tmp/fake-bin")
	updateCompletionAuditAttestationSummaryDigest(t, evidence)

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected mismatched trusted_path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "trusted_path expected /usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin got /tmp/fake-bin") {
		t.Fatalf("expected trusted_path mismatch refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsMissingTrustedPath(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	replaceTextFile(t, evidence.SummaryPath,
		"trusted_path=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\n",
		"")
	updateCompletionAuditAttestationSummaryDigest(t, evidence)

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected missing trusted_path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "missing required field trusted_path") {
		t.Fatalf("expected missing trusted_path refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsUnexpectedTrustedShell(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	replaceTextFile(t, evidence.SummaryPath,
		"trusted_shell="+trustedShellForTest(t),
		"trusted_shell=/tmp/fake-sh")
	updateCompletionAuditAttestationSummaryDigest(t, evidence)

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected unexpected trusted_shell to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "trusted_shell must be /bin/sh or /usr/bin/sh") {
		t.Fatalf("expected trusted_shell refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsMissingTrustedShell(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	replaceTextFile(t, evidence.SummaryPath,
		"trusted_shell="+trustedShellForTest(t)+"\n",
		"")
	updateCompletionAuditAttestationSummaryDigest(t, evidence)

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected missing trusted_shell to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "missing required field trusted_shell") {
		t.Fatalf("expected missing trusted_shell refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsMissingStageWorkspaceArtifacts(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{
		OmitStageWorkspaceArtifacts: true,
	})

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected missing stage workspace artifact to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "release managed workspace rule link must be a symlink") {
		t.Fatalf("expected release managed workspace artifact refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsReleaseManagedRuleDirectoryPlaceholder(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{
		ReleaseRuleArtifactAsDirectory: true,
	})

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected directory rule artifact placeholder to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "release managed workspace rule link must be a symlink") {
		t.Fatalf("expected non-symlink rule link refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsReleaseManagedRuleLinkSymlinkPathComponents(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	rulesDir := filepath.Dir(evidence.ReleaseRuleLinkPath)
	realRulesDir := filepath.Join(filepath.Dir(rulesDir), "rules-real")
	if err := os.Rename(rulesDir, realRulesDir); err != nil {
		t.Fatalf("rename release managed rules dir: %v", err)
	}
	if err := os.Symlink(realRulesDir, rulesDir); err != nil {
		t.Fatalf("symlink release managed rules dir: %v", err)
	}

	logDir := filepath.Join(t.TempDir(), "logs")
	output, err := runCompletionAuditScriptWithLogDir(t, evidence, logDir)
	if err == nil {
		t.Fatalf("expected release managed rule link symlink component to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "release managed workspace rule link path component must not be a symlink") {
		t.Fatalf("expected release managed rule link symlink component refusal, got:\n%s", output)
	}
	for _, name := range []string{"completion-evidence-manifest.txt", "completion-evidence-manifest.sha256"} {
		if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
			t.Fatalf("release managed rule link symlink component failure must not leave %s, stat=%v", name, err)
		}
	}
}

func TestCompletionAuditScriptRejectsReleaseManagedRuleTargetSymlinkPathComponents(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	releaseWorkspace := filepath.Join(filepath.Dir(evidence.SummaryPath), "release-managed-workspace")
	managedBundlesDir := filepath.Join(releaseWorkspace, "toolchain", "managed-bundles")
	realManagedBundlesDir := filepath.Join(releaseWorkspace, "toolchain", "managed-bundles-real")
	if err := os.Rename(managedBundlesDir, realManagedBundlesDir); err != nil {
		t.Fatalf("rename release managed bundle dir: %v", err)
	}
	if err := os.Symlink(realManagedBundlesDir, managedBundlesDir); err != nil {
		t.Fatalf("symlink release managed bundle dir: %v", err)
	}

	logDir := filepath.Join(t.TempDir(), "logs")
	output, err := runCompletionAuditScriptWithLogDir(t, evidence, logDir)
	if err == nil {
		t.Fatalf("expected release managed rule target symlink component to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "release managed workspace rule link expected target path component must not be a symlink") {
		t.Fatalf("expected release managed rule target symlink component refusal, got:\n%s", output)
	}
	for _, name := range []string{"completion-evidence-manifest.txt", "completion-evidence-manifest.sha256"} {
		if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
			t.Fatalf("release managed rule target symlink component failure must not leave %s, stat=%v", name, err)
		}
	}
}

func TestCompletionAuditScriptRejectsReleaseManagedRulePayloadDigestMismatch(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	writeTextFile(t, filepath.Join(evidence.ReleaseRulePayloadPath, "template.yaml"), "id: tampered-template\n")

	logDir := filepath.Join(t.TempDir(), "logs")
	output, err := runCompletionAuditScriptWithLogDir(t, evidence, logDir)
	if err == nil {
		t.Fatalf("expected release managed rule payload digest mismatch to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "release managed workspace rule payload mismatch") {
		t.Fatalf("expected release managed rule payload digest mismatch refusal, got:\n%s", output)
	}
	for _, name := range []string{"completion-evidence-manifest.txt", "completion-evidence-manifest.sha256"} {
		if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
			t.Fatalf("release managed rule payload digest failure must not leave %s, stat=%v", name, err)
		}
	}
}

func TestCompletionAuditScriptRejectsManagedPayloadArtifactDigestMismatch(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	writeTextFile(t, evidence.ManagedBinaryPath, "tampered managed payload\n")

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected managed payload digest mismatch to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "managed payload workspace binary payload mismatch") {
		t.Fatalf("expected managed payload digest mismatch refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsWorkflowSelectionWithoutCurrentStatus(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	writeTextFile(t, evidence.WorkflowSelectionPath, `{"status":"cleared"}`+"\n")

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected non-current workflow selection to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "workflow closure workspace report selection status must be current") {
		t.Fatalf("expected workflow selection current marker refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptRejectsWorkflowSelectionWithConflictingStatus(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	writeTextFile(t, evidence.WorkflowSelectionPath, `{"status":"cleared","history":[{"status":"current"}]}`+"\n")

	logDir := filepath.Join(t.TempDir(), "logs")
	output, err := runCompletionAuditScriptWithLogDir(t, evidence, logDir)
	if err == nil {
		t.Fatalf("expected conflicting workflow selection status to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "workflow closure workspace report selection status must be current") {
		t.Fatalf("expected workflow selection top-level status refusal, got:\n%s", output)
	}
	for _, name := range []string{"completion-evidence-manifest.txt", "completion-evidence-manifest.sha256"} {
		if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
			t.Fatalf("conflicting workflow selection status failure must not leave %s, stat=%v", name, err)
		}
	}
}

func TestCompletionAuditScriptRejectsTruncatedWorkflowSelectionArtifact(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	writeTextFile(t, evidence.WorkflowSelectionPath, `{"status":"current"}`+"\n")

	logDir := filepath.Join(t.TempDir(), "logs")
	output, err := runCompletionAuditScriptWithLogDir(t, evidence, logDir)
	if err == nil {
		t.Fatalf("expected truncated workflow selection artifact to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "workflow closure workspace report selection mode is missing") {
		t.Fatalf("expected workflow selection provenance refusal, got:\n%s", output)
	}
	for _, name := range []string{"completion-evidence-manifest.txt", "completion-evidence-manifest.sha256"} {
		if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
			t.Fatalf("truncated workflow selection failure must not leave %s, stat=%v", name, err)
		}
	}
}

func TestCompletionAuditScriptRejectsWorkflowSelectionProvenanceMismatch(t *testing.T) {
	cases := []struct {
		name          string
		tamper        func(t *testing.T, evidence completionAuditEvidence)
		expectedError string
	}{
		{
			name: "selection report path",
			tamper: func(t *testing.T, evidence completionAuditEvidence) {
				t.Helper()
				selection := readTextFileForTest(t, evidence.WorkflowSelectionPath)
				replaceTextFile(t, evidence.WorkflowSelectionPath,
					`"report_path": "`+readWorkflowSelectionFieldForTest(t, selection, "report_path")+`"`,
					`"report_path": "`+filepath.Join(filepath.Dir(evidence.WorkflowSelectionPath), "other-run.md")+`"`)
			},
			expectedError: "workflow closure workspace report selection report_path expected",
		},
		{
			name: "execution status",
			tamper: func(t *testing.T, evidence completionAuditEvidence) {
				t.Helper()
				selection := readTextFileForTest(t, evidence.WorkflowSelectionPath)
				executionPath := readWorkflowSelectionFieldForTest(t, selection, "execution_path")
				replaceTextFile(t, executionPath, `"status": "completed"`, `"status": "failed"`)
			},
			expectedError: "workflow closure workspace report selection execution status expected completed got failed",
		},
		{
			name: "dispatch mode",
			tamper: func(t *testing.T, evidence completionAuditEvidence) {
				t.Helper()
				selection := readTextFileForTest(t, evidence.WorkflowSelectionPath)
				dispatchPath := readWorkflowSelectionFieldForTest(t, selection, "dispatch_path")
				replaceTextFile(t, dispatchPath, `"mode": "metadata-only"`, `"mode": "execute"`)
			},
			expectedError: "workflow closure workspace report selection dispatch mode expected metadata-only got execute",
		},
		{
			name: "selection written",
			tamper: func(t *testing.T, evidence completionAuditEvidence) {
				t.Helper()
				replaceTextFile(t, evidence.WorkflowSelectionPath, `"written": true`, `"written": false`)
			},
			expectedError: "workflow closure workspace report selection written expected true got false",
		},
		{
			name: "execution report digest",
			tamper: func(t *testing.T, evidence completionAuditEvidence) {
				t.Helper()
				selection := readTextFileForTest(t, evidence.WorkflowSelectionPath)
				reportPath := readWorkflowSelectionFieldForTest(t, selection, "report_path")
				writeTextFile(t, reportPath, "# Tampered workflow closure report\n")
			},
			expectedError: "workflow closure workspace report selection execution report_sha256 mismatch",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			tc.tamper(t, evidence)

			logDir := filepath.Join(t.TempDir(), "logs")
			output, err := runCompletionAuditScriptWithLogDir(t, evidence, logDir)
			if err == nil {
				t.Fatalf("expected workflow selection provenance mismatch to fail, got output:\n%s", output)
			}
			if !strings.Contains(string(output), tc.expectedError) {
				t.Fatalf("expected workflow selection provenance refusal %q, got:\n%s", tc.expectedError, output)
			}
			for _, name := range []string{"completion-evidence-manifest.txt", "completion-evidence-manifest.sha256"} {
				if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
					t.Fatalf("workflow selection provenance failure must not leave %s, stat=%v", name, err)
				}
			}
		})
	}
}

func TestCompletionAuditScriptRejectsWorkflowSelectionSelectedAtOutsideProductionWindow(t *testing.T) {
	cases := []struct {
		name          string
		selectedAt    string
		expectedError string
	}{
		{
			name:          "before production started",
			selectedAt:    "2026-06-07T23:59:59Z",
			expectedError: "workflow closure workspace report selection selected_at must not be before started_at",
		},
		{
			name:          "after production completed",
			selectedAt:    "2999-06-08T00:01:00Z",
			expectedError: "workflow closure workspace report selection selected_at must not be after completed_at",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			replaceTextFile(t, evidence.WorkflowSelectionPath,
				`"selected_at": "2026-06-08T00:01:00Z"`,
				`"selected_at": "`+tc.selectedAt+`"`)

			logDir := filepath.Join(t.TempDir(), "logs")
			output, err := runCompletionAuditScriptWithLogDir(t, evidence, logDir)
			if err == nil {
				t.Fatalf("expected workflow selection selected_at outside production window to fail, got output:\n%s", output)
			}
			if !strings.Contains(string(output), tc.expectedError) {
				t.Fatalf("expected workflow selection selected_at refusal %q, got:\n%s", tc.expectedError, output)
			}
			for _, name := range []string{"completion-evidence-manifest.txt", "completion-evidence-manifest.sha256"} {
				if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
					t.Fatalf("workflow selection selected_at failure must not leave %s, stat=%v", name, err)
				}
			}
		})
	}
}

func TestCompletionAuditScriptRejectsWorkflowReportArtifactTimestampsAfterSelection(t *testing.T) {
	cases := []struct {
		name          string
		tamper        func(t *testing.T, evidence completionAuditEvidence)
		expectedError string
	}{
		{
			name: "dispatch created after selection",
			tamper: func(t *testing.T, evidence completionAuditEvidence) {
				t.Helper()
				selection := readTextFileForTest(t, evidence.WorkflowSelectionPath)
				dispatchPath := readWorkflowSelectionFieldForTest(t, selection, "dispatch_path")
				replaceTextFile(t, dispatchPath,
					`"created_at": "2026-06-08T00:00:30Z"`,
					`"created_at": "2999-06-08T00:00:30Z"`)
			},
			expectedError: "workflow closure workspace report selection dispatch created_at must not be after selected_at",
		},
		{
			name: "execution executed after selection",
			tamper: func(t *testing.T, evidence completionAuditEvidence) {
				t.Helper()
				selection := readTextFileForTest(t, evidence.WorkflowSelectionPath)
				executionPath := readWorkflowSelectionFieldForTest(t, selection, "execution_path")
				replaceTextFile(t, executionPath,
					`"executed_at": "2026-06-08T00:00:40Z"`,
					`"executed_at": "2999-06-08T00:00:40Z"`)
			},
			expectedError: "workflow closure workspace report selection execution executed_at must not be after selected_at",
		},
		{
			name: "annotation reviewed after selection",
			tamper: func(t *testing.T, evidence completionAuditEvidence) {
				t.Helper()
				selection := readTextFileForTest(t, evidence.WorkflowSelectionPath)
				executionPath := readWorkflowSelectionFieldForTest(t, selection, "execution_path")
				replaceTextFile(t, executionPath,
					`"reviewed_at": "2026-06-08T00:00:50Z"`,
					`"reviewed_at": "2999-06-08T00:00:50Z"`)
			},
			expectedError: "workflow closure workspace report selection execution annotation reviewed_at must not be after selected_at",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
			tc.tamper(t, evidence)

			logDir := filepath.Join(t.TempDir(), "logs")
			output, err := runCompletionAuditScriptWithLogDir(t, evidence, logDir)
			if err == nil {
				t.Fatalf("expected workflow report artifact timestamp after selection to fail, got output:\n%s", output)
			}
			if !strings.Contains(string(output), tc.expectedError) {
				t.Fatalf("expected workflow report artifact timestamp refusal %q, got:\n%s", tc.expectedError, output)
			}
			for _, name := range []string{"completion-evidence-manifest.txt", "completion-evidence-manifest.sha256"} {
				if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
					t.Fatalf("workflow report artifact timestamp failure must not leave %s, stat=%v", name, err)
				}
			}
		})
	}
}

func TestCompletionAuditScriptRejectsWrongStageScriptPathWithMatchingDigest(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	fakeScript := filepath.Join(t.TempDir(), "fake-smoke-release-managed.sh")
	writeTextFile(t, fakeScript, "#!/bin/sh\nprintf 'release-managed-smoke: ok\\n'\n")
	realScript := repoFilePathForTest(t, "scripts", "smoke_release_managed.sh")
	replaceTextFile(t, evidence.SummaryPath, "release_managed_script="+realScript, "release_managed_script="+fakeScript)
	replaceTextFile(t, evidence.SummaryPath, "release_managed_script_sha256="+fileSHA256ForTest(t, realScript), "release_managed_script_sha256="+fileSHA256ForTest(t, fakeScript))
	updateCompletionAuditAttestationSummaryDigest(t, evidence)

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected wrong stage script path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "release_managed_script must reference expected script") {
		t.Fatalf("expected wrong stage script path refusal, got:\n%s", output)
	}
}

func TestCompletionAuditScriptDirectExecUsesTrustedShellBeforePrelude(t *testing.T) {
	toolDir := t.TempDir()
	fakeShLog := filepath.Join(t.TempDir(), "fake-sh.log")
	fakeSh := filepath.Join(toolDir, "sh")
	writeTextFile(t, fakeSh, "#!/bin/sh\nprintf 'fake sh invoked\\n' >> \"$FAKE_SH_LOG\"\nexit 42\n")
	if err := os.Chmod(fakeSh, 0o755); err != nil {
		t.Fatalf("chmod fake sh: %v", err)
	}
	fakeDirnameLog := filepath.Join(t.TempDir(), "fake-dirname.log")
	fakeDirname := filepath.Join(toolDir, "dirname")
	writeTextFile(t, fakeDirname, "#!/bin/sh\nprintf 'fake dirname invoked\\n' >> \"$FAKE_DIRNAME_LOG\"\nexit 42\n")
	if err := os.Chmod(fakeDirname, 0o755); err != nil {
		t.Fatalf("chmod fake dirname: %v", err)
	}

	cmd := exec.Command("./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"FAKE_SH_LOG="+fakeShLog,
		"FAKE_DIRNAME_LOG="+fakeDirnameLog,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+filepath.Join(t.TempDir(), "logs"),
		"PATH="+toolDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected completion audit direct exec to fail for missing summary, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY is required") {
		t.Fatalf("expected missing summary failure from real script, got:\n%s", output)
	}
	if _, err := os.Stat(fakeShLog); !os.IsNotExist(err) {
		t.Fatalf("completion audit direct exec must not invoke PATH sh, fake sh log stat=%v", err)
	}
	if _, err := os.Stat(fakeDirnameLog); !os.IsNotExist(err) {
		t.Fatalf("completion audit prelude must not invoke PATH dirname, fake dirname log stat=%v", err)
	}
}

func TestCompletionAuditScriptRejectsSymlinkReferencedPayloadPath(t *testing.T) {
	evidence := writeCompletionAuditEvidence(t, completionAuditEvidenceOptions{})
	evidenceDir := filepath.Dir(evidence.SummaryPath)
	binaryTarget := filepath.Join(evidenceDir, "httpx")
	binaryLink := filepath.Join(evidenceDir, "httpx-link")
	symlinkFileForTest(t, binaryTarget, binaryLink)
	inventoryPath := filepath.Join(evidenceDir, "inventory.env")
	replaceTextFile(t, inventoryPath, "binary_payload="+binaryTarget, "binary_payload="+binaryLink)

	output, err := runCompletionAuditScript(t, evidence)
	if err == nil {
		t.Fatalf("expected symlink referenced payload path to fail, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "referenced binary payload file must not be a symlink") {
		t.Fatalf("expected symlink referenced payload refusal, got:\n%s", output)
	}
}

type completionAuditEvidence struct {
	SummaryPath            string
	AttestationPath        string
	InventoryPath          string
	PolicyPath             string
	IdeLogPath             string
	IdeDirectUseLogPath    string
	IdeAIProviderLogPath   string
	ReleaseLogPath         string
	ManagedLogPath         string
	WorkflowLogPath        string
	ReleaseRuleLinkPath    string
	ReleaseRulePayloadPath string
	ManagedBinaryPath      string
	WorkflowSelectionPath  string
}

type completionAuditEvidenceOptions struct {
	ReleaseLogDigestOverride         string
	OmitNoLiveTargets                bool
	AttestationSummaryDigestOverride string
	OmitStageWorkspaceArtifacts      bool
	ReleaseRuleArtifactAsDirectory   bool
}

func writeCompletionAuditEvidence(t *testing.T, opts completionAuditEvidenceOptions) completionAuditEvidence {
	t.Helper()
	evidenceDir := filepath.Join(t.TempDir(), "operator evidence")
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		t.Fatalf("create evidence dir: %v", err)
	}
	binaryPayload := filepath.Join(evidenceDir, "httpx")
	writeTextFile(t, binaryPayload, "#!/usr/bin/env sh\nexit 0\n")
	rulePayload := filepath.Join(evidenceDir, "nuclei-templates")
	if err := os.MkdirAll(rulePayload, 0o755); err != nil {
		t.Fatalf("create rule payload: %v", err)
	}
	writeTextFile(t, filepath.Join(rulePayload, "template.yaml"), "id: test-template\n")
	inventoryPath := filepath.Join(evidenceDir, "inventory.env")
	writeTextFile(t, inventoryPath, strings.Join([]string{
		"schema=ouroboros.production_acceptance.inventory.v1",
		"inventory_status=operator_owned",
		"binary_name=httpx",
		"binary_pack=web-api-pack",
		"binary_payload=" + binaryPayload,
		"binary_expected_sha256=" + fileSHA256ForTest(t, binaryPayload),
		"binary_version=operator-test",
		"binary_source=operator-local",
		"rule_bundle_name=nuclei-templates",
		"rule_bundle_pack=web-api-pack",
		"rule_bundle_payload=" + rulePayload,
		"rule_bundle_expected_sha256=" + digestManagedPayloadsSmokeDirectory(t, rulePayload),
		"rule_bundle_version=operator-test",
		"rule_bundle_source=operator-local",
		"",
	}, "\n"))
	policyPath := filepath.Join(evidenceDir, "policy.env")
	writeTextFile(t, policyPath, strings.Join([]string{
		"schema=ouroboros.production_acceptance.policy.v1",
		"policy_status=approved",
		"deployment_environment=local-acceptance",
		"accepted_by=operator",
		"accepted_at=2026-06-08T00:00:00Z",
		"authorization_scope=authorized-local-fixture",
		"managed_payload_acceptance_required=true",
		"workflow_closure_required=true",
		"no_live_targets=true",
		"payload_execution_allowed=false",
		"network_download_allowed=false",
		"package_manager_allowed=false",
		"provider_calls_allowed=false",
		"worker_contact_allowed=false",
		"",
	}, "\n"))

	releaseLogPath := filepath.Join(evidenceDir, "release-managed.txt")
	managedLogPath := filepath.Join(evidenceDir, "managed-payloads.txt")
	workflowLogPath := filepath.Join(evidenceDir, "workflow-closure.txt")
	ideLogPath := filepath.Join(evidenceDir, "ide-smoke.txt")
	ideChildLogDir := filepath.Join(evidenceDir, "ide-smoke-logs")
	if err := os.MkdirAll(ideChildLogDir, 0o755); err != nil {
		t.Fatalf("create IDE child log dir: %v", err)
	}
	ideDirectUseLogPath := filepath.Join(ideChildLogDir, "direct-use.txt")
	ideAIProviderLogPath := filepath.Join(ideChildLogDir, "ai-provider.txt")
	writeTextFile(t, ideLogPath, "ide-smoke: ok\n")
	writeTextFile(t, ideDirectUseLogPath, "chat: available\nchat: composer\ndirect-use-smoke: ok\n")
	writeTextFile(t, ideAIProviderLogPath, "ai-provider-smoke: ok\nchat-prompt: preview=created stored=false\n")
	writeTextFile(t, releaseLogPath, "release-managed-smoke: ok\n")
	writeTextFile(t, managedLogPath, "managed-payloads-smoke: ok\n")
	writeTextFile(t, workflowLogPath, "workflow-closure-smoke: ok\n")
	ideWorkspace := filepath.Join(evidenceDir, "ide-smoke-workspace")
	releaseWorkspace := filepath.Join(evidenceDir, "release-managed-workspace")
	managedWorkspace := filepath.Join(evidenceDir, "managed-payloads-workspace")
	workflowWorkspace := filepath.Join(evidenceDir, "workflow-closure-workspace")
	for _, dir := range []string{ideWorkspace, releaseWorkspace, managedWorkspace, workflowWorkspace} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create stage workspace %s: %v", dir, err)
		}
	}
	if !opts.OmitStageWorkspaceArtifacts {
		if err := os.MkdirAll(filepath.Join(releaseWorkspace, "toolchain", "rules"), 0o755); err != nil {
			t.Fatalf("create release managed rules dir: %v", err)
		}
		releaseRulePayload := filepath.Join(releaseWorkspace, "toolchain", "managed-bundles", "nuclei-templates", "payload")
		if err := os.MkdirAll(releaseRulePayload, 0o755); err != nil {
			t.Fatalf("create release managed rule payload fixture: %v", err)
		}
		writeTextFile(t, filepath.Join(releaseRulePayload, "template.yaml"), readTextFileForTest(t, filepath.Join(rulePayload, "template.yaml")))
		releaseRuleLink := filepath.Join(releaseWorkspace, "toolchain", "rules", "nuclei-templates")
		if opts.ReleaseRuleArtifactAsDirectory {
			if err := os.MkdirAll(releaseRuleLink, 0o755); err != nil {
				t.Fatalf("create release managed rule directory placeholder: %v", err)
			}
		} else if err := os.Symlink(releaseRulePayload, releaseRuleLink); err != nil {
			t.Fatalf("create release managed rule link fixture: %v", err)
		}
		managedBinary := filepath.Join(managedWorkspace, "toolchain", "managed-bundles", "httpx", "payload", "httpx")
		if err := os.MkdirAll(filepath.Dir(managedBinary), 0o755); err != nil {
			t.Fatalf("create managed payload artifact dir: %v", err)
		}
		writeTextFile(t, managedBinary, readTextFileForTest(t, binaryPayload))
		workflowSelection := filepath.Join(workflowWorkspace, "reports", "report-projection-selection.json")
		if err := os.MkdirAll(filepath.Dir(workflowSelection), 0o755); err != nil {
			t.Fatalf("create workflow report dir: %v", err)
		}
		runID := "workflow-closure-run"
		reportPath := filepath.Join(workflowWorkspace, "reports", runID+".md")
		dispatchPath := filepath.Join(workflowWorkspace, "reports", "report-projection-dispatches", "report-projection-dispatch-workflow-closure-run.json")
		executionPath := filepath.Join(workflowWorkspace, "reports", "report-projection-dispatch-executions", "report-projection-dispatch-execution-workflow-closure-run.json")
		if err := os.MkdirAll(filepath.Dir(dispatchPath), 0o755); err != nil {
			t.Fatalf("create workflow report dispatch dir: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(executionPath), 0o755); err != nil {
			t.Fatalf("create workflow report execution dir: %v", err)
		}
		writeTextFile(t, reportPath, "# Workflow closure report\n")
		reportSHA256 := fileSHA256ForTest(t, reportPath)
		writeTextFile(t, dispatchPath, strings.Join([]string{
			"{",
			`  "version": 1,`,
			`  "status": "dry-run",`,
			`  "mode": "metadata-only",`,
			`  "created_at": "2026-06-08T00:00:30Z",`,
			`  "actor": "operator",`,
			`  "run_id": "` + runID + `",`,
			`  "report_path": "` + reportPath + `",`,
			`  "event_count": 3,`,
			`  "replay_digest": "test-replay-digest"`,
			"}",
			"",
		}, "\n"))
		writeTextFile(t, executionPath, strings.Join([]string{
			"{",
			`  "version": 1,`,
			`  "status": "completed",`,
			`  "mode": "execute",`,
			`  "executed_at": "2026-06-08T00:00:40Z",`,
			`  "actor": "operator",`,
			`  "dispatch_path": "` + dispatchPath + `",`,
			`  "run_id": "` + runID + `",`,
			`  "report_path": "` + reportPath + `",`,
			`  "report_sha256": "` + reportSHA256 + `",`,
			`  "event_count": 3,`,
			`  "written": true,`,
			`  "annotation": {`,
			`    "decision": "accepted",`,
			`    "reviewer": "operator",`,
			`    "reviewed_at": "2026-06-08T00:00:50Z"`,
			`  }`,
			"}",
			"",
		}, "\n"))
		writeTextFile(t, workflowSelection, strings.Join([]string{
			"{",
			`  "path": "` + workflowSelection + `",`,
			`  "status": "current",`,
			`  "mode": "metadata-only",`,
			`  "execution_path": "` + executionPath + `",`,
			`  "dispatch_path": "` + dispatchPath + `",`,
			`  "run_id": "` + runID + `",`,
			`  "report_path": "` + reportPath + `",`,
			`  "event_count": 3,`,
			`  "written": true,`,
			`  "selected_by": "operator",`,
			`  "selected_at": "2026-06-08T00:01:00Z",`,
			`  "reason": "completion audit fixture"`,
			"}",
			"",
		}, "\n"))
	}

	releaseDigest := fileSHA256ForTest(t, releaseLogPath)
	if opts.ReleaseLogDigestOverride != "" {
		releaseDigest = opts.ReleaseLogDigestOverride
	}
	summaryLines := []string{
		"schema=ouroboros.production_acceptance.audit.v1",
		"audit_id=production-acceptance-test",
		"started_at=2026-06-08T00:00:00Z",
		"completed_at=2026-06-08T00:01:00Z",
		"status=accepted",
		"mode=operator-owned",
		"inventory=" + inventoryPath,
		"policy=" + policyPath,
		"inventory_sha256=" + fileSHA256ForTest(t, inventoryPath),
		"policy_sha256=" + fileSHA256ForTest(t, policyPath),
		"workspace=" + evidenceDir,
		"ide_smoke_workspace=" + ideWorkspace,
		"release_managed_workspace=" + releaseWorkspace,
		"managed_payloads_workspace=" + managedWorkspace,
		"workflow_closure_workspace=" + workflowWorkspace,
		"trusted_shell=" + trustedShellForTest(t),
		"trusted_path=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"production_acceptance_script=" + repoFilePathForTest(t, "scripts", "smoke_production_acceptance.sh"),
		"production_acceptance_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_production_acceptance.sh")),
		"ide_smoke_script=" + repoFilePathForTest(t, "scripts", "smoke_ide.sh"),
		"ide_smoke_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_ide.sh")),
		"ide_direct_use_script=" + repoFilePathForTest(t, "scripts", "smoke_direct_use.sh"),
		"ide_direct_use_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_direct_use.sh")),
		"ide_ai_provider_script=" + repoFilePathForTest(t, "scripts", "smoke_ai_provider.sh"),
		"ide_ai_provider_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_ai_provider.sh")),
		"release_managed_script=" + repoFilePathForTest(t, "scripts", "smoke_release_managed.sh"),
		"release_managed_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_release_managed.sh")),
		"managed_payloads_script=" + repoFilePathForTest(t, "scripts", "smoke_managed_payloads.sh"),
		"managed_payloads_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_managed_payloads.sh")),
		"workflow_closure_script=" + repoFilePathForTest(t, "scripts", "smoke_workflow_closure.sh"),
		"workflow_closure_script_sha256=" + fileSHA256ForTest(t, repoFilePathForTest(t, "scripts", "smoke_workflow_closure.sh")),
		"binary_name=httpx",
		"binary_expected_sha256=" + fileSHA256ForTest(t, binaryPayload),
		"rule_bundle_name=nuclei-templates",
		"rule_bundle_expected_sha256=" + digestManagedPayloadsSmokeDirectory(t, rulePayload),
		"ide_smoke_acceptance=passed",
		"release_managed_acceptance=passed",
		"managed_payload_acceptance=passed",
		"workflow_closure_acceptance=passed",
		"ide_smoke_log=" + ideLogPath,
		"ide_direct_use_log=" + ideDirectUseLogPath,
		"ide_ai_provider_log=" + ideAIProviderLogPath,
		"release_managed_log=" + releaseLogPath,
		"managed_payloads_log=" + managedLogPath,
		"workflow_closure_log=" + workflowLogPath,
		"ide_smoke_log_sha256=" + fileSHA256ForTest(t, ideLogPath),
		"ide_direct_use_log_sha256=" + fileSHA256ForTest(t, ideDirectUseLogPath),
		"ide_ai_provider_log_sha256=" + fileSHA256ForTest(t, ideAIProviderLogPath),
		"release_managed_log_sha256=" + releaseDigest,
		"managed_payloads_log_sha256=" + fileSHA256ForTest(t, managedLogPath),
		"workflow_closure_log_sha256=" + fileSHA256ForTest(t, workflowLogPath),
	}
	if !opts.OmitNoLiveTargets {
		summaryLines = append(summaryLines, "no_live_targets=true")
	}
	summaryLines = append(summaryLines,
		"payload_execution_allowed=false",
		"network_download_allowed=false",
		"package_manager_allowed=false",
		"provider_calls_allowed=false",
		"worker_contact_allowed=false",
		"",
	)
	summaryPath := filepath.Join(evidenceDir, "summary.txt")
	writeTextFile(t, summaryPath, strings.Join(summaryLines, "\n"))

	summaryDigest := fileSHA256ForTest(t, summaryPath)
	if opts.AttestationSummaryDigestOverride != "" {
		summaryDigest = opts.AttestationSummaryDigestOverride
	}
	attestationPath := filepath.Join(evidenceDir, "completion-attestation.env")
	writeTextFile(t, attestationPath, strings.Join([]string{
		"schema=ouroboros.completion_audit.attestation.v1",
		"attestation_status=approved",
		"reviewed_by=operator",
		"reviewed_at=2026-06-08T00:02:00Z",
		"production_summary_sha256=" + summaryDigest,
		"production_evidence=operator_owned",
		"fixture_evidence_allowed=false",
		"scope_reviewed=true",
		"safety_boundaries_reviewed=true",
		"release_managed_reviewed=true",
		"managed_payloads_reviewed=true",
		"workflow_closure_reviewed=true",
		"documentation_reviewed=true",
		"",
	}, "\n"))
	return completionAuditEvidence{
		SummaryPath:            summaryPath,
		AttestationPath:        attestationPath,
		InventoryPath:          inventoryPath,
		PolicyPath:             policyPath,
		IdeLogPath:             ideLogPath,
		IdeDirectUseLogPath:    ideDirectUseLogPath,
		IdeAIProviderLogPath:   ideAIProviderLogPath,
		ReleaseLogPath:         releaseLogPath,
		ManagedLogPath:         managedLogPath,
		WorkflowLogPath:        workflowLogPath,
		ReleaseRuleLinkPath:    filepath.Join(releaseWorkspace, "toolchain", "rules", "nuclei-templates"),
		ReleaseRulePayloadPath: filepath.Join(releaseWorkspace, "toolchain", "managed-bundles", "nuclei-templates", "payload"),
		ManagedBinaryPath:      filepath.Join(managedWorkspace, "toolchain", "managed-bundles", "httpx", "payload", "httpx"),
		WorkflowSelectionPath:  filepath.Join(workflowWorkspace, "reports", "report-projection-selection.json"),
	}
}

func readTextFileForTest(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func finalEvidenceRunbookBashBlocksForTest(doc string) []string {
	const heading = "## Final Evidence Runbook"
	start := strings.Index(doc, heading)
	if start < 0 {
		return nil
	}
	rest := doc[start+len(heading):]
	if next := strings.Index(rest, "\n## "); next >= 0 {
		rest = rest[:next]
	}
	var blocks []string
	var current []string
	inBash := false
	for _, line := range strings.Split(rest, "\n") {
		switch strings.TrimSpace(line) {
		case "```bash":
			inBash = true
			current = nil
			continue
		case "```":
			if inBash {
				blocks = append(blocks, strings.Join(current, "\n")+"\n")
			}
			inBash = false
			current = nil
			continue
		}
		if inBash {
			current = append(current, line)
		}
	}
	return blocks
}

func bashBlocksForTest(doc string) []string {
	var blocks []string
	var current []string
	inBash := false
	for _, line := range strings.Split(doc, "\n") {
		switch strings.TrimSpace(line) {
		case "```bash":
			inBash = true
			current = nil
			continue
		case "```":
			if inBash {
				blocks = append(blocks, strings.Join(current, "\n")+"\n")
			}
			inBash = false
			current = nil
			continue
		}
		if inBash {
			current = append(current, line)
		}
	}
	return blocks
}

func writeTextFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func appendTextFile(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("append %s: %v", path, err)
	}
}

func runCompletionAuditScript(t *testing.T, evidence completionAuditEvidence) ([]byte, error) {
	t.Helper()
	return runCompletionAuditScriptWithLogDir(t, evidence, filepath.Join(t.TempDir(), "logs"))
}

func runCompletionAuditScriptWithLogDir(t *testing.T, evidence completionAuditEvidence, logDir string) ([]byte, error) {
	t.Helper()
	cmd := exec.Command("sh", "./scripts/audit_completion.sh")
	cmd.Env = append(os.Environ(),
		"OURO_COMPLETION_AUDIT_PRODUCTION_SUMMARY="+evidence.SummaryPath,
		"OURO_COMPLETION_AUDIT_ATTESTATION="+evidence.AttestationPath,
		"OURO_COMPLETION_AUDIT_LOG_DIR="+logDir,
	)
	return cmd.CombinedOutput()
}

func updateCompletionAuditAttestationSummaryDigest(t *testing.T, evidence completionAuditEvidence) {
	t.Helper()
	data, err := os.ReadFile(evidence.AttestationPath)
	if err != nil {
		t.Fatalf("read attestation: %v", err)
	}
	lines := strings.Split(string(data), "\n")
	updated := false
	for i, line := range lines {
		if strings.HasPrefix(line, "production_summary_sha256=") {
			lines[i] = "production_summary_sha256=" + fileSHA256ForTest(t, evidence.SummaryPath)
			updated = true
			break
		}
	}
	if !updated {
		t.Fatalf("attestation missing production_summary_sha256 line")
	}
	writeTextFile(t, evidence.AttestationPath, strings.Join(lines, "\n"))
}

func updateCompletionAuditSummaryDigestField(t *testing.T, evidence completionAuditEvidence, key, path string) {
	t.Helper()
	oldValue := readEvidenceFieldForTest(t, evidence.SummaryPath, key)
	replaceTextFile(t, evidence.SummaryPath, key+"="+oldValue, key+"="+fileSHA256ForTest(t, path))
	updateCompletionAuditAttestationSummaryDigest(t, evidence)
}

func readEvidenceFieldForTest(t *testing.T, path, key string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	prefix := key + "="
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	t.Fatalf("%s missing %s", path, key)
	return ""
}

func readWorkflowSelectionFieldForTest(t *testing.T, data, key string) string {
	t.Helper()
	prefix := `"` + key + `": "`
	start := strings.Index(data, prefix)
	if start < 0 {
		t.Fatalf("workflow selection missing %s in:\n%s", key, data)
	}
	rest := data[start+len(prefix):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		t.Fatalf("workflow selection field %s is unterminated in:\n%s", key, data)
	}
	return rest[:end]
}

func repoFilePathForTest(t *testing.T, elems ...string) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	parts := append([]string{wd}, elems...)
	return filepath.Join(parts...)
}

func trustedShellForTest(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		// These production acceptance / completion audit script tests validate
		// POSIX security semantics (trusted /bin/sh, /usr/local/sbin:... PATH,
		// readlink, symlink rejection) that only apply on Unix. The scripts are
		// not supported on Windows, so skip rather than falsify their assertions.
		t.Skip("POSIX production audit script tests are Linux-only")
	}
	for _, candidate := range []string{"/bin/sh", "/usr/bin/sh"} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	t.Fatalf("trusted shell fixture not found")
	return ""
}

func runSmokeDirectExecTrustedPreludeCheck(t *testing.T, scriptPath, logDirEnv string) ([]byte, string, string, string) {
	t.Helper()
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	writeTextFile(t, filepath.Join(workspace, "scope.yaml"), "version: 1\n")

	toolDir := t.TempDir()
	fakeShLog := filepath.Join(t.TempDir(), "fake-sh.log")
	fakeSh := filepath.Join(toolDir, "sh")
	writeTextFile(t, fakeSh, "#!/bin/sh\nprintf 'fake sh invoked\\n' >> \"$FAKE_SH_LOG\"\nexit 42\n")
	if err := os.Chmod(fakeSh, 0o755); err != nil {
		t.Fatalf("chmod fake sh: %v", err)
	}
	fakeDirnameLog := filepath.Join(t.TempDir(), "fake-dirname.log")
	fakeDirname := filepath.Join(toolDir, "dirname")
	writeTextFile(t, fakeDirname, "#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$FAKE_DIRNAME_LOG\"\nprintf '.\\n'\n")
	if err := os.Chmod(fakeDirname, 0o755); err != nil {
		t.Fatalf("chmod fake dirname: %v", err)
	}
	fakeGoLog := filepath.Join(t.TempDir(), "fake-go.log")
	fakeGo := filepath.Join(toolDir, "go")
	writeTextFile(t, fakeGo, "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$FAKE_GO_LOG\"\nexit 0\n")
	if err := os.Chmod(fakeGo, 0o755); err != nil {
		t.Fatalf("chmod fake go: %v", err)
	}

	cmd := exec.Command(scriptPath)
	cmd.Env = append(os.Environ(),
		"FAKE_SH_LOG="+fakeShLog,
		"FAKE_DIRNAME_LOG="+fakeDirnameLog,
		"FAKE_GO_LOG="+fakeGoLog,
		"GOPROXY=https://goproxy.cn,direct",
		"WORKSPACE="+workspace,
		logDirEnv+"="+filepath.Join(t.TempDir(), "logs"),
		"PATH="+toolDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected %s direct exec to fail for existing configured workspace, got output:\n%s", scriptPath, output)
	}
	return output, fakeShLog, fakeDirnameLog, fakeGoLog
}

func assertSmokeDirectExecTrustedPrelude(t *testing.T, scriptPath string, output []byte, fakeShLog, fakeDirnameLog, fakeGoLog string) {
	t.Helper()
	if !strings.Contains(string(output), "refusing existing non-empty workspace") {
		t.Fatalf("expected existing-workspace refusal from %s, got:\n%s", scriptPath, output)
	}
	if _, err := os.Stat(fakeShLog); !os.IsNotExist(err) {
		t.Fatalf("%s direct exec must not invoke PATH sh, fake sh log stat=%v", scriptPath, err)
	}
	if _, err := os.Stat(fakeDirnameLog); !os.IsNotExist(err) {
		t.Fatalf("%s prelude must not invoke PATH dirname, fake dirname log stat=%v", scriptPath, err)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("%s trusted preflight must fail before invoking PATH go, fake go log stat=%v", scriptPath, err)
	}
}

func replaceTextFile(t *testing.T, path, old, new string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(data)
	if !strings.Contains(text, old) {
		t.Fatalf("%s does not contain %q", path, old)
	}
	writeTextFile(t, path, strings.Replace(text, old, new, 1))
}

func symlinkFileForTest(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink %s -> %s: %v", link, target, err)
	}
}

func pathWithSymlinkAncestorForTest(t *testing.T, root, target string) string {
	t.Helper()
	rel, err := filepath.Rel(root, target)
	if err != nil {
		t.Fatalf("make %s relative to %s: %v", target, root, err)
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		t.Fatalf("target %s must be under root %s, got rel %s", target, root, rel)
	}
	link := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-symlink-parent")
	if err := os.Symlink(filepath.Dir(root), link); err != nil {
		t.Fatalf("symlink path ancestor %s -> %s: %v", link, filepath.Dir(root), err)
	}
	return filepath.Join(link, filepath.Base(root), rel)
}

func repoPathWithSymlinkAncestorForTest(t *testing.T, target string) string {
	t.Helper()
	repoRoot := repoFilePathForTest(t)
	rel, err := filepath.Rel(repoRoot, target)
	if err != nil {
		t.Fatalf("make %s relative to repo root %s: %v", target, repoRoot, err)
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		t.Fatalf("target %s must be under repo root %s, got rel %s", target, repoRoot, rel)
	}
	link := filepath.Join(t.TempDir(), "repo-parent-link")
	if err := os.Symlink(filepath.Dir(repoRoot), link); err != nil {
		t.Fatalf("symlink repo parent %s -> %s: %v", link, filepath.Dir(repoRoot), err)
	}
	return filepath.Join(link, filepath.Base(repoRoot), rel)
}

func relativePathForTest(t *testing.T, path string) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	rel, err := filepath.Rel(wd, path)
	if err != nil {
		t.Fatalf("make relative path for %s: %v", path, err)
	}
	if filepath.IsAbs(rel) {
		t.Fatalf("expected relative path for %s, got %s", path, rel)
	}
	return rel
}

func validProductionAcceptancePolicyFixture() string {
	return strings.Join([]string{
		"schema=ouroboros.production_acceptance.policy.v1",
		"policy_status=approved",
		"deployment_environment=production",
		"accepted_by=operator",
		"accepted_at=2026-06-08T00:00:00Z",
		"authorization_scope=operator-owned-production-acceptance",
		"managed_payload_acceptance_required=true",
		"workflow_closure_required=true",
		"no_live_targets=true",
		"payload_execution_allowed=false",
		"network_download_allowed=false",
		"package_manager_allowed=false",
		"provider_calls_allowed=false",
		"worker_contact_allowed=false",
		"",
	}, "\n")
}

func runRejectedProductionAcceptanceSmoke(t *testing.T, inventoryPath, policyPath string) ([]byte, string) {
	t.Helper()
	toolDir := t.TempDir()
	fakeGoLog := filepath.Join(t.TempDir(), "fake-go.log")
	fakeGo := filepath.Join(toolDir, "go")
	writeTextFile(t, fakeGo, "#!/usr/bin/env sh\nprintf '%s\\n' \"$*\" >> \"$FAKE_GO_LOG\"\nexit 0\n")
	if err := os.Chmod(fakeGo, 0o755); err != nil {
		t.Fatalf("chmod fake go: %v", err)
	}
	logDir := filepath.Join(t.TempDir(), "logs")
	cmd := exec.Command("sh", "./scripts/smoke_production_acceptance.sh")
	cmd.Env = append(os.Environ(),
		"FAKE_GO_LOG="+fakeGoLog,
		"GOPROXY=https://goproxy.cn,direct",
		"OURO_PRODUCTION_ACCEPTANCE_SMOKE_LOG_DIR="+logDir,
		"OURO_PRODUCTION_ACCEPTANCE_INVENTORY="+inventoryPath,
		"OURO_PRODUCTION_ACCEPTANCE_POLICY="+policyPath,
		"PATH="+toolDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected production acceptance smoke to fail, got output:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(logDir, "release-managed.txt")); !os.IsNotExist(err) {
		t.Fatalf("production acceptance smoke must not start release-managed smoke, stat=%v", err)
	}
	return output, fakeGoLog
}

func assertProductionAcceptanceStageLogsAbsent(t *testing.T, logDir string) {
	t.Helper()
	for _, name := range []string{"release-managed.txt", "managed-payloads.txt", "workflow-closure.txt"} {
		if _, err := os.Stat(filepath.Join(logDir, name)); !os.IsNotExist(err) {
			t.Fatalf("production acceptance smoke must not start child stage before workspace acceptance; %s stat=%v", name, err)
		}
	}
}

func runRejectedManagedPayloadsSmoke(t *testing.T, binaryPayload, rulePayload string) ([]byte, string) {
	t.Helper()
	toolDir := t.TempDir()
	fakeGoLog := filepath.Join(t.TempDir(), "fake-go.log")
	fakeGo := filepath.Join(toolDir, "go")
	writeTextFile(t, fakeGo, "#!/usr/bin/env sh\nprintf '%s\\n' \"$*\" >> \"$FAKE_GO_LOG\"\nexit 0\n")
	if err := os.Chmod(fakeGo, 0o755); err != nil {
		t.Fatalf("chmod fake go: %v", err)
	}
	binarySHA := fileSHA256ForTest(t, strings.TrimPrefix(binaryPayload, "./"))
	ruleSHA := digestManagedPayloadsSmokeDirectory(t, rulePayload)
	logDir := filepath.Join(t.TempDir(), "logs")
	cmd := exec.Command("sh", "./scripts/smoke_managed_payloads.sh")
	cmd.Env = append(os.Environ(),
		"FAKE_GO_LOG="+fakeGoLog,
		"GOPROXY=https://goproxy.cn,direct",
		"OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR="+logDir,
		"OURO_MANAGED_BINARY_NAME=httpx",
		"OURO_MANAGED_BINARY_PAYLOAD="+binaryPayload,
		"OURO_MANAGED_BINARY_EXPECTED_SHA256="+binarySHA,
		"OURO_MANAGED_RULE_BUNDLE_NAME=nuclei-templates",
		"OURO_MANAGED_RULE_BUNDLE_PAYLOAD="+rulePayload,
		"OURO_MANAGED_RULE_BUNDLE_EXPECTED_SHA256="+ruleSHA,
		"PATH="+toolDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected managed payloads smoke to fail, got output:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(logDir, "binary-import.txt")); !os.IsNotExist(err) {
		t.Fatalf("managed payloads smoke must not start binary import for refused payload, stat=%v", err)
	}
	return output, fakeGoLog
}

func runManagedPayloadsSmokePathPreflight(t *testing.T, workspace, logDir string) ([]byte, string) {
	t.Helper()
	toolDir := t.TempDir()
	fakeGoLog := filepath.Join(t.TempDir(), "fake-go.log")
	fakeGo := filepath.Join(toolDir, "go")
	writeTextFile(t, fakeGo, "#!/usr/bin/env sh\nprintf '%s\\n' \"$*\" >> \"$FAKE_GO_LOG\"\nexit 0\n")
	if err := os.Chmod(fakeGo, 0o755); err != nil {
		t.Fatalf("chmod fake go: %v", err)
	}
	cmd := exec.Command("sh", "./scripts/smoke_managed_payloads.sh")
	cmd.Env = append(os.Environ(),
		"FAKE_GO_LOG="+fakeGoLog,
		"GOPROXY=https://goproxy.cn,direct",
		"WORKSPACE="+workspace,
		"OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR="+logDir,
		"PATH="+toolDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected managed payloads smoke path preflight to fail, got output:\n%s", output)
	}
	return output, fakeGoLog
}

func runWorkflowClosureSmokePathPreflight(t *testing.T, workspace, logDir string) ([]byte, string) {
	t.Helper()
	toolDir := t.TempDir()
	fakeGoLog := filepath.Join(t.TempDir(), "fake-go.log")
	fakeGo := filepath.Join(toolDir, "go")
	writeTextFile(t, fakeGo, "#!/usr/bin/env sh\nprintf '%s\\n' \"$*\" >> \"$FAKE_GO_LOG\"\nexit 0\n")
	if err := os.Chmod(fakeGo, 0o755); err != nil {
		t.Fatalf("chmod fake go: %v", err)
	}
	cmd := exec.Command("sh", "./scripts/smoke_workflow_closure.sh")
	cmd.Env = append(os.Environ(),
		"FAKE_GO_LOG="+fakeGoLog,
		"GOPROXY=https://goproxy.cn,direct",
		"WORKSPACE="+workspace,
		"OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR="+logDir,
		"PATH="+toolDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected workflow closure smoke path preflight to fail, got output:\n%s", output)
	}
	return output, fakeGoLog
}

func fileSHA256ForTest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestManagedPayloadsSmokeRefusesExistingConfiguredWorkspaceWithoutOptIn(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "scope.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	toolDir := t.TempDir()
	fakeGoLog := filepath.Join(t.TempDir(), "fake-go.log")
	fakeGo := filepath.Join(toolDir, "go")
	if err := os.WriteFile(fakeGo, []byte("#!/usr/bin/env sh\nprintf '%s\\n' \"$*\" >> \"$FAKE_GO_LOG\"\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake go: %v", err)
	}

	logDir := filepath.Join(t.TempDir(), "logs")
	cmd := exec.Command("sh", "./scripts/smoke_managed_payloads.sh")
	cmd.Env = append(os.Environ(),
		"FAKE_GO_LOG="+fakeGoLog,
		"GOPROXY=https://goproxy.cn,direct",
		"WORKSPACE="+workspace,
		"OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR="+logDir,
		"PATH="+toolDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected existing configured workspace to be refused, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "refusing existing non-empty workspace") {
		t.Fatalf("expected existing-workspace refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("managed payloads smoke must fail before invoking go, fake go log stat=%v", err)
	}
	if _, err := os.Stat(filepath.Join(logDir, "init.txt")); !os.IsNotExist(err) {
		t.Fatalf("managed payloads smoke must not create init log for refused workspace, stat=%v", err)
	}
}

func TestManagedPayloadsSmokeRejectsRelativeWorkspacePath(t *testing.T) {
	output, fakeGoLog := runManagedPayloadsSmokePathPreflight(t, relativePathForTest(t, filepath.Join(t.TempDir(), "workspace")), filepath.Join(t.TempDir(), "logs"))
	if !strings.Contains(string(output), "workspace path must be absolute") {
		t.Fatalf("expected relative workspace refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("managed payloads smoke must fail before invoking go, fake go log stat=%v", err)
	}
}

func TestManagedPayloadsSmokeRejectsSymlinkWorkspacePath(t *testing.T) {
	tempDir := t.TempDir()
	workspaceTarget := filepath.Join(tempDir, "workspace-target")
	if err := os.MkdirAll(workspaceTarget, 0o755); err != nil {
		t.Fatalf("create workspace target: %v", err)
	}
	workspaceLink := filepath.Join(tempDir, "workspace-link")
	if err := os.Symlink(workspaceTarget, workspaceLink); err != nil {
		t.Fatalf("symlink workspace: %v", err)
	}

	output, fakeGoLog := runManagedPayloadsSmokePathPreflight(t, workspaceLink, filepath.Join(t.TempDir(), "logs"))
	if !strings.Contains(string(output), "workspace directory must not be a symlink") {
		t.Fatalf("expected symlink workspace refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("managed payloads smoke must fail before invoking go, fake go log stat=%v", err)
	}
}

func TestManagedPayloadsSmokeRejectsSymlinkLogDir(t *testing.T) {
	tempDir := t.TempDir()
	outsideDir := filepath.Join(tempDir, "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("create outside dir: %v", err)
	}
	logDir := filepath.Join(tempDir, "logs")
	if err := os.Symlink(outsideDir, logDir); err != nil {
		t.Fatalf("symlink log dir: %v", err)
	}

	output, fakeGoLog := runManagedPayloadsSmokePathPreflight(t, filepath.Join(t.TempDir(), "workspace"), logDir)
	if !strings.Contains(string(output), "managed payloads log dir directory must not be a symlink") {
		t.Fatalf("expected symlink log dir refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("managed payloads smoke must fail before invoking go, fake go log stat=%v", err)
	}
}

func TestManagedPayloadsSmokeDirectExecUsesTrustedShellAndPathBeforePrelude(t *testing.T) {
	script := "./scripts/smoke_managed_payloads.sh"
	output, fakeShLog, fakeDirnameLog, fakeGoLog := runSmokeDirectExecTrustedPreludeCheck(t, script, "OURO_MANAGED_PAYLOADS_SMOKE_LOG_DIR")
	assertSmokeDirectExecTrustedPrelude(t, script, output, fakeShLog, fakeDirnameLog, fakeGoLog)
}

func TestSmokeWorkflowClosureRefusesExistingConfiguredWorkspaceWithoutOptIn(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "scope.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	toolDir := t.TempDir()
	fakeGoLog := filepath.Join(t.TempDir(), "fake-go.log")
	fakeGo := filepath.Join(toolDir, "go")
	if err := os.WriteFile(fakeGo, []byte("#!/usr/bin/env sh\nprintf '%s\\n' \"$*\" >> \"$FAKE_GO_LOG\"\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake go: %v", err)
	}

	logDir := filepath.Join(t.TempDir(), "logs")
	cmd := exec.Command("sh", "./scripts/smoke_workflow_closure.sh")
	cmd.Env = append(os.Environ(),
		"FAKE_GO_LOG="+fakeGoLog,
		"GOPROXY=https://goproxy.cn,direct",
		"WORKSPACE="+workspace,
		"OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR="+logDir,
		"PATH="+toolDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected existing configured workspace to be refused, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "refusing existing non-empty workspace") {
		t.Fatalf("expected existing-workspace refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("workflow closure smoke must fail before invoking go, fake go log stat=%v", err)
	}
	if _, err := os.Stat(filepath.Join(logDir, "init.txt")); !os.IsNotExist(err) {
		t.Fatalf("workflow closure smoke must not create init log for refused workspace, stat=%v", err)
	}
}

func TestSmokeWorkflowClosureRejectsRelativeWorkspacePath(t *testing.T) {
	output, fakeGoLog := runWorkflowClosureSmokePathPreflight(t, relativePathForTest(t, filepath.Join(t.TempDir(), "workspace")), filepath.Join(t.TempDir(), "logs"))
	if !strings.Contains(string(output), "workspace path must be absolute") {
		t.Fatalf("expected relative workspace refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("workflow closure smoke must fail before invoking go, fake go log stat=%v", err)
	}
}

func TestSmokeWorkflowClosureRejectsSymlinkWorkspacePath(t *testing.T) {
	tempDir := t.TempDir()
	workspaceTarget := filepath.Join(tempDir, "workspace-target")
	if err := os.MkdirAll(workspaceTarget, 0o755); err != nil {
		t.Fatalf("create workspace target: %v", err)
	}
	workspaceLink := filepath.Join(tempDir, "workspace-link")
	if err := os.Symlink(workspaceTarget, workspaceLink); err != nil {
		t.Fatalf("symlink workspace: %v", err)
	}

	output, fakeGoLog := runWorkflowClosureSmokePathPreflight(t, workspaceLink, filepath.Join(t.TempDir(), "logs"))
	if !strings.Contains(string(output), "workspace directory must not be a symlink") {
		t.Fatalf("expected symlink workspace refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("workflow closure smoke must fail before invoking go, fake go log stat=%v", err)
	}
}

func TestSmokeWorkflowClosureRejectsRelativeLogDir(t *testing.T) {
	output, fakeGoLog := runWorkflowClosureSmokePathPreflight(t, filepath.Join(t.TempDir(), "workspace"), relativePathForTest(t, filepath.Join(t.TempDir(), "logs")))
	if !strings.Contains(string(output), "workflow closure log dir path must be absolute") {
		t.Fatalf("expected relative log dir refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("workflow closure smoke must fail before invoking go, fake go log stat=%v", err)
	}
}

func TestSmokeWorkflowClosureRejectsSymlinkLogDir(t *testing.T) {
	tempDir := t.TempDir()
	outsideDir := filepath.Join(tempDir, "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("create outside dir: %v", err)
	}
	logDir := filepath.Join(tempDir, "logs")
	if err := os.Symlink(outsideDir, logDir); err != nil {
		t.Fatalf("symlink log dir: %v", err)
	}

	output, fakeGoLog := runWorkflowClosureSmokePathPreflight(t, filepath.Join(t.TempDir(), "workspace"), logDir)
	if !strings.Contains(string(output), "workflow closure log dir directory must not be a symlink") {
		t.Fatalf("expected symlink log dir refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("workflow closure smoke must fail before invoking go, fake go log stat=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "init.txt")); !os.IsNotExist(err) {
		t.Fatalf("workflow closure smoke must not write through symlink log dir, stat=%v", err)
	}
}

func TestSmokeWorkflowClosureDirectExecUsesTrustedShellAndPathBeforePrelude(t *testing.T) {
	script := "./scripts/smoke_workflow_closure.sh"
	output, fakeShLog, fakeDirnameLog, fakeGoLog := runSmokeDirectExecTrustedPreludeCheck(t, script, "OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR")
	assertSmokeDirectExecTrustedPrelude(t, script, output, fakeShLog, fakeDirnameLog, fakeGoLog)
}

func TestSmokeWorkflowClosureBindsReviewArtifactFromWorkflowStatusLinks(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	reviewDir := filepath.Join(workspace, "toolchain", "compile-preview-reviews")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		t.Fatalf("create review dir: %v", err)
	}
	staleReview := filepath.Join(reviewDir, "stale-review.json")
	writeTextFile(t, staleReview, `{"stale":true}`+"\n")
	currentReview := filepath.Join(workspace, "toolchain", "current review.json")
	if err := os.MkdirAll(filepath.Dir(currentReview), 0o755); err != nil {
		t.Fatalf("create current review dir: %v", err)
	}
	writeTextFile(t, currentReview, `{"current":true}`+"\n")

	fakeRoot := t.TempDir()
	scriptDir := filepath.Join(fakeRoot, "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("create fake scripts dir: %v", err)
	}
	scriptBytes, err := os.ReadFile("./scripts/smoke_workflow_closure.sh")
	if err != nil {
		t.Fatalf("read workflow closure script: %v", err)
	}
	fakeScript := filepath.Join(scriptDir, "smoke_workflow_closure.sh")
	if err := os.WriteFile(fakeScript, scriptBytes, 0o755); err != nil {
		t.Fatalf("write fake workflow closure script: %v", err)
	}
	freshBinBytes, err := os.ReadFile("./scripts/smoke_fresh_bin.sh")
	if err != nil {
		t.Fatalf("read fresh bin script: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptDir, "smoke_fresh_bin.sh"), freshBinBytes, 0o755); err != nil {
		t.Fatalf("write fake fresh bin script: %v", err)
	}
	writeTextFile(t, filepath.Join(fakeRoot, "go.mod"), "module fakeouro\n\ngo 1.22\n")
	fakeCmdDir := filepath.Join(fakeRoot, "cmd", "ouro")
	if err := os.MkdirAll(fakeCmdDir, 0o755); err != nil {
		t.Fatalf("create fake cmd/ouro dir: %v", err)
	}
	writeTextFile(t, filepath.Join(fakeCmdDir, "main.go"), `package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func write(path, content string) {
	must(os.MkdirAll(filepath.Dir(path), 0o755))
	must(os.WriteFile(path, []byte(content), 0o644))
}

func appendLog(args []string) {
	path := os.Getenv("FAKE_GO_LOG")
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	must(err)
	defer file.Close()
	_, err = fmt.Fprintln(file, strings.Join(args, " "))
	must(err)
}

func nextGate() string {
	path := os.Getenv("FAKE_WORKFLOW_STEP_FILE")
	step := 0
	if data, err := os.ReadFile(path); err == nil {
		step, _ = strconv.Atoi(strings.TrimSpace(string(data)))
	}
	step++
	must(os.WriteFile(path, []byte(strconv.Itoa(step)), 0o644))
	switch step {
	case 1:
		return "create-compile-preview-review-execution-dispatch"
	case 2:
		return "execute-compile-preview-review-execution-dispatch"
	case 3:
		return "create-report-projection-dispatch"
	case 4:
		return "execute-report-projection-dispatch"
	case 5:
		return "review-report-projection-execution"
	case 6:
		return "select-report-projection-execution"
	default:
		return "review-current-report-projection"
	}
}

func main() {
	args := os.Args[1:]
	appendLog(args)
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "missing command")
		os.Exit(1)
	}
	switch args[0] {
	case "init":
		return
	case "doctor":
		fmt.Println("workspace: ready")
	case "run":
		fmt.Println("run: run-linked-review")
		fmt.Println("report: not-generated")
	case "workflow-status":
		gate := nextGate()
		fmt.Printf("workflow-status: status=ready mode=operator stage=model-tool-plan next_gate=%s reason=fake action=enumerate_paths previews=1 acceptances=1 dispatches=0 executions=0 report_dispatches=0 report_executions=0 safety_errors=0\n", gate)
		fmt.Printf("workflow-links: review=%s report_run=run-linked-review\n", os.Getenv("FAKE_CURRENT_REVIEW"))
	case "toolchain":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "missing toolchain args")
			os.Exit(1)
		}
		workspace := args[2]
		switch args[1] {
		case "compile-preview-review-execution-dispatch":
			if len(args) < 4 || args[3] != os.Getenv("FAKE_CURRENT_REVIEW") {
				fmt.Fprintf(os.Stderr, "unexpected review artifact: %s\n", strings.Join(args, " "))
				os.Exit(42)
			}
			dispatch := filepath.Join(workspace, "toolchain", "compile-preview-review-execution-dispatches", "fake-dispatch.json")
			write(dispatch, "{\"dispatch\":true}\n")
			fmt.Println("compile-preview-review-execution-dispatch: fake status=dry-run mode=metadata-only")
			fmt.Printf("artifact: %s\n", dispatch)
		case "execute-compile-preview-review-execution-dispatch":
			execution := filepath.Join(workspace, "toolchain", "compile-preview-review-executions", "fake-execution.json")
			write(execution, "{\"execution\":true}\n")
			fmt.Println("compile-preview-review-execution: run-linked-review status=completed mode=execute")
			fmt.Printf("artifact: %s\n", execution)
		default:
			fmt.Fprintf(os.Stderr, "unexpected toolchain subcommand: %s\n", args[1])
			os.Exit(1)
		}
	case "report-project-dispatch":
		workspace := args[1]
		dispatch := filepath.Join(workspace, "reports", "report-projection-dispatches", "fake-report-dispatch.json")
		write(dispatch, "{\"report_dispatch\":true}\n")
		fmt.Println("report-project-dispatch: fake status=dry-run mode=metadata-only")
		fmt.Printf("artifact: %s\n", dispatch)
	case "execute-report-project-dispatch":
		workspace := args[1]
		execution := filepath.Join(workspace, "reports", "report-projection-dispatch-executions", "fake-report-execution.json")
		report := filepath.Join(workspace, "reports", "fake-report.md")
		write(execution, "{\"report_execution\":true}\n")
		write(report, "fake report\n")
		fmt.Println("report-project-dispatch-execution: fake status=completed mode=execute report=fake written=true")
		fmt.Printf("artifact: %s\n", execution)
		fmt.Printf("report: %s\n", report)
	case "report-project-dispatch-execution-annotate":
		fmt.Println("report-project-dispatch-execution-annotation: decision=accepted")
	case "report-project-dispatch-execution-select":
		workspace := args[1]
		selection := filepath.Join(workspace, "reports", "report-projection-selection.json")
		write(selection, "{\"status\":\"current\"}\n")
		fmt.Println("report-project-dispatch-execution-selection: status=current")
	case "report-project-dispatch-execution-selection":
		fmt.Println("report-project-dispatch-execution-selection: status=current")
	case "replay":
		runID := "run-linked-review"
		if len(args) > 2 {
			runID = args[2]
		}
		fmt.Printf("replay: %s\n", runID)
		fmt.Println("Workspace")
		fmt.Println("Mission Conversation")
		fmt.Println("Attack Graph")
		fmt.Println("Runtime Tabs")
	default:
		fmt.Fprintf(os.Stderr, "unexpected fake ouro args: %s\n", strings.Join(args, " "))
		os.Exit(1)
	}
}
`)

	fakeGoLog := filepath.Join(t.TempDir(), "fake-go.log")
	stepFile := filepath.Join(t.TempDir(), "workflow-step")
	logDir := filepath.Join(t.TempDir(), "logs")
	cmd := exec.Command(fakeScript)
	cmd.Env = append(os.Environ(),
		"FAKE_GO_LOG="+fakeGoLog,
		"FAKE_WORKFLOW_STEP_FILE="+stepFile,
		"FAKE_CURRENT_REVIEW="+currentReview,
		"GOCACHE="+filepath.Join(fakeRoot, "gocache"),
		"GOPROXY=https://goproxy.cn,direct",
		"WORKSPACE="+workspace,
		"OURO_WORKFLOW_CLOSURE_SMOKE_LOG_DIR="+logDir,
		"OURO_WORKFLOW_CLOSURE_SMOKE_ALLOW_EXISTING_WORKSPACE=1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("workflow closure smoke should use workflow-links review artifact, err=%v output:\n%s", err, output)
	}
	if !strings.Contains(string(output), "workflow-closure-smoke: ok") {
		t.Fatalf("expected workflow closure smoke ok, got:\n%s", output)
	}
	fakeLog := readTextFileForTest(t, fakeGoLog)
	if strings.Contains(fakeLog, staleReview) {
		t.Fatalf("workflow closure smoke must not dispatch stale review artifact, fake go log:\n%s", fakeLog)
	}
	if !strings.Contains(fakeLog, currentReview) {
		t.Fatalf("workflow closure smoke must dispatch workflow-links review artifact %s, fake go log:\n%s", currentReview, fakeLog)
	}
}

func TestReleaseManagedSmokeRefusesExistingConfiguredWorkspaceWithoutOptIn(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "scope.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	toolDir := t.TempDir()
	fakeGoLog := filepath.Join(t.TempDir(), "fake-go.log")
	fakeGo := filepath.Join(toolDir, "go")
	if err := os.WriteFile(fakeGo, []byte("#!/usr/bin/env sh\nprintf '%s\\n' \"$*\" >> \"$FAKE_GO_LOG\"\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake go: %v", err)
	}

	logDir := filepath.Join(t.TempDir(), "logs")
	cmd := exec.Command("sh", "./scripts/smoke_release_managed.sh")
	cmd.Env = append(os.Environ(),
		"FAKE_GO_LOG="+fakeGoLog,
		"GOPROXY=https://goproxy.cn,direct",
		"WORKSPACE="+workspace,
		"OURO_RELEASE_MANAGED_SMOKE_LOG_DIR="+logDir,
		"PATH="+toolDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected existing configured workspace to be refused, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "refusing existing non-empty workspace") {
		t.Fatalf("expected existing-workspace refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("release-managed smoke must fail before invoking go, fake go log stat=%v", err)
	}
	if _, err := os.Stat(filepath.Join(logDir, "release.txt")); !os.IsNotExist(err) {
		t.Fatalf("release-managed smoke must not create release log for refused workspace, stat=%v", err)
	}
}

func TestReleaseManagedSmokeDirectExecUsesTrustedShellAndPathBeforePrelude(t *testing.T) {
	script := "./scripts/smoke_release_managed.sh"
	output, fakeShLog, fakeDirnameLog, fakeGoLog := runSmokeDirectExecTrustedPreludeCheck(t, script, "OURO_RELEASE_MANAGED_SMOKE_LOG_DIR")
	assertSmokeDirectExecTrustedPrelude(t, script, output, fakeShLog, fakeDirnameLog, fakeGoLog)
}

func TestSmokeWorkflowRefusesExistingConfiguredWorkspaceWithoutOptIn(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://example.test
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(workspace, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	toolDir := t.TempDir()
	fakeGoLog := filepath.Join(t.TempDir(), "fake-go.log")
	fakeGo := filepath.Join(toolDir, "go")
	fakeGoScript := `#!/usr/bin/env sh
set -eu
printf '%s\n' "$*" >> "$FAKE_GO_LOG"
case "${3:-}" in
init)
	exit 0
	;;
doctor)
	printf 'workspace: ready\n'
	exit 0
	;;
run)
	report="${4}/reports/run-fake.md"
	mkdir -p "$(dirname "$report")"
	printf 'fake report\n' > "$report"
	printf 'run: run-fake\n'
	printf 'events: 1\n'
	printf 'report: %s\n' "$report"
	printf 'Workspace\nMission Conversation\nAttack Graph\nRuntime Tabs\n'
	exit 0
	;;
replay)
	printf 'replay: %s\n' "${5:-run-fake}"
	printf 'tools: ready\n'
	printf 'Workspace\nMission Conversation\nAttack Graph\nRuntime Tabs\n'
	exit 0
	;;
workflow-status)
	printf 'workflow-status: status=ready mode=operator stage=compile next_gate=create-compile-preview-review-execution-dispatch\n'
	exit 0
	;;
*)
	printf 'unexpected fake go args: %s\n' "$*" >&2
	exit 1
	;;
esac
`
	if err := os.WriteFile(fakeGo, []byte(fakeGoScript), 0o755); err != nil {
		t.Fatalf("write fake go: %v", err)
	}

	logDir := filepath.Join(t.TempDir(), "logs")
	cmd := exec.Command("sh", "./scripts/smoke_workflow.sh")
	cmd.Env = append(os.Environ(),
		"FAKE_GO_LOG="+fakeGoLog,
		"GOPROXY=https://goproxy.cn,direct",
		"WORKSPACE="+workspace,
		"OURO_SMOKE_LOG_DIR="+logDir,
		"PATH="+toolDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected existing configured workspace to be refused, got output:\n%s", output)
	}
	if !strings.Contains(string(output), "refusing existing non-empty workspace") {
		t.Fatalf("expected existing-workspace refusal, got:\n%s", output)
	}
	if _, err := os.Stat(fakeGoLog); !os.IsNotExist(err) {
		t.Fatalf("smoke script must fail before invoking go, fake go log stat=%v", err)
	}
	if _, err := os.Stat(filepath.Join(logDir, "run.txt")); !os.IsNotExist(err) {
		t.Fatalf("smoke script must not create run log for refused workspace, stat=%v", err)
	}
}

func TestSmokeWorkflowDirectExecUsesTrustedShellAndPathBeforePrelude(t *testing.T) {
	script := "./scripts/smoke_workflow.sh"
	output, fakeShLog, fakeDirnameLog, fakeGoLog := runSmokeDirectExecTrustedPreludeCheck(t, script, "OURO_SMOKE_LOG_DIR")
	assertSmokeDirectExecTrustedPrelude(t, script, output, fakeShLog, fakeDirnameLog, fakeGoLog)
}

func tarGzMembers(t *testing.T, archivePath string) map[string]bool {
	t.Helper()
	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("open gzip: %v", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	members := map[string]bool{}
	for {
		header, err := reader.Next()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("read tar: %v", err)
		}
		members[header.Name] = true
	}
	return members
}

func tarGzFileContent(t *testing.T, archivePath string, memberName string) string {
	t.Helper()
	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("open gzip: %v", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("read tar: %v", err)
		}
		if header.Name != memberName {
			continue
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read tar member %s: %v", memberName, err)
		}
		return string(content)
	}
	t.Fatalf("archive member %s not found", memberName)
	return ""
}

func createTarGz(t *testing.T, archivePath string, root string, names ...string) {
	t.Helper()
	out, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer out.Close()
	gz := gzip.NewWriter(out)
	defer gz.Close()
	writer := tar.NewWriter(gz)
	defer writer.Close()
	for _, name := range names {
		path := filepath.Join(root, filepath.FromSlash(name))
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat tar member: %v", err)
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			t.Fatalf("tar header: %v", err)
		}
		header.Name = name
		if err := writer.WriteHeader(header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read tar member: %v", err)
		}
		if _, err := writer.Write(content); err != nil {
			t.Fatalf("write tar member: %v", err)
		}
	}
}

func writeManagedPayloadsSmokeRuleBundleFixture(t *testing.T, root string, files map[string]string) string {
	t.Helper()
	payloadDir := filepath.Join(root, "nuclei-templates")
	for rel, body := range files {
		path := filepath.Join(payloadDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create smoke rule bundle dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write smoke rule bundle payload: %v", err)
		}
	}
	return payloadDir
}

func digestManagedPayloadsSmokeDirectory(t *testing.T, dir string) string {
	t.Helper()
	paths := []string{}
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			t.Fatalf("unexpected symlink in smoke rule bundle fixture: %s", path)
		}
		if !entry.Type().IsRegular() {
			t.Fatalf("unexpected non-regular file in smoke rule bundle fixture: %s", path)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			t.Fatalf("relative smoke rule bundle path: %v", err)
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		t.Fatalf("walk smoke rule bundle fixture: %v", err)
	}
	sort.Strings(paths)
	hash := sha256.New()
	hash.Write([]byte("ouroboros managed rule_bundle directory digest v1\n"))
	for _, rel := range paths {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read smoke rule bundle fixture: %v", err)
		}
		var field [8]byte
		relBytes := []byte(rel)
		binary.BigEndian.PutUint64(field[:], uint64(len(relBytes)))
		hash.Write(field[:])
		hash.Write(relBytes)
		binary.BigEndian.PutUint64(field[:], uint64(len(data)))
		hash.Write(field[:])
		digest := sha256.Sum256(data)
		hash.Write(digest[:])
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func createInstallScriptArchive(t *testing.T) (string, string) {
	t.Helper()
	releaseRoot := t.TempDir()
	payloadDir := filepath.Join(releaseRoot, "ouro_0.1.0-test_linux_amd64")
	if err := os.MkdirAll(payloadDir, 0o755); err != nil {
		t.Fatalf("mkdir payload: %v", err)
	}
	binary := filepath.Join(payloadDir, "ouro")
	if err := os.WriteFile(binary, []byte("#!/usr/bin/env sh\nprintf 'fake ouro\\n'\n"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	archivePath := filepath.Join(releaseRoot, "ouro_0.1.0-test_linux_amd64.tar.gz")
	createTarGz(t, archivePath, releaseRoot, filepath.Base(payloadDir)+"/ouro")
	return releaseRoot, archivePath
}

func writeReleaseScriptManagedBundleFixture(t *testing.T, name string) {
	t.Helper()
	bundleDir := filepath.Join("toolchains", "managed-bundles", name)
	t.Cleanup(func() {
		_ = os.RemoveAll(bundleDir)
	})
	payloadBody := []byte("#!/usr/bin/env sh\nexit 0\n")
	if err := os.MkdirAll(filepath.Join(bundleDir, "payload"), 0o755); err != nil {
		t.Fatalf("create release script bundle payload dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "payload", name), payloadBody, 0o755); err != nil {
		t.Fatalf("write release script bundle payload: %v", err)
	}
	digest := sha256.Sum256(payloadBody)
	manifest := `name: ` + name + `
kind: binary
version: "1.3.0"
source: projectdiscovery/` + name + `
payload: payload/` + name + `
sha256: ` + hex.EncodeToString(digest[:]) + `
`
	if err := os.WriteFile(filepath.Join(bundleDir, "bundle.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write release script bundle manifest: %v", err)
	}
}
