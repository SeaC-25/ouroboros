package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func requireReplayWorkbenchDirectory(path string, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s missing: %s", label, path)
		}
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must not be a symlink: %s", label, path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory: %s", label, path)
	}
	return nil
}

func rejectUnsafeReplayWorkbenchDirectory(path string, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must not be a symlink: %s", label, path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s path is not a directory: %s", label, path)
	}
	return nil
}

func rejectUnsafeReplayWorkbenchFile(path string, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must not be a symlink: %s", label, path)
	}
	if info.IsDir() {
		return fmt.Errorf("write %s: path is a directory: %s", label, path)
	}
	return nil
}

func ensureReplayWorkbenchDirectory(path string, label string) error {
	if err := rejectUnsafeReplayWorkbenchDirectory(path, label); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", label, err)
	}
	return requireReplayWorkbenchDirectory(path, label)
}

func resolveReplayWorkbenchJSONPath(dir string, input string, label string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", fmt.Errorf("%s path is required", label)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve %s directory: %w", label, err)
	}
	if err := rejectUnsafeReplayWorkbenchDirectory(absDir, label+" directory"); err != nil {
		return "", err
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(absDir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve %s path: %w", label, err)
	}
	rel, err := filepath.Rel(absDir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve %s relative path: %w", label, err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%s path must stay under %s", label, absDir)
	}
	if strings.Contains(rel, string(os.PathSeparator)) {
		return "", fmt.Errorf("%s path must be a direct json file under %s", label, absDir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("%s path must be a json file", label)
	}
	return candidate, nil
}
