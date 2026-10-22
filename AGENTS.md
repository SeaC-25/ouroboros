# Repository Guidelines(仓库指南)

## 项目概述

Ouroboros 是一个以证据为闸门(evidence-gated)的 AI 渗透测试 IDE:核心为 **Go 安全编排后端**(`ouro` CLI + API 服务),配以 **React/Electron 桌面 GUI**。任务运行在文件化工作区中(事件、证据、工件),工具链采用摘要固定(digest-pinned)的托管工具包,并具备显式的安全授权闸门。默认离线/干跑(dry-run),真实执行需要显式授权。

## 架构与数据流

```
cmd/ouro(Cobra CLI)
  └─ internal/app        领域服务 + 每个工具一个解析器(nmap/amass/ffuf/…_parser.go)
       ├─ internal/compiler    目标(goal)→ 语义动作 → 工具计划
       ├─ internal/toolchain   带摘要校验的 YAML 工具包清单
       ├─ internal/swarm       多 agent 编排(worker pool、dispatcher)
       ├─ internal/frontier    预算分配、覆盖度、评分
       └─ internal/evidence + internal/artifact   发现物投影、工件库
            └─ internal/eventlog  SQLite 事件日志(事实来源,支持回放)
internal/api              GUI 消费的 HTTP/WebSocket 服务
internal/tui              Bubble Tea 终端 UI
internal/{domain,patterns,goal,sisyphus,modelruntime}  领域包(webapi/infra/ctf)、
                          路径规划模式、目标图、重试引擎、LLM 运行时
gui/                      Electron(gui/src/main 主进程)→ window.ouroboros 桥
                          → renderer React 应用(gui/src/app),由 internal/api 支撑
```

数据流:mission YAML → compiler/toolchain 计划 → 工具执行(原始输出经 `internal/app/*_parser.go` 解析)→ eventlog(SQLite)→ evidence/artifact 查询 → report/TUI/GUI 投影。所有状态都以工作区文件为基准;replay 离线读取事件日志。

## 关键目录

| 路径 | 作用 |
|---|---|
| `cmd/ouro/` | Cobra CLI 入口(`main.go`、`serve.go`);尽量薄,逻辑在 `internal/` |
| `internal/app/` | 核心服务 + 每个安全工具一个 `*_parser.go` 解析器 |
| `internal/api/` | GUI 调用的 HTTP API DTO/处理器(agent 执行、mission、报告) |
| `toolchains/{web-api-pack,infra-pack,ctf-pack,managed-bundles}/` | YAML 工具清单包 |
| `configs/` | `providers.example.yaml`、`patterns/` 策略定义 |
| `gui/src/renderer/` | 渲染进程入口:`main.tsx` → `App.tsx` → `OuroborosIDE.tsx` |
| `gui/src/app/` | IDE 外壳(`OuroborosIDE.tsx` 约 2.5 万行单体)、`api.ts`、`zh-cn.ts` |
| `gui/src/main/` `gui/electron/` | Electron 主进程(BackendManager 拉起 Go 后端、IPC、preload 桥) |
| `cmd/desktop/` | 桌面工具链 Go 入口(dev/build-main/package/install/runtime-smoke/check-*,原 `gui/scripts/*.mjs`) |
| `internal/desktop/` | 桌面工具链实现:copymain、buildmain、devmgr、packager、installer、runtimesmoke、guicontract、desktopcontract、productcontract、electronruntime |
| `scripts/smoke_*.sh` | 分层验收冒烟(后端 + GUI + 打包运行时) |
| `test/fixtures/` | 小型确定性测试夹具 |
| `docs/` | 架构书(`Ouroboros_平台架构设计书.md`)、`goal.md`;README/`development.md` 达 100–250KB 且常过期——以代码为准 |

## 开发命令

后端(Go 1.26,模块 `github.com/ouroboros/ouroboros`,默认 `GOPROXY=https://goproxy.cn,direct`):

```sh
make test                 # go test ./...
make build                # → bin/ouro(scripts/build.sh)
make doctor / doctor-strict   # 初始化并校验临时工作区(strict 附加安全授权检查)
make smoke-ide            # 后端冒烟伞(direct-use + ai-provider)
make smoke-desktop-gui    # 真实 API + Vite + 无头 Chromium CDP 冒烟
make release              # 在 dist/ 生成版本化归档
go run ./cmd/ouro <init|doctor|run|replay|chat|serve> ...
```

GUI(Node + npm、Electron 33、Vite 6、React 18、Tailwind 4):

```sh
cd gui && npm install
npm run dev                         # Electron + 热更新渲染进程 → go run ../cmd/desktop dev
npm run typecheck                   # tsc --noEmit
npm run test:contract / test:product-contract / test:desktop-contract   # 契约检查(Go 实现)
npm run smoke:desktop-package:runtime:quick     # 可运行打包应用的冒烟
```

桌面工具链已全部 Go 化(`cmd/desktop`,子命令 `dev|build-main|copy-main|package|install|runtime-smoke|check-gui|check-desktop|check-product|check-electron-runtime`),
`gui/scripts/*.mjs` 已删除;`npm run *` 只是对 `go run ../cmd/desktop ...` 的薄封装。需要脱离 npm 直接跑时用:

```sh
go run ./cmd/desktop package --dir --dry-run
go run ./cmd/desktop check-desktop
```

Vite 浏览器预览仅限开发用途;产品运行时是打包后的 Electron。一键启动见 `start.sh`(后端 :8080 + GUI)。

## 代码约定与常见模式

- **Go**:`gofmt`,短小写包名,错误必须包装 `fmt.Errorf("上下文: %w", err)`(上下文信息中英混用——面向用户的用中文);每个工作区文件访问都必须做路径/symlink/范围显式校验;不使用 mock——用真实工作区与清单。
- **测试**:`*_test.go` 与实现同目录,命名 `Test<对象><场景>`,`t.TempDir()` 夹具,shell 脚本测试用 `skipIfWindows`;解析器测试需覆盖合法/畸形/越范围/密钥脱敏四类用例。
- **TS/React**:严格 `tsc`、2 空格缩进、单引号、`import type`、路径别名 `@/* → src/*`、组件 PascalCase;状态用 Zustand、请求用 react-query;仓库没有 eslint/prettier,对齐既有风格即可。
- **UI 文案**:所有用户可见字符串集中在 `gui/src/app/zh-cn.ts`(`zhCN`,界面为纯中文)——禁止在组件里硬编码字符串。
- **样式**:Tailwind 4 + CSS 变量;视觉体系在 `gui/src/styles/ouroboros-ide.css`——黑色底色、`--brand-red: #ef3f49` 强调色,绿色仅用于语义化成功状态。
- **Electron 边界**:特权能力(系统对话框、窗口控制、后端拉起)只放在 `gui/src/main` + `preload.js`;渲染进程一律通过 `window.ouroboros` 桥访问,绝不回退到硬编码 localhost 地址。

## 重要文件

- 入口:`cmd/ouro/main.go`、`cmd/ouro/serve.go`;`gui/index.html` → `gui/src/renderer/main.tsx` → `gui/src/app/OuroborosIDE.tsx`
- 配置:`go.mod`、`Makefile`、`configs/providers.example.yaml`、`gui/package.json`、`gui/tsconfig.json`、`gui/tailwind.config.js`
- 桥层:`gui/src/main/preload.js` + `gui/src/app/api.ts`(`window.ouroboros`)
- 工具链清单:`toolchains/*/manifest.yaml`、`internal/toolchain/manifest.go`

## 运行时与工具链要求

- Go 1.26(`gofmt`,测试只用标准库 `testing`——不用 testify);依赖:Cobra、gorilla/websocket、modernc.org/sqlite、Bubble Tea。
- `gui/` 用 Node.js + **npm**(package-lock.json;不用 bun/yarn/pnpm)。
- 冒烟脚本为 bash,需 POSIX shell(Windows 用 Git Bash);部分冒烟在 Windows 下 `skipIfWindows`。
- 托管工具包按摘要固定;`model/` 与 `test-workspace/` 是产物/状态目录——不要手工改动。

## 测试与质量保障

- Go:`go test ./...`(`make test`);迭代时只跑单个包,提交前跑全量。无覆盖率门槛。
- **测试必须离线可跑**:`internal/app` 的 `TestMain` 默认设 `OURO_PROVIDER_PREFLIGHT=off`(跳过对 provider 的
  远程 health check),因此第三方中继域名解析失败不会让不相关测试变红;真正验证 preflight 的用例用
  `t.Setenv(ProviderPreflightEnvVar, "on")` 显式开启,且一律打 `httptest` 本地服务。生产默认仍是 `on`(fail-fast)。
- GUI:契约脚本(`npm run test:contract`、`test:desktop-contract`),再用打包运行时冒烟(`smoke:desktop-package:runtime:quick`)做真实验证。
- 产品铁律(见 `docs/goal.md`):**不允许 mock/假数据**——`scripts/check_no_gui_mock_literals.sh` 会拒绝 `internal/api` 与 `gui/src/app` 中出现 mock 字面量;GUI 所有状态必须来自真实工作区。
- 完整验收链:`make test → doctor-strict → build → smoke-* → audit-completion → release`。

## 测试脚本与工具链现状(2026-09 修复记录)

已实测通过:`go build ./...`、`go vet`、全量 `go test ./...`、`doctor`、GUI `typecheck`、
`check-gui` / `check-desktop` / `check-product` / `check-electron-runtime` 四项契约检查。

本轮修复(全部已落地并被测试覆盖):

- **PATH 覆盖(P0)**:`scripts/smoke_*.sh` 不再强制清空 PATH;仅当显式设置 `OURO_TRUSTED_PATH` 时才启用严格路径。
- **陈旧二进制(P1)**:新增 `scripts/smoke_fresh_bin.sh`,`ensure_fresh_ouro_bin` 在 `bin/ouro` 缺失或源码更新时先构建;
  9 个 smoke 已接入,`OURO_SMOKE_SKIP_BUILD=1` 可跳过。
- **进程/端口泄漏(P1)**:新增 `scripts/lib_smoke_common.sh`(`cleanup_tree`/`wait_http`/`assert_port_free`);
  GUI/打包冒烟改为直接跑构建产物并按进程树回收,端口启动前预检,默认等待上限 120s。
- **mock 闸门静默失效(P1)**:`check_no_gui_mock_literals.sh` 目录缺失时报错退出(exit 2),不再被 `2>/dev/null` 吞掉。
- **工具链 Go 化**:`gui/scripts/*.mjs` 全部删除,改由 `cmd/desktop` + `internal/desktop/*` 实现;
  仅剩的 JS 是各契约包内嵌的 node 驱动(`driver.mjs`、`drivers/*.mjs`),由 Go 编排、行为逐条对照验证。
- **带日期硬编码路径(P2)**:产品契约的 master plan 改为按目录最新 .md 解析,缺失时明确报错。
- **默认 provider 语义**:`smoke_release_managed.sh` 不再硬编码 `model:offline-model`,改为从工作区 `providers.yaml`
  的第一个 enabled provider 推导期望模型(默认 provider 保持 `ouro init` 现状)。

仍待处理:`smoke_desktop_gui.sh` 的「工具动作日志」断言在 Windows 上失败(后端 `sh -lc` 工具执行路径不适配 win32);
`gui/scripts` 目录已空,后续新增桌面工具请直接落到 `internal/desktop/` 并挂到 `cmd/desktop`。

## 工具包 Go 化(26/26 全覆盖 + 检测/认证能力升级)

路线图见 `docs/toolchain-go-roadmap.md`:25 个外部工具 + 新增 hydra 等价共 26 个工具,
20 个 operation 全部具备 `internal/nativetools` 原生 handler,`native_go` adapter 与
`managed_binary` 逐 operation 同构(同 operation、parser 与依赖声明,仅执行方式不同)。

检测/认证能力(2026-10 升级):
- **nuclei 通用模板引擎**(`template_engine.go`):完整 matchers(status/word/regex/binary/
  size/DSL-lite/negative)与 extractors(regex/kval/json/xpath),多请求变量流转,
  YAML 模板解析,`OURO_TEMPLATES_DIR` 外部模板目录加载;内置 13 个高信号模板。
- **sqlmap 五技术检测**:error/boolean/time-based/UNION 列数/stacked,GET/POST/Cookie
  三 place,参数自动发现,19 签名 DBMS 指纹;不含数据提取/提权。
- **dalfox**:参数自动发现(表单/链接)+ DOM sink 反射启发。
- **auth_test(hydra 等价,新 operation)**:HTTP Basic/Form 双模式、表单字段发现、
  哨兵基线差分、锁定检测,字典与阈值走 env(`OURO_AUTH_USERWORDS`/`OURO_AUTH_PASSWORDS`/
  `OURO_AUTH_LOCKOUT_THRESHOLD`)。

新增 parser:`exiftool_json`、`gitleaks_json`、`testssl_json`、`sslscan_xml`、
`trufflehog_json`、`binwalk_text`、`semgrep_sarif`、`apktool_text`、`jadx_metadata_json`、
`auth_test_jsonl`,填补"有契约无实现"的证据落库缺口。

**开关**:设置 `OURO_NATIVE_TOOLS=httpx,ffuf,gitleaks`(或 `all`)才把对应工具切到 `native_go`;
未设置时一律走 `managed_binary`——默认不改变既有运行行为,保证可复现。执行路径在
`internal/app/native_tool_execution.go`,复用 `requireCommandScopeTarget`、exclusion 与 worker 闸门;
scope 不满足时降级到 offline runner(与"外部二进制未安装"同语义)。

**环境变量优先(硬编码禁令)**:任何外部可观测的运行参数(上游端点、结果上限)一律走项目环境变量,
禁止硬编码。现有变量(入口 `internal/nativetools/env.go`,调用时求值,不缓存):
`OURO_CT_ENDPOINT`(crt.sh 证书透明源,subfinder/amass 共用,格式含 `%%25.%s`)、
`OURO_CDX_ENDPOINT`(Internet Archive CDX,gau/waybackurls)、`OURO_ARCHIVE_LIMIT`(归档结果上限,默认 2000)、
`OURO_CRAWL_LIMIT`(爬取路由上限,默认 256)。新增 handler 的可配置参数必须走同一入口;
User-Agent、协议 scheme、内嵌规则/模板属工具身份或数据本体,不算硬编码。

约定:原生 handler 必须产出与被替代外部工具**字节兼容**的输出(parser 与事件链零改动),不得重写 scope/授权判定。

## 安全与提交规范

- 禁止提交 API 密钥、凭据、真实目标数据、生成的证据、二进制、日志;Provider 配置走 `configs/providers.example.yaml` + 环境变量。
- 禁止削弱安全闸门(`--require-safety-authorization`、路径/范围/摘要校验),不允许为了让冒烟通过而放宽检查。
- 提交信息遵循 Conventional Commits,动词开头带 scope:`feat(api): …`、`fix(gui): …`、`test(gui): …`。PR 需附验证命令与结果,UI 变更附截图。
