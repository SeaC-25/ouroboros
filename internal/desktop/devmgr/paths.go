package devmgr

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed drivers/vite-driver.mjs
var viteDriverSource []byte

// fileExists reports whether path is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// findGUIRoot walks upwards from the current working directory until it
// finds a directory that contains gui/scripts/dev-desktop.mjs, returning the
// gui root. Mirrors how the mjs script anchors itself relative to its own
// location.
func findGUIRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", &EnvError{What: "resolve working directory", Err: err}
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if isGUIRoot(dir) {
			return dir, nil
		}
		candidate := filepath.Join(dir, "gui")
		if isGUIRoot(candidate) {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", &EnvError{
		What: fmt.Sprintf("locate gui root walking up from %s", cwd),
		Err:  os.ErrNotExist,
	}
}

// materializeDriver writes the embedded vite driver into a temporary
// directory directly under the gui root so the driver's bare `vite` import
// resolves against the gui tree's node_modules, mirroring the mjs script's
// module resolution from gui/scripts/. Callers remove the file afterwards.
func materializeDriver(guiRoot, name string, source []byte) (string, error) {
	dir, err := os.MkdirTemp(guiRoot, ".devmgr-")
	if err != nil {
		return "", &EnvError{What: fmt.Sprintf("create driver dir under %s", guiRoot), Err: err}
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, source, 0o644); err != nil {
		_ = os.RemoveAll(dir)
		return "", &EnvError{What: fmt.Sprintf("write driver %s", path), Err: err}
	}
	return path, nil
}

// nodeBinary resolves the node executable; the mjs script uses
// process.execPath, for which the closest host equivalent is whatever node
// is on PATH.
func nodeBinary() (string, error) {
	path, err := exec.LookPath("node")
	if err != nil {
		return "", &EnvError{What: "locate node on PATH", Err: err}
	}
	return path, nil
}

// electronBinary resolves the electron executable the way
// `import electronPath from 'electron'` does in the mjs script: the dist
// binary inside the gui tree's node_modules/electron package.
func electronBinary(guiRoot string) (string, error) {
	path := filepath.Join(guiRoot, "node_modules", "electron", "dist", "electron"+exeSuffix)
	if !fileExists(path) {
		return "", &EnvError{
			What: fmt.Sprintf("locate electron binary at %s", path),
			Err:  os.ErrNotExist,
		}
	}
	return path, nil
}
