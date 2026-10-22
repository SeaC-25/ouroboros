// desktop 是 Ouroboros 桌面工具链的 Go 入口,替代原 gui/scripts/*.mjs。
//
// 子命令对照原 .mjs 脚本:
//
//	dev              ← scripts/dev-desktop.mjs
//	build-main       ← scripts/build-desktop-main.mjs
//	copy-main        ← scripts/copy-main.mjs
//	package          ← scripts/package-desktop.mjs
//	install          ← scripts/install-desktop.mjs
//	runtime-smoke    ← scripts/run-desktop-runtime-smoke.mjs
//	check-electron-runtime ← scripts/check-electron-runtime.mjs
//	check-desktop    ← scripts/check-desktop-contract.mjs
//	check-product    ← scripts/check-product-contract.mjs
//	check-gui        ← scripts/check-gui-contract.mjs
//
// 退出码契约:0=成功,1=契约失败,2=环境/IO 错误。
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/ouroboros/ouroboros/internal/desktop/buildmain"
	"github.com/ouroboros/ouroboros/internal/desktop/copymain"
	"github.com/ouroboros/ouroboros/internal/desktop/desktopcontract"
	"github.com/ouroboros/ouroboros/internal/desktop/devmgr"
	"github.com/ouroboros/ouroboros/internal/desktop/electronruntime"
	"github.com/ouroboros/ouroboros/internal/desktop/guicontract"
	"github.com/ouroboros/ouroboros/internal/desktop/installer"
	"github.com/ouroboros/ouroboros/internal/desktop/packager"
	"github.com/ouroboros/ouroboros/internal/desktop/productcontract"
	"github.com/ouroboros/ouroboros/internal/desktop/runtimesmoke"
)

// roots derives the repo/gui root pair relative to whichever flag or
// environment the caller supplies. Defaults: gui root is ./gui when the
// working directory is the repo root, else the working directory treated as
// the gui root itself when it contains src/renderer.
func roots(guiRoot, repoRoot string) (string, string, error) {
	if guiRoot != "" && repoRoot == "" {
		repoRoot = filepath.Dir(guiRoot)
	}
	if repoRoot != "" && guiRoot == "" {
		guiRoot = filepath.Join(repoRoot, "gui")
	}
	if guiRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", err
		}
		if st, err := os.Stat(filepath.Join(cwd, "src", "renderer")); err == nil && st.IsDir() {
			guiRoot, repoRoot = cwd, filepath.Dir(cwd)
		} else if st, err := os.Stat(filepath.Join(cwd, "gui", "src", "renderer")); err == nil && st.IsDir() {
			guiRoot, repoRoot = filepath.Join(cwd, "gui"), cwd
		} else {
			return "", "", fmt.Errorf("无法定位 gui 目录(请用 --gui-root 指定)")
		}
	}
	return guiRoot, repoRoot, nil
}

func main() {
	root := &cobra.Command{Use: "desktop", Short: "Ouroboros 桌面工具链(Go 版)"}
	root.SilenceUsage = true
	root.SilenceErrors = true

	var guiRoot, repoRoot string
	root.PersistentFlags().StringVar(&guiRoot, "gui-root", "", "gui/ 目录路径(默认自动探测)")
	root.PersistentFlags().StringVar(&repoRoot, "repo-root", "", "仓库根目录(默认自动探测)")

	// dev ──────────────────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use: "dev", Short: "启动 Electron + Vite 开发会话(原 dev-desktop.mjs)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			g, _, err := roots(guiRoot, repoRoot)
			if err != nil {
				return err
			}
			return devmgr.Run(devmgr.Options{GUIRoot: g})
		},
	})

	// build-main ────────────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use: "build-main", Short: "装配桌面 main 进程 bundle(原 build-desktop-main.mjs)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			g, _, err := roots(guiRoot, repoRoot)
			if err != nil {
				return err
			}
			return buildmain.Build(g, buildmain.Options{}, cmd.OutOrStdout())
		},
	})

	// copy-main ─────────────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use: "copy-main", Short: "复制 src/main 到 dist/main(原 copy-main.mjs)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			g, _, err := roots(guiRoot, repoRoot)
			if err != nil {
				return err
			}
			out, err := copymain.Copy(g)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	})

	// package ───────────────────────────────────────────────────────────
	pkgCmd := &cobra.Command{
		Use: "package", Short: "打包桌面应用(原 package-desktop.mjs)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			g, r, err := roots(guiRoot, repoRoot)
			if err != nil {
				return err
			}
			dry, _ := cmd.Flags().GetBool("dry-run")
			dir, _ := cmd.Flags().GetBool("dir")
			if code := packager.Run(cmd.Context(), packager.Options{GUIroot: g, RepoRoot: r, DryRun: dry, DirMode: dir}); code != 0 {
				return fmt.Errorf("打包失败(exit %d)", code)
			}
			return nil
		},
	}
	pkgCmd.Flags().Bool("dry-run", false, "只打印步骤列表")
	pkgCmd.Flags().Bool("dir", false, "完整打包(--dir 模式)")
	root.AddCommand(pkgCmd)

	// install ───────────────────────────────────────────────────────────
	instCmd := &cobra.Command{
		Use: "install", Short: "安装桌面应用(原 install-desktop.mjs,现已支持 Windows/macOS/Linux)",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, r, err := roots(guiRoot, repoRoot)
			if err != nil {
				return err
			}
			// installer.Main 沿用原脚本的 argv 约定解析 --dry-run。
			argv := args
			if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
				argv = append([]string{"--dry-run"}, argv...)
			}
			code := installer.Main(cmd.Context(), argv, installer.Options{GUIroot: g, RepoRoot: r}, cmd.OutOrStdout(), cmd.ErrOrStderr())
			if code != 0 {
				return fmt.Errorf("安装失败(exit %d)", code)
			}
			return nil
		},
	}
	instCmd.Flags().Bool("dry-run", false, "只打印安装计划")
	root.AddCommand(instCmd)

	// runtime-smoke ─────────────────────────────────────────────────────
	smokeCmd := &cobra.Command{
		Use: "runtime-smoke [quick|diagnostics|ai-lite]", Short: "打包运行时冒烟(原 run-desktop-runtime-smoke.mjs)",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, r, err := roots(guiRoot, repoRoot)
			if err != nil {
				return err
			}
			mode := ""
			if len(args) == 1 {
				mode = args[0]
			}
			if code := runtimesmoke.Execute(runtimesmoke.Options{RepoRoot: r, Mode: mode}); code != 0 {
				return fmt.Errorf("运行冒烟失败(exit %d)", code)
			}
			return nil
		},
	}
	root.AddCommand(smokeCmd)

	// check-gui ─────────────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use: "check-gui", Short: "GUI 契约检查(原 check-gui-contract.mjs)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			g, _, err := roots(guiRoot, repoRoot)
			if err != nil {
				return err
			}
			return guicontract.Run(cmd.Context(), guicontract.Options{GUIRoot: g})
		},
	})

	// check-desktop ─────────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use: "check-desktop", Short: "桌面契约检查(原 check-desktop-contract.mjs)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			g, r, err := roots(guiRoot, repoRoot)
			if err != nil {
				return err
			}
			if code := desktopcontract.Check(desktopcontract.Options{
				GUIRoot:  g,
				RepoRoot: r,
				Stderr:   cmd.ErrOrStderr(),
			}); code != 0 {
				return fmt.Errorf("桌面契约检查失败(exit %d)", code)
			}
			return nil
		},
	})

	// check-product ─────────────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use: "check-product", Short: "产品契约检查(原 check-product-contract.mjs)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			g, r, err := roots(guiRoot, repoRoot)
			if err != nil {
				return err
			}
			return productcontract.Run(cmd.Context(), productcontract.Options{GUIRoot: g, RepoRoot: r, Stderr: cmd.ErrOrStderr()})
		},
	})

	// check-electron-runtime ────────────────────────────────────────────
	root.AddCommand(&cobra.Command{
		Use: "check-electron-runtime", Short: "Electron 运行时契约(原 check-electron-runtime.mjs)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			g, _, err := roots(guiRoot, repoRoot)
			if err != nil {
				return err
			}
			return electronruntime.Run(context.Background(), electronruntime.Options{GUIRoot: g})
		},
	})

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
