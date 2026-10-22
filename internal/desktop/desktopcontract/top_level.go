package desktopcontract

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ouroboros/ouroboros/internal/desktop/packager"
)

// checkTopLevel ports the static half of gui/scripts/check-desktop-contract.mjs
// (the assertion blocks preceding the dynamic module contracts, mjs lines
// 9-447 plus the env-gated legacy package smoke block). Message texts keep the
// original wording so failures stay greppable against the mjs.
func (r *runner) checkTopLevel() {
	r.checkRequiredFiles()
	r.checkElectron()
	r.checkMain()
	r.checkPreloadAndRenderer()
	r.checkBackend()
	r.checkWorkspaceState()
	r.checkPackager()
	r.checkPackageJSON()
	r.checkLegacyPackageSmoke()
}

func (r *runner) checkRequiredFiles() {
	for _, file := range []string{
		"src/main/main.js",
		"src/main/preload.js",
		"src/main/backend.js",
		"src/main/workspace.js",
		"src/main/desktop-state.js",
		"electron/main.js",
		"electron/memory-profile.js",
		"electron/backend.js",
		"electron/assets/ouroboros-icon.png",
	} {
		if !fileExists(joinPath(r.opts.GUIRoot, file)) {
			r.errf("missing desktop file: %s", file)
		}
	}
}

func (r *runner) checkElectron() {
	packagedMain := r.read("electron/main.js")
	r.requireAll("packaged Electron bootstrap missing low-memory runtime behavior: %s", packagedMain, []string{
		"OURO_DESKTOP_MEMORY_MODE", "app.relaunch", "app.exit(0)", "buildMemoryRelaunchArgs",
		"hasMemoryLaunchProfile", "appendSwitch('renderer-process-limit'", "appendSwitch('js-flags'",
		"import('./application-main.js')",
	})
	if strings.Contains(packagedMain, "NODE_OPTIONS") {
		r.errf("packaged Electron bootstrap must not use Electron-restricted NODE_OPTIONS")
	}

	memoryProfile := r.read("electron/memory-profile.js")
	r.requireAll("packaged Electron memory profile missing executable low-memory behavior: %s", memoryProfile, []string{
		"DEFAULT_PACKAGED_MEMORY_MODE = 'low'", "resolveMemoryProfile", "applyMemoryEnvironment",
		"buildMemoryRelaunchArgs", "hasMemoryLaunchProfile", "GOMEMLIMIT", "GOGC",
		"--max-old-space-size=", "--max-semi-space-size=",
	})
	if strings.Contains(memoryProfile, "NODE_OPTIONS") {
		r.errf("packaged Electron memory profile must not use Electron-restricted NODE_OPTIONS")
	}

	packagedBackend := r.read("electron/backend.js")
	r.requireAll("packaged backend wrapper missing sidecar permission/crash recovery behavior: %s", packagedBackend, []string{
		"ensurePackagedSidecarExecutable", "sidecar.recovery_scheduled", "sidecar.recovery_exhausted",
		"OURO_DESKTOP_SIDECAR_RESTART_LIMIT", "scheduleRecovery", "cancelRecovery",
		"normalizeRecordedExitCode",
	})
	if !mustPattern(`if \(this\.restartAttempts >= this\.restartLimit\) \{\s*this\.recoveryDisabled = true;`).MatchString(packagedBackend) {
		r.errf("packaged backend recovery must disable itself after the bounded restart budget is exhausted")
	}

	// 桌面工具链已 Go 化:构建/打包/冒烟的行为契约由以下 Go 包与其单测保证
	// (internal/desktop/{buildmain,packager,runtimesmoke}),这里只校验工具链
	// 入口与实现文件存在,避免契约与被删除的 .mjs 脚本共同失效。
	r.checkGoDesktopTooling()
}

// checkGoDesktopTooling asserts the Go desktop toolchain that replaced
// gui/scripts/*.mjs is present in the repository.
func (r *runner) checkGoDesktopTooling() {
	for _, file := range []string{
		"cmd/desktop/main.go",
		"internal/desktop/buildmain/buildmain.go",
		"internal/desktop/packager/packager.go",
		"internal/desktop/runtimesmoke/runtimesmoke.go",
		"internal/desktop/guicontract/guicontract.go",
	} {
		if !fileExists(joinPath(r.opts.RepoRoot, file)) {
			r.errf("missing Go desktop toolchain file: %s", file)
		}
	}
}

func (r *runner) checkMain() {
	main := r.read("src/main/main.js")
	r.requireAll("main process missing %s", main, []string{
		"Menu", "nativeImage", "shell", "showItemInFolder", "buildApplicationMenu",
		"nativeImage.createFromPath(path.join(__dirname, 'assets', 'ouroboros-icon.png'))",
		"window.setIcon(windowIcon)", "preload:", "backend:get-api-base-url", "backend:status",
		"backend:restart", "workspace:select", "workspace:open", "workspace:create",
		"desktop:recent-workspaces", "desktop:open-recent-workspace", "desktop:show-workspace-file",
		"desktop:window-state", "desktop:runtime-info", "window:minimize", "window:toggle-maximize",
		"window:close", "desktop:menu-action", "initializeWorkspace", "BackendManager",
		"WorkspaceManager", "DesktopStateManager", "runtimeInfo()",
	})
	if strings.Contains(main, "process.env.VITE_OURO_API_BASE_URL") {
		r.errf("native backend URL IPC must not fall back to an unverified Vite URL")
	}

	restartMenuEntry := ""
	if m := mustPattern(`(?s)label: '重启后端',.{0,420}`).FindString(main); m != "" {
		restartMenuEntry = m
	}
	if strings.Contains(restartMenuEntry, "sendMenuAction(window, 'backend:restart')") &&
		strings.Contains(restartMenuEntry, "backend.restart()") {
		r.errf("native restart menu must not dispatch a second renderer restart alongside backend.restart()")
	}

	r.requireAll("main process desktop menu must expose first-class runtime workbench mode: %s", main, []string{
		"menuModeItem(window, 'runtime'", "运行时", "click: () => sendMenuAction(window, `mode:${mode}`)",
	})
	r.requireAll("main process must stop packaged backend sidecar before quitting: %s", main, []string{
		"backendStopBeforeQuit", "event.preventDefault()", "await backend.stop()", "app.quit()",
		"backend.before_quit", "registerProcessDiagnostics()", "'SIGINT', 'SIGTERM'",
		"process.on(signal", "desktop.process_signal",
	})
	r.requireAll("main process must keep packaged desktop to one backend-owning instance: %s", main, []string{
		"requestSingleInstanceLock()", "'second-instance'", "desktop.second_instance",
		"focusMainWindow()", "sendBackendSnapshotToWindow(mainWindow, 'second-instance')",
	})
	r.requireAll("main process must replay backend status after packaged renderer startup: %s", main, []string{
		"wireRendererStartupSync(window)", "'did-finish-load'",
		"sendBackendSnapshotToWindow(window, 'workspace-auto-start')", "desktop.renderer_status_sync",
		"sendMenuAction(window, 'refresh')",
	})
	r.requireAll("main process must configure Electron runtime for desktop GPU compatibility: %s", main, []string{
		"configureElectronRuntime()", "disableHardwareAcceleration()", "appendSwitch('disable-gpu')",
		"appendSwitch('disable-gpu-compositing')", "appendSwitch('disable-gpu-sandbox')",
		"appendSwitch('disable-vulkan')", "appendSwitch('disable-webgl')", "appendSwitch('disable-3d-apis')",
		"WebGPU",
	})
}

func (r *runner) checkPreloadAndRenderer() {
	preload := r.read("src/main/preload.js")
	r.requireAll("preload bridge missing %s", preload, []string{
		"contextBridge.exposeInMainWorld", "getAPIBaseURL", "backendStatus", "restartBackend",
		"selectWorkspace", "openWorkspace", "createWorkspace", "recentWorkspaces",
		"openRecentWorkspace", "showWorkspaceFile", "windowState", "runtimeInfo", "minimizeWindow",
		"toggleMaximizeWindow", "closeWindow", "onMenuAction", "onBackendStatus",
	})
	if strings.Contains(preload, "import { contextBridge, ipcRenderer } from 'electron'") ||
		strings.Contains(preload, `import { contextBridge, ipcRenderer } from "electron"`) {
		r.errf("preload bridge must be packaged-runtime compatible CommonJS, not a top-level ESM import")
	}
	if !strings.Contains(preload, "require('electron')") && !strings.Contains(preload, `require("electron")`) {
		r.errf("preload bridge must require electron so packaged Electron can load the preload script")
	}

	renderer := r.read("src/app/OuroborosIDE.tsx")
	r.requireAll("renderer missing real window control: %s", renderer, []string{
		"minimizeDesktopWindow", "toggleMaximizeDesktopWindow", "closeDesktopWindow",
		"zhCN.windowControls.minimize", "zhCN.windowControls.maximize", "zhCN.windowControls.close",
		"onMinimizeWindow", "onToggleMaximizeWindow", "onCloseWindow",
	})
	r.requireAll("renderer missing workspace file reveal action: %s", renderer, []string{
		"showDesktopWorkspaceFile", "showOpenFileInDesktop", "zhCN.editor.showInFolder",
	})
	if strings.Contains(renderer, "<span />\n        <span />\n        <span />") {
		r.errf("renderer window controls must not be static spans")
	}

	rendererAPI := r.read("src/app/api.ts")
	r.requireAll("renderer API missing desktop workspace file reveal: %s", rendererAPI, []string{
		"showDesktopWorkspaceFile", "showWorkspaceFile?: (path: string) => Promise<unknown>",
	})

	zhCN := r.read("src/app/zh-cn.ts")
	r.requireAll("Chinese copy missing workspace file reveal label: %s", zhCN, []string{
		"showInFolder", "showingInFolder", "shownInFolder", "showInFolderFailed",
	})
}

func (r *runner) checkBackend() {
	backend := r.read("src/main/backend.js")
	main := r.read("src/main/main.js")
	r.requireAll("backend manager missing %s", backend, []string{
		"EventEmitter", "spawn(", "go", "serve", "init", "/api/health", "choosePort",
		"initializeWorkspace", "restart()", "status = 'crashed'", "lastError", "emitStatus",
		"runtimeInfo()", "backendBinaryPath", "backendBinaryExists", "lastExitCode",
		"diagnosticLogPath", "clearBackendConnection()", "lastStdout: this.lastStdout",
		"lastStderr: this.lastStderr", "normalizeExitCode", "recordProcessExit",
		"isCleanBackendExit", "expectedStop || code === 0", "backend.clean_exit",
		"startupHealthFailed", "rawCode", "未记录退出码", "后端进程非预期退出", "markExpectedStop",
		"backend.expected_stop", "'before-quit'", "'will-quit'", "BACKEND_GRACEFUL_TERMINATION_MS",
		"BACKEND_FORCE_TERMINATION_MS",
	})
	for _, forbidden := range []string{
		"workspace path is required",
		"backend command exited with",
		"unrecorded exit code",
		"unable to choose backend port",
		"backend health returned",
		"backend health timed out",
	} {
		if strings.Contains(backend, forbidden) {
			r.errf("native backend diagnostics must not expose internal English text: %s", forbidden)
		}
	}
	r.requireAll("desktop runtime PID state must be published atomically with process identity: %s", backend, []string{
		"processStartToken", "renameSync", "desktop-runtime-state.json.tmp",
	})
	for _, required := range []string{
		"backend.startDesktopRuntimeSession()", "backend.stopDesktopRuntimeSession",
		"'render-process-gone'", "'child-process-gone'", "desktop.render_process_gone",
		"desktop.child_process_gone", "desktopRuntimeStatePath", "lastDesktopRuntimeIssue",
		"desktop.previous_unclean_exit", "shutdownFromFatalError", "app.exit(1)", "'crashed'",
	} {
		if !strings.Contains(main, required) && !strings.Contains(backend, required) {
			r.errf("main/backend process must record desktop runtime and renderer crash diagnostics: %s", required)
		}
	}
	if n := len(mustPattern(`this\.clearBackendConnection\(\);`).FindAllString(backend, -1)); n < 6 {
		r.errf("backend manager must clear stale apiBaseURL on offline/crashed sidecar transitions")
	}

	processTreeCheck := ""
	if m := mustPattern(`(?s)function backendProcessTreeMayBeAlive\(child\) \{(.*?)\n\}`).FindStringSubmatch(backend); len(m) > 1 {
		processTreeCheck = m[1]
	}
	r.requireAll("backend process-tree liveness must avoid reusing an exited child PID: %s", processTreeCheck, []string{
		"child.exitCode === null", "child.signalCode === null", "backendProcessGroupMayBeAlive(child)",
	})
}

func (r *runner) checkWorkspaceState() {
	workspace := r.read("src/main/workspace.js")
	r.requireAll("workspace manager missing %s", workspace, []string{
		"recentWorkspaces", "rememberWorkspace", "createWorkspace", "选择或创建 Ouroboros 工作区",
	})
	body := ""
	if m := mustPattern(`(?s)async createWorkspace\(parentWindow\) \{(.*?)\n  async openWorkspace`).FindStringSubmatch(workspace); len(m) > 1 {
		body = m[1]
	}
	if body == "" || !mustPattern(`(?m)if \(result\.canceled \|\| result\.filePaths\.length === 0\) \{\s*return '';\s*\}`).MatchString(body) {
		r.errf("createWorkspace must return an empty selection when the dialog is canceled")
	}

	desktopState := r.read("src/main/desktop-state.js")
	r.requireAll("desktop state manager missing %s", desktopState, []string{
		"DesktopStateManager", "loadWindowState", "saveWindowState", "window-state.json",
	})
}

func (r *runner) checkPackager() {
	// 打包器已 Go 化:直接断言 Go 实现生成的 Linux 桌面入口内容(行为级),
	// 而不是匹配源码文本。原 mjs 的对应断言由 packager 单测与本检查共同覆盖。
	entry := packager.LinuxDesktopEntryFor(filepath.Join, "/tmp/ouroboros-package")
	r.requireAll("desktop package preparation missing %s", entry, []string{
		"[Desktop Entry]", "Name=Ouroboros", "StartupNotify=false", " %U",
		"ouroboros-icon.png",
	})
	installer := packager.LinuxDesktopInstallerScript(filepath.Join)
	r.requireAll("desktop package preparation missing %s", installer, []string{
		"dev.ouroboros.Ouroboros.desktop", `cat > "$DESKTOP_FILE"`, "update-desktop-database",
	})
	for _, smokePath := range []string{"scripts/smoke_desktop_package.sh", "scripts/smoke_desktop_package_runtime.sh"} {
		smoke := r.readRP(smokePath)
		if strings.Contains(smoke, "git rev-parse --path-format=absolute --git-common-dir") {
			r.errf("%s must not fall back to a package from another worktree", smokePath)
		}
	}
}

func (r *runner) checkPackageJSON() {
	packageJSON := r.read("package.json")
	r.requireAll("package.json missing desktop package script: %s", packageJSON, []string{
		"package:desktop:dir", "package:desktop:dir:dry-run",
	})
	// 注意:package.json 的 runtime-smoke 脚本断言在原始 mjs 中位于
	// OUROBOROS_DESKTOP_CONTRACT=2 门控块内(见 checkLegacyPackageSmoke),
	// 因此这里不做断言,以免在默认合同下比原脚本更严格。
}

// checkLegacyPackageSmoke ports the OUROBOROS_DESKTOP_CONTRACT=2 gated block:
// the older Linux package-smoke assertions stay opt-in so the canonical
// Windows/renderer contract does not depend on another branch.
func (r *runner) checkLegacyPackageSmoke() {
	if os.Getenv("OUROBOROS_DESKTOP_CONTRACT") != "2" {
		return
	}

	packageSmoke := r.readRP("scripts/smoke_desktop_package.sh")
	r.requireAll("desktop package sidecar/layout smoke must identify itself clearly: %s", packageSmoke, []string{
		"sidecar/layout", "BACKEND_BINARY", "packaged backend sidecar", "dist/main/memory-profile.js",
		"packaged memory profile", "DEFAULT_PACKAGED_MEMORY_MODE = 'low'", "backendStopBeforeQuit",
		"await backend.stop()", "clearBackendConnection()", "DESKTOP_ENTRY", "desktop-file-validate",
		"StartupNotify=false", "warp-oss", "package-local Linux desktop entry", "%k %U", "setsid",
		"API_PGID", "process_group_is_running", "api_runtime_is_running",
	})
	r.requireAll("desktop package sidecar/layout smoke must verify Linux desktop entry install in an isolated home: %s", packageSmoke, []string{
		"DESKTOP_ENTRY_INSTALLED", `HOME="$SMOKE_ROOT/home"`, "installed Linux desktop entry",
		"Ouroboros desktop entry installed", "COPIED_PACKAGE_DIR",
		"copied package-local Linux desktop entry", "copied installed Linux desktop entry",
	})
	launchIdx := strings.Index(packageSmoke, "API_PID=$!")
	readyIdx := indexOfFrom(packageSmoke, `wait_http "http://127.0.0.1:$API_PORT/api/health"`, launchIdx)
	groupIdx := indexOfFrom(packageSmoke, `api_pgid="$(ps -o pgid=`, launchIdx)
	if launchIdx < 0 || readyIdx < launchIdx || groupIdx < readyIdx {
		r.errf("desktop package smoke must sample the packaged backend process group after sidecar readiness")
	}

	runtimeSmoke := r.readRP("scripts/smoke_desktop_package_runtime.sh")
	r.requireAll("desktop package runtime smoke missing real Electron coverage: %s", runtimeSmoke, []string{
		"APP_BINARY", "--remote-debugging-port", "window.ouroboros", "getAPIBaseURL",
		"backendStatus", "OUROBOROS_WORKSPACE", "XDG_CONFIG_HOME", "xvfb-run",
	})
	r.requireAll("desktop package runtime smoke must verify packaged GUI mission launch: %s", runtimeSmoke, []string{
		"async function fillMissionLaunch(name, goal, command)",
		"async function clickMissionLaunchAction(label)",
		"async function waitForExecutionByCommand(apiBaseURL, commandNeedle)",
		"async function waitForExecutionReferences(apiBaseURL, executionID)",
		"async function clickExecutionDetailAction(label)",
		"async function waitForGraphDetail(nodeID, executionID)",
		"async function waitForReplayDetail(eventID, executionID)",
		"await fillMissionLaunch('打包运行时表单任务'",
		"await clickMissionLaunchAction('启动任务')",
		"const runtimeMissionReferences = await waitForExecutionReferences(last.apiBaseURL, runtimeMissionExecution.id)",
		"await clickExecutionDetailAction('定位图谱')",
		"await waitForGraphDetail(runtimeMissionReferences.node.id, runtimeMissionExecution.id)",
		"await clickExecutionDetailAction('定位回放')",
		"await waitForReplayDetail(runtimeMissionReferences.event.id, runtimeMissionExecution.id)",
		`assert_contains "$WORKSPACE/mission.yaml" "打包运行时表单任务"`,
		`grep -R -F -q -- "packaged-runtime-form-log" "$WORKSPACE/executions"`,
		`assert_contains "$RUNTIME_OUT" "runtimeMissionReferences"`,
		`assert_contains "$RUNTIME_OUT" "节点详情"`,
		`assert_contains "$RUNTIME_OUT" "回放详情"`,
		`assert_contains "$RUNTIME_OUT" "关联执行文件"`,
		"await window.ouroboros.showWorkspaceFile('mission.yaml')",
		`assert_contains "$RUNTIME_OUT" "shownWorkspaceFile"`,
	})
	r.requireAll("desktop package runtime smoke must verify packaged prompt registry runtime wiring: %s", runtimeSmoke, []string{
		"AI_PROMPT_MARKER='打包运行时提示词注册表烟测标记'",
		"async function waitForSettingsSection(section, needle)",
		"async function savePromptRegistryThroughGUI(marker)",
		"text.includes('提示词工程')",
		"await waitForSettingsSection('prompts', '提示词注册表')",
		"includes('保存提示词')",
		"const savedRegistry = await waitForPromptRegistryYAML([",
		"await waitForSettingsSection('prompts', marker)",
		"只能依据真实打包运行时工作区上下文推进。",
		"必须引用真实文件、执行、证据和报告。",
		"await clickMode('设置')",
		"await savePromptRegistryThroughGUI(aiPromptMarker)",
		`assert_contains "$WORKSPACE/prompts/registry.yaml" "$AI_PROMPT_MARKER"`,
		`assert_contains "$WORKSPACE/prompts/registry.yaml" "ai.workbench.system"`,
		`assert_contains "$PROVIDER_LOG" "$AI_PROMPT_MARKER"`,
		`assert_contains "$DOM_OUT" "$AI_PROMPT_MARKER"`,
	})
	for _, injected := range []string{
		"window.ouroboros =",
		"Object.defineProperty(window, 'ouroboros'",
		`Object.defineProperty(window, "ouroboros"`,
	} {
		if strings.Contains(runtimeSmoke, injected) {
			r.errf("desktop package runtime smoke must use the real preload bridge, not inject window.ouroboros")
			break
		}
	}
	r.requireAll("desktop package runtime smoke must verify packaged quit lifecycle and sidecar shutdown: %s", runtimeSmoke, []string{
		"wait_for_app_exit", "wait_for_backend_shutdown", "backend.before_quit", "diagnosticLogPath",
		"runtime quit lifecycle", "assert_clean_backend_exit_event", "record.event === 'backend.exit'",
		"latest.expectedStop !== true",
		"latest.code === 0 || latest.signal === 'SIGTERM' || latest.signal === 'SIGINT'", "'rawCode'",
	})
	r.requireAll("desktop package runtime smoke must support low-memory real packaged quick verification: %s", runtimeSmoke, []string{
		"OURO_DESKTOP_PACKAGE_RUNTIME_SMOKE_MODE", "quick", "diagnostics", "desktop-runtime-state.json",
		"packaged-runtime-diagnostics-previous", "desktop.previous_unclean_exit", "diagnosticsReady",
		"runtimeInfo()", "backendStatus()", "/api/gui/bootstrap",
	})
	r.requireAll("desktop package runtime smoke must launch and clean up the packaged low-memory profile cross-platform: %s", runtimeSmoke, []string{
		"--ouro-desktop-memory-profile=low", "--max-old-space-size=256 --max-semi-space-size=8",
		"--renderer-process-limit=1", `MSYS2_ARG_CONV_EXCL='*' taskkill.exe /PID "$APP_PID" /T /F`,
	})
	r.requireAll("desktop package quick/ai-lite runtime smoke must keep first-class runtime workbench markers: %s", runtimeSmoke, []string{
		"data-runtime-workbench", "data-runtime-summary", "data-runtime-backend-status",
		"data-runtime-package-status",
	})
	r.requireAll("desktop package runtime smoke must support low-memory real AI chat verification: %s", runtimeSmoke, []string{
		"ai-lite", "/api/ai/chat", `AI_CONVERSATION="$WORKSPACE/runtime/ai/conversation.jsonl"`,
		`PROVIDER_LOG="$SMOKE_ROOT/provider-requests.jsonl"`, "$WORKSPACE/runtime/model-usage",
	})
	r.requireAll("desktop package ai-lite smoke must verify the AI collaboration topology panel without full smoke: %s", runtimeSmoke, []string{
		"waitForAICollaborationTopology",
		`document.querySelector('[data-ai-collaboration-topology="true"]')`,
		"data-ai-topology-action", "topologySnapshot", "panelReady: true", "copyReady",
		`"topologySnapshot"`, `"panelReady": true`, `"copyReady": true`,
		`assert_contains "$DOM_OUT" "协作拓扑"`, `assert_contains "$DOM_OUT" "拓扑节点"`,
		`assert_contains "$DOM_OUT" "拓扑关系"`, `assert_contains "$DOM_OUT" "沉淀评测"`,
	})
	r.requireAll("desktop package ai-lite smoke must verify real report Markdown preview without full smoke: %s", runtimeSmoke, []string{
		"reports/runtime.md", "waitForReportPreview", "clickReportWorkbenchMode",
		`document.querySelector('[data-report-preview-panel="true"]')`,
		`document.querySelector('[data-report-preview-content="true"]')`,
		"data-report-preview-heading", "data-report-source-editor", `"reportPreview"`,
		`"hasSourceEditor": true`, `assert_contains "$DOM_OUT" "报告预览"`,
		`assert_contains "$DOM_OUT" "编辑源码"`, `assert_contains "$DOM_OUT" "打包运行时报告"`,
	})
	r.requireAll("desktop package ai-lite smoke must verify real Harness environment snapshot evidence: %s", runtimeSmoke, []string{
		"harness.cases.from_environment_snapshot", "ai-lite-environment-baseline",
		"harness/environment-snapshots/ai-lite-environment-baseline/snapshot.json",
		"runDirectHarnessEnvironmentSnapshotAction", "waitForHarnessEnvironmentSnapshotEvidence",
		`"environmentSnapshot"`, "environment_snapshot", "providerSnapshot", "toolchainSnapshot",
		"runtime-pack",
	})
	packageJSON := r.read("package.json")
	r.requireAll("package.json missing desktop runtime smoke script: %s", packageJSON, []string{
		"smoke:desktop-package:runtime", "smoke:desktop-package:runtime:quick",
		"smoke:desktop-package:runtime:diagnostics", "smoke:desktop-package:runtime:ai-lite",
		"run-desktop-runtime-smoke.mjs", "OURO_DESKTOP_PACKAGE_RUNTIME_SMOKE_MODE",
		"smoke_desktop_package_runtime.sh",
	})
}
