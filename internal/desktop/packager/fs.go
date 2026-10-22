package packager

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// FS abstracts the filesystem operations the packager needs so tests can
// substitute an in-memory implementation.
type FS interface {
	MkdirAll(path string, perm fs.FileMode) error
	RemoveAll(path string) error
	WriteFile(path string, data []byte, perm fs.FileMode) error
	CopyFile(src, dst string) error
	CopyDir(src, dst string) error
	Rename(oldpath, newpath string) error
	Chmod(path string, mode fs.FileMode) error
	Stat(path string) (fs.FileInfo, error)
}

// OSFS is the production FS backed by the os package.
type OSFS struct{}

func (OSFS) MkdirAll(path string, perm fs.FileMode) error { return os.MkdirAll(path, perm) }
func (OSFS) RemoveAll(path string) error                  { return os.RemoveAll(path) }
func (OSFS) WriteFile(path string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(path, data, perm)
}
func (OSFS) Rename(oldpath, newpath string) error      { return os.Rename(oldpath, newpath) }
func (OSFS) Chmod(path string, mode fs.FileMode) error { return os.Chmod(path, mode) }
func (OSFS) Stat(path string) (fs.FileInfo, error)     { return os.Stat(path) }

func (OSFS) CopyFile(src, dst string) error { return copyFile(src, dst) }
func (OSFS) CopyDir(src, dst string) error  { return copyDir(src, dst) }

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// copyDir mirrors `cp -r` / Node's cpSync(recursive): the destination is
// created and the source tree merged into it.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			os.Remove(target)
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			if err := os.MkdirAll(filepath.Dir(target), 0o777); err != nil {
				return err
			}
			return copyFile(path, target)
		default:
			return fmt.Errorf("unsupported file type: %s", path)
		}
	})
}
