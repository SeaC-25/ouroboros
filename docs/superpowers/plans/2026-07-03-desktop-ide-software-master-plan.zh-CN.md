# Ouroboros 桌面 IDE 软件化总计划

## 目标
把当前的 GUI 预览和 Go API 组合，推进成一个完整可用的桌面软件：Electron/React 负责软件前端，Go 负责软件后端，应用启动后能选择工作区、启动本地后端、读取真实工作区数据、执行真实工具、展示真实证据/回放/报告，并完成桌面打包。

前端必须全部使用中文可见文案。业务状态不能依赖 mock、fixture、演示 IP、演示漏洞、演示工具包数量或硬编码的“看起来像真的”数据。

## 核心原则

1. 软件优先，不是 Web UI 预览
   - `npm run preview` 只能作为开发检查手段。
   - 产品路径必须是桌面应用：Electron 主进程启动窗口、管理工作区、启动/停止 Go 后端 sidecar。
   - 后端端口、工作区路径、连接状态都由桌面壳管理并通过 preload bridge 提供给渲染进程。

2. 能力直接可用
   - 工具执行、AI 辅助、工作区文件、证据、回放、报告和设置能力都作为普通软件能力直接暴露。
   - 工作区里的文件都按普通 IDE 文件处理：列出、打开、编辑、搜索、索引、展示。
   - 执行控件使用直接的软件动作：运行、停止、重试、查看日志、复制命令、打开产物。

3. 不硬编码业务数据
   - 前端不保留 fixture 数据。
   - 后端不返回 mock 图谱、mock 节点、mock 工具链数量、mock run、mock 报告。
   - 没有真实数据时显示真实空状态，而不是伪造一个任务、漏洞或工具结果。

4. 中文前端统一入口
   - 可见 UI 文案集中在 `gui/src/app/zh-cn.ts` 或后续 `gui/src/i18n/zh-cn.ts`。
   - TSX 中不直接写英文标签。
   - 后端状态值可以是稳定英文枚举，但进入界面前必须映射成中文。

5. Go 后端是能力核心
   - Electron 不重写后端逻辑。
   - Go 继续负责 workspace、eventlog/replay、evidence、toolchain、provider、worker/runtime、report 和 execution。
   - GUI API 是 Go 能力的桌面 DTO 投影，不是另一个 demo server。

## 当前状态判断

已经完成：
- `gui/` 已经有 Electron + Vite + React 壳。
- 主界面已经改成中文 IDE 风格，并连接了当前 Go API。
- `gui/src/app/fixtures.ts` 已删除。
- 当前界面能从 `/api/workspace/info`、`/api/evidence/graph`、`/api/runs`、`/api/toolchain/health` 读取数据。

还没完成：
- Electron 主进程还没有 preload bridge，也没有管理 Go 后端 sidecar。
- Go API 的图谱、节点、工具链仍包含 mock-shaped 数据。
- 前端还在 fanout 多个旧 endpoint，没有统一的桌面 bootstrap DTO。
- 状态栏和部分类型里仍残留旧字段名，需要改成 backend/execution/workflow 这类真实软件状态。
- 还没有工作区选择、最近工作区、桌面打包、桌面 smoke 测试。

## 最终架构

```text
Electron 主进程
  - 创建桌面窗口
  - 选择/记住工作区
  - 为每个工作区启动 Go backend sidecar
  - 选择可用 localhost 端口
  - 等待 /api/health
  - 退出时停止后端进程
  - 通过 IPC 提供 backend 状态和 apiBaseURL

Electron preload
  - window.ouroboros.getAPIBaseURL()
  - window.ouroboros.backendStatus()
  - window.ouroboros.selectWorkspace()
  - window.ouroboros.openWorkspace(path)

React 渲染进程
  - 只调用 preload 暴露的桌面 API 和 Go GUI endpoint
  - 首屏请求 /api/gui/bootstrap
  - 通过 /api/events/stream 接收事件刷新
  - 所有可见文案走中文文案表

Go 后端
  - /api/health
  - /api/gui/bootstrap
  - /api/workspace/files
  - /api/evidence/graph
  - /api/runs
  - /api/replay/timeline
  - /api/reports
  - /api/toolchain/health
  - /api/execution/*
  - /api/events/stream
```

## 数据来源规则

| 界面区域 | 真实来源 | 空状态 |
|---|---|---|
| 工作区名称/路径 | Electron 选择的目录 + Go workspace projector | 未打开工作区 |
| 文件树 | 工作区目录扫描 | 空工作区 |
| 图谱 | evidence/replay/workspace 投影 | 暂无证据节点 |
| 节点详情 | evidence 节点属性和关联 artifact | 请选择节点 |
| 工具链 | 实际工具包 manifest、安装状态、可执行文件检查 | 未发现工具包 |
| 执行视图 | runtime/execution records、日志、退出码 | 暂无执行记录 |
| Run 列表 | `replay/events.db` / eventlog | 暂无运行历史 |
| 回放 | eventlog timeline | 暂无回放事件 |
| 报告 | `reports/*.md` 和报告投影 | 暂无报告 |
| 设置 | 本地配置、provider 配置、桌面设置 | 使用默认设置 |

## 实施阶段

### 阶段 1：产品契约和检查器归位

目标：先把方向固定到桌面软件、真实数据和中文界面，后续实现直接按这个合同推进。

交付：
- 更新 `PRODUCT.md`、`DESIGN.md`、GUI spec 和 planning 文件。
- 新增 `gui/scripts/check-product-contract.mjs`。
- 扩展 `gui/scripts/check-gui-contract.mjs`。

检查项：
- 文档不再把产品描述成 Web UI 或 TUI-first。
- 合约检查能发现 fixture、mock 字符串、英文 UI 标签和旧字段回流。

### 阶段 2：Go 后端提供真实 GUI DTO

目标：让前端不再拼多个旧 endpoint，也不消费 mock shape。

交付：
- `internal/api/dto.go`：定义 GUI DTO。
- `internal/api/projector.go`：从工作区、eventlog、evidence、toolchain、report、provider、execution 投影真实数据。
- `internal/api/server.go`：新增 `/api/gui/bootstrap`，并把旧 GUI endpoint 改成 projector-backed。
- `internal/api/*_test.go`：覆盖 bootstrap、graph、runs、toolchain、reports、empty workspace。

硬要求：
- 没有真实数据时返回空数组/空状态，不返回演示漏洞或演示 IP。
- 工具数量从真实 manifest 或实际探测结果来。
- 运行历史从 eventlog 来。
- 报告从真实 report 文件或报告投影来。

### 阶段 3：Electron 变成真正软件壳

目标：桌面应用自己管理后端，不再要求用户手动开一个 Web API 再打开预览。

交付：
- `gui/src/main/preload.js`：暴露桌面 API。
- `gui/src/main/backend.js`：启动、健康检查、停止 Go sidecar。
- `gui/src/main/workspace.js`：工作区选择、最近工作区、路径持久化。
- `gui/src/main/main.js`：接入 preload、IPC、sidecar 生命周期。
- `gui/scripts/check-desktop-contract.mjs`：检查 preload/sidecar/IPC 必备接口。

验收：
- 双击/`npm run electron` 能打开桌面窗口。
- 未选择工作区时显示中文工作区打开界面。
- 选择工作区后自动启动 Go 后端，并把 apiBaseURL 传给渲染进程。
- 应用退出时后端进程被清理。

### 阶段 4：前端完全切到桌面数据流

目标：React 不再依赖固定 `127.0.0.1:8080` 和旧 endpoint fanout。

交付：
- `gui/src/app/api.ts`：优先通过 `window.ouroboros.getAPIBaseURL()` 请求 `/api/gui/bootstrap`。
- `gui/src/app/types.ts`：对齐 Go GUI DTO，改成 backend/execution/workflow 等真实软件状态字段。
- `gui/src/app/OuroborosIDE.tsx`：渲染 bootstrap 数据、真实空状态、真实错误状态。
- `gui/src/app/zh-cn.ts`：补齐所有中文文案。

验收：
- 断开后端时显示中文离线状态。
- 空工作区显示中文空状态。
- 有真实 evidence/run/report/toolchain 数据时界面展示真实数据。
- TSX 中没有英文可见 UI 文案。

### 阶段 5：工作区 IDE 能力补齐

目标：让前端像软件前端，而不是只读 dashboard。

交付：
- 工作区文件树：真实目录、过滤系统文件、刷新。
- 文件打开/编辑：至少支持文本、Markdown、JSON、YAML、日志。
- 搜索：工作区文本搜索和结果定位。
- 最近文件/最近工作区。
- 设置页：provider、工具链路径、主题、字体、后端状态。

验收：
- 用户能从桌面应用打开一个工作区并浏览真实文件。
- 用户能查看和编辑普通项目文件。
- 搜索结果来自真实工作区内容。

### 阶段 6：执行、日志和事件流

目标：把工具执行从展示状态推进成真实可用功能。

交付：
- Go execution API：列出可执行动作、启动、停止、重试、读取日志。
- 事件流：执行状态变化通过 `/api/events/stream` 推送。
- 前端执行面板：命令、参数、stdout/stderr、退出码、耗时、重跑按钮。

原则：
- 操作按钮直接表达软件动作：运行、停止、重试、打开日志、复制命令。
- 每次执行都记录到事件/运行历史，便于回放和报告引用。

### 阶段 7：图谱、回放、报告真实化

目标：主要业务视图全部来自真实 artifact。

交付：
- 图谱：从 evidence/eventlog 投影节点和边。
- 节点检查器：展示真实属性、来源文件、关联执行、关联报告段落。
- 回放：从 eventlog 构建时间线和快照。
- 报告：读取真实 Markdown，支持目录、搜索、证据引用跳转。

验收：
- 没有证据时图谱为空，不造节点。
- 有事件时回放能按时间线展示。
- 报告页显示真实文件内容。

### 阶段 8：打包、安装和 smoke

目标：产物是完整桌面软件。

交付：
- 构建 Go backend binary。
- Electron packaged app 包含 backend binary。
- 开发模式和打包模式都能找到正确后端。
- `scripts/smoke_desktop_gui.sh`：创建临时工作区、启动桌面/后端、截图、检查中文 UI 和真实数据。

验收命令：

```bash
GOPROXY=https://goproxy.cn,direct go test ./internal/api ./cmd/ouro -count=1
npm --prefix gui run test:contract
npm --prefix gui run test:desktop-contract
npm --prefix gui run build
sh scripts/smoke_desktop_gui.sh
git diff --check -- PRODUCT.md DESIGN.md docs/superpowers/specs .planning gui internal/api cmd/ouro scripts Makefile
```

## 硬编码清理清单

必须清理：
- `SQL Injection`、`Port 80 (HTTP)`、`10.0.0.1` 这类演示图谱数据。
- 固定工具包数量，如 `42`、`37`、`29`。
- 前端翻译函数中专门翻译 mock label 的逻辑。
- `DEFAULT_API_BASE_URL = 'http://127.0.0.1:8080'` 作为产品路径依赖。
- 旧的限制/门控类字段作为产品状态字段。
- TSX 中英文可见文案。

允许保留：
- 后端内部稳定枚举值，例如 `ready`、`running`、`failed`，但 UI 必须映射中文。
- 开发 fallback，例如本地调试时使用环境变量指定 API 地址。
- 测试 fixture，但只能在 `_test.go` 或明确 test fixture 目录里，不能进入产品响应。

## 优先级

P0：必须先做
- 文档/合约修正。
- Go API 去 mock，新增 `/api/gui/bootstrap`。
- Electron preload + backend sidecar。
- 前端切到 bootstrap + 中文文案检查。

P1：软件可用性
- 工作区选择和最近工作区。
- 文件树、文件打开、文本编辑、搜索。
- 执行面板和事件流。
- 图谱/回放/报告真实 artifact 化。

P2：发行质量
- 打包后端 binary。
- 桌面 smoke。
- 多尺寸截图检查。
- 错误状态、加载骨架、离线重连。

## 立即下一步

从阶段 1 开始执行，但不要停在文档：阶段 1 完成后马上进入阶段 2 和阶段 3。真正的关键路径是后端真实 DTO 和 Electron sidecar，只有这两项完成，Ouroboros 才会从 Web UI 预览变成桌面软件。
