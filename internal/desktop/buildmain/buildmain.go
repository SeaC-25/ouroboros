// Package buildmain mirrors gui/scripts/build-desktop-main.mjs: assemble the
// desktop main-process bundle into dist/main (or OURO_DESKTOP_MAIN_OUTPUT),
// renaming entry points so the packaged electron overlay can shadow them.
package buildmain

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ouroboros/ouroboros/internal/desktop/copymain"
)

// OutputEnvVar overrides the output directory, matching
// process.env.OURO_DESKTOP_MAIN_OUTPUT in build-desktop-main.mjs.
const OutputEnvVar = "OURO_DESKTOP_MAIN_OUTPUT"

// requiredOutputs must all exist after the build, matching the check list in
// build-desktop-main.mjs.
var requiredOutputs = []string{
	"main.js",
	"application-main.js",
	"backend.js",
	"base-backend.js",
	"memory-profile.js",
	"preload.js",
	"workspace.js",
	"desktop-state.js",
	"assets/ouroboros-icon.png",
}

// Options controls Build. Output overrides the destination directly; when
// empty, env (OURO_DESKTOP_MAIN_OUTPUT) and then <guiRoot>/dist/main apply,
// in that order — the same precedence as the mjs script.
type Options struct {
	Output string
}

// Build assembles the desktop main bundle under guiRoot and prints
// "desktop main ready: <output>" to out, exactly like the mjs script.
func Build(guiRoot string, opts Options, out io.Writer) error {
	return BuildWithEnv(guiRoot, opts, os.Getenv, out)
}

// BuildWithEnv is Build with an injectable environment lookup for tests.
func BuildWithEnv(guiRoot string, opts Options, getenv func(string) string, out io.Writer) error {
	outputMain := opts.Output
	if outputMain == "" {
		if env := getenv(OutputEnvVar); env != "" {
			outputMain = env
		} else {
			outputMain = filepath.Join(guiRoot, "dist", "main")
		}
	}
	sourceMain := filepath.Join(guiRoot, "src", "main")
	packagedMain := filepath.Join(guiRoot, "electron")

	// rmSync(outputMain, {recursive, force}); mkdirSync(outputMain, {recursive})
	if err := os.RemoveAll(outputMain); err != nil {
		return fmt.Errorf("remove %s: %w", outputMain, err)
	}
	if err := os.MkdirAll(outputMain, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", outputMain, err)
	}

	// cpSync(sourceMain, outputMain, {recursive})
	if err := copymain.CopyTree(sourceMain, outputMain); err != nil {
		return err
	}

	// renameRequired main.js -> application-main.js, backend.js -> base-backend.js
	if err := renameRequired(
		filepath.Join(outputMain, "main.js"),
		filepath.Join(outputMain, "application-main.js"),
	); err != nil {
		return err
	}
	if err := renameRequired(
		filepath.Join(outputMain, "backend.js"),
		filepath.Join(outputMain, "base-backend.js"),
	); err != nil {
		return err
	}

	// cpSync(packagedMain, outputMain, {recursive}) — overlay packaged files.
	if err := copymain.CopyTree(packagedMain, outputMain); err != nil {
		return err
	}

	for _, required := range requiredOutputs {
		path := filepath.Join(outputMain, filepath.FromSlash(required))
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("desktop main build missing %s", required)
		}
	}

	if out != nil {
		fmt.Fprintf(out, "desktop main ready: %s\n", outputMain)
	}
	return nil
}

// renameRequired mirrors renameRequired in build-desktop-main.mjs: missing
// source is a hard error carrying the absolute-ish source path.
func renameRequired(source, destination string) error {
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("desktop main source missing: %s", source)
	}
	if err := os.Rename(source, destination); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", source, destination, err)
	}
	return nil
}
