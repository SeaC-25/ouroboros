package copymain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyRecursesAndOverwrites(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src", "main", "nested")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		filepath.Join(root, "src", "main", "main.js"):        "main-v1",
		filepath.Join(root, "src", "main", "nested", "a.js"): "a",
		filepath.Join(root, "dist", "main", "stale.js"):      "stale",
		filepath.Join(root, "dist", "main", "main.js"):       "old",
	} {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dest, err := Copy(root)
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if want := filepath.Join(root, "dist", "main"); dest != want {
		t.Fatalf("dest = %q, want %q", dest, want)
	}
	assertFile(t, filepath.Join(dest, "main.js"), "main-v1")
	assertFile(t, filepath.Join(dest, "nested", "a.js"), "a")
	// cpSync without explicit removal keeps pre-existing files.
	assertFile(t, filepath.Join(dest, "stale.js"), "stale")
}

func TestCopyMissingSourceIsError(t *testing.T) {
	root := t.TempDir()
	if _, err := Copy(root); err == nil {
		t.Fatal("expected error for missing src/main")
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
