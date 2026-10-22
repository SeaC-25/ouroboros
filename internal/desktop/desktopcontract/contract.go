package desktopcontract

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Options configures Check.
type Options struct {
	// GUIRoot is the gui/ directory (contains src/, electron/, scripts/, package.json).
	GUIRoot string
	// RepoRoot is the repository root (parent of gui/).
	RepoRoot string
	// Stderr receives contract failure messages. Defaults to os.Stderr.
	Stderr io.Writer
	// Node is the node executable used for the dynamic module contracts.
	// Defaults to "node".
	Node string
}

// Check runs the desktop contract. Returns 0 on success, 1 on contract
// failure and 2 on environment/IO errors (e.g. node missing).
func Check(o Options) int {
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	if o.Node == "" {
		o.Node = "node"
	}
	r := &runner{
		opts:   o,
		read:   func(file string) string { return readTolerant(joinPath(o.GUIRoot, file)) },
		readRP: func(file string) string { return readTolerant(joinPath(o.RepoRoot, file)) },
	}
	return r.run()
}

func joinPath(base, rel string) string {
	return filepath.Join(base, filepath.FromSlash(rel))
}

// readTolerant mirrors the mjs read()/readRepo() helpers: a missing file
// yields an empty string so the presence assertions fail instead of crashing.
func readTolerant(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

type runner struct {
	opts   Options
	failed bool
	envErr bool
	read   func(string) string
	readRP func(string) string
}

func (r *runner) errf(format string, args ...any) {
	fmt.Fprintf(r.opts.Stderr, format+"\n", args...)
	r.failed = true
}

// requireAll fails once per needle missing from src.
func (r *runner) requireAll(message string, src string, needles []string) {
	for _, needle := range needles {
		if !strings.Contains(src, needle) {
			r.errf(message, needle)
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (r *runner) run() int {
	r.checkTopLevel()
	r.runNodeContract(dynamicMemoryContractDriver, joinPath(r.opts.GUIRoot, "electron/memory-profile.js"))
	r.runNodeContract(dynamicBackendExitContractDriver, joinPath(r.opts.GUIRoot, "src/main/backend.js"))
	if fileExists(joinPath(r.opts.RepoRoot, "scripts/desktop_runtime_pid.mjs")) {
		r.runNodeContract(dynamicRuntimePIDContractDriver,
			r.opts.RepoRoot,
			joinPath(r.opts.GUIRoot, "src/main/backend.js"),
			joinPath(r.opts.RepoRoot, "scripts/desktop_runtime_pid.mjs"),
		)
	}
	if fileExists(joinPath(r.opts.RepoRoot, "scripts/install_desktop_app.sh")) {
		r.runNodeContract(dynamicInstallerContractDriver, r.opts.RepoRoot)
	}
	if r.envErr {
		return 2
	}
	if r.failed {
		return 1
	}
	return 0
}

// indexOfFrom mirrors String.prototype.indexOf(needle, fromIndex):
// -1 when absent; the search starts at fromIndex bytes in.
func indexOfFrom(s, needle string, from int) int {
	if from < 0 {
		from = 0
	} else if from > len(s) {
		return -1
	}
	rel := strings.Index(s[from:], needle)
	if rel < 0 {
		return -1
	}
	return from + rel
}

func mustPattern(expr string) *regexp.Regexp {
	return regexp.MustCompile(expr)
}
