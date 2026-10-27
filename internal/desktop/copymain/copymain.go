// Package copymain mirrors gui/scripts/copy-main.mjs: recursively copy
// gui/src/main into gui/dist/main, creating the destination tree.
package copymain

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// MainDir is the source directory relative to the GUI root, matching
// join(guiRoot, 'src', 'main') in copy-main.mjs.
const MainDir = "src/main"

// DestDir is the copy destination relative to the GUI root, matching
// join(guiRoot, 'dist', 'main') in copy-main.mjs.
const DestDir = "dist/main"

// Copy copies <guiRoot>/src/main into <guiRoot>/dist/main and returns the
// destination path. It mirrors cpSync(src, dest, {recursive, force: true}):
// existing files are overwritten, directories are merged.
func Copy(guiRoot string) (string, error) {
	dest := filepath.Join(guiRoot, DestDir)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", dest, err)
	}
	src := filepath.Join(guiRoot, MainDir)
	if err := CopyTree(src, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// CopyTree recursively copies src into dst using filepath.Walk, overwriting
// existing regular files (force semantics of cpSync).
func CopyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source %s is not a directory", src)
	}
	return filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", path, err)
		}
		target := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
				return fmt.Errorf("create directory %s: %w", target, err)
			}
		case info.Mode().IsRegular():
			if err := copyFile(path, target, info.Mode().Perm()); err != nil {
				return err
			}
		default:
			// cpSync copies symlinks as links; resolve them like Node does
			// when dereferencing content. Symlinks in this tree are not
			// expected; treat anything else as an error with the path.
			return fmt.Errorf("unsupported file type at %s", path)
		}
		return nil
	})
}

func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}
