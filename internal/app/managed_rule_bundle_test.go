package app

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManagedRuleBundleFixtureForRunTest(t *testing.T, root string, name string, version string, files map[string]string) string {
	t.Helper()
	bundleDir := filepath.Join(root, "toolchain", "managed-bundles", name)
	payloadDir := filepath.Join(bundleDir, "payload")
	for rel, body := range files {
		path := filepath.Join(payloadDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create managed rule payload dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write managed rule payload: %v", err)
		}
	}
	digest := digestDirectoryForAppTest(t, payloadDir)
	manifest := `name: ` + name + `
kind: rule_bundle
version: "` + version + `"
source: projectdiscovery/` + name + `
payload: payload
sha256: ` + digest + `
`
	if err := os.WriteFile(filepath.Join(bundleDir, "bundle.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write managed rule bundle manifest: %v", err)
	}
	return payloadDir
}
