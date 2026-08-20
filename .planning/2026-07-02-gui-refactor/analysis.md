# Ouroboros GUI 全面分析报告

**生成时间**: 2026-07-02  
**分析范围**: UI设计稿、当前实现、后端数据层、前后端集成

---

## 一、设计稿分析（ui-designs/）

### 1.1 主界面布局 (main_ui.png)

**设计稿展示的完整界面**：
- **左侧边栏（80px）**：垂直导航，包含品牌标识 + 6个主导航按钮 + 设置按钮
  - Attack Graph（攻击图，当前激活）
  - Targets（目标管理）
  - Packs（工具包管理）
  - Evidence（证据库）
  - Runs（运行历史）
  - Safety（安全检查）
  
- **顶部工具栏（60px）**：
  - 左侧：工作区标题 + 状态描述
  - 右侧：搜索、重置视图、过滤器、保存、Run按钮（主操作按钮）
  
- **主工作区**：
  - 攻击图可视化画布
  - 节点卡片（带图标、标题、状态描述）
  - 连接线（普通/高亮两种状态）
  - 状态指示器（右下角浮动）

- **配色方案**：
  - 背景：`#0a0a0a`（纯黑）
  - 面板：`#111113`、`#17171a`（深灰层次）
  - 边框：`#27272a`（常规）、`#3f3f46`（强调）
  - 文本：`#f4f4f5`（主要）、`#a1a1aa`（次要）、`#71717a`（弱化）
  - 强调色：`#2dd4bf`（青绿色，主要）、`#f97316`（橙色，风险）、`#fb7185`（粉红色，危险）

### 1.2 命令面板 (command_palette.png)

**快捷命令系统**：
- 模态覆盖层设计
- 搜索框 + 命令列表
- 命令分类：Recent、Workspace、Navigation、Tools、Settings
- 每个命令带图标、名称、快捷键提示
- 支持模糊搜索和键盘导航

### 1.3 节点检查器 (node_inspector.png)

**右侧面板详情视图**：
- 节点类型标识
- 节点元数据（ID、创建时间、状态）
- 关联边信息（incoming/outgoing）
- 证据引用列表
- 操作按钮（验证、标记、导出）
- Tab切换：Overview、Evidence、History、Actions

### 1.4 工具执行视图 (tool_execution.png)

**工具运行监控界面**：
- 实时日志流显示
- 工具状态指示器（运行中/成功/失败）
- 进度条或动画
- 输出过滤和搜索
- 停止/暂停控制按钮

### 1.5 报告视图 (report_view.png)

**报告生成和预览**：
- Markdown/HTML 预览
- 章节导航树
- 证据卡片嵌入
- 导出选项（PDF、HTML、Markdown）
- 实时编辑预览

### 1.6 设置面板 (settings_panel.png)

**配置管理界面**：
- 分类设置：General、Providers、Toolchain、Safety、Advanced
- 表单控件：输入框、选择器、开关、滑块
- 验证反馈
- 保存/重置按钮

### 1.7 启动渗透测试模态框 (start_pentest_modal.png)

**任务启动引导**：
- 目标输入（URL/IP/CIDR）
- Scope定义
- Pack选择（Web API / Infra / CTF）
- 安全选项确认
- 预检查结果展示

### 1.8 空状态 (empty_state.png)

**初始化引导界面**：
- 空状态插画
- 引导文案
- 快速开始按钮
- 示例项目链接

### 1.9 预检查 (preflight_check.png)

**启动前验证界面**：
- Toolchain健康检查
- Provider配置验证
- 工作区完整性检查
- 警告和错误汇总

### 1.10 回放视图 (replay_view.png)

**事件重放界面**：
- 时间轴控制
- 事件列表（按类型分层）
- 播放/暂停/步进控制
- 速度调节
- 书签和注释

---

## 二、当前实现分析（gui/src/）

### 2.1 技术栈

```json
{
  "runtime": "Electron 33.4.11",
  "framework": "React 18.3.1",
  "bundler": "Vite 6.0.3",
  "icons": "lucide-react 0.468.0",
  "language": "TypeScript 5.7.2"
}
```

### 2.2 文件结构

```
gui/
├── src/
│   ├── main/
│   │   └── main.js         # Electron主进程（80行）
│   ├── renderer/
│   │   ├── main.tsx        # React入口
│   │   └── App.tsx         # 主应用组件（377行，含CSS-in-JS）
│   └── components/
│       ├── Sidebar.tsx     # 侧边栏（49行）
│       ├── Toolbar.tsx     # 工具栏（31行）
│       └── AttackGraph.tsx # 攻击图（95行）
├── dist/                   # 构建输出
├── package.json
├── vite.config.ts
└── index.html
```

### 2.3 组件实现状态

#### Sidebar.tsx
**已实现**：
- 品牌标识"O"
- 6个导航按钮（硬编码图标）
- 设置按钮

**缺失**：
- 按钮点击事件（无路由切换）
- 活跃状态管理（只有第一个按钮写死为active）
- 工具提示（虽有title但无悬停样式）
- 响应式布局

#### Toolbar.tsx
**已实现**：
- 工作区标题显示
- 5个工具按钮图标
- Run主按钮

**缺失**：
- 所有按钮功能（无事件处理）
- 动态标题（写死为"Ouroboros Workspace"）
- 搜索功能
- 过滤器功能
- 保存功能

#### AttackGraph.tsx
**已实现**：
- 6个硬编码节点（固定位置）
- 6条SVG连线
- 节点卡片样式
- 状态指示器

**缺失**：
- 动态数据加载
- 节点交互（点击、拖拽）
- 图布局算法
- 缩放/平移
- 节点详情面板
- 实时更新

#### App.tsx
**已实现**：
- 基础布局框架（Grid布局）
- CSS变量定义
- 暗色主题
- 响应式媒体查询

**缺失**：
- 路由系统
- 状态管理
- 数据层
- 其他视图组件（10+个缺失）

### 2.4 严重问题

#### ❌ 前后端完全隔离
- **没有任何API调用**：整个前端代码中没有`fetch`、`axios`、`WebSocket`等网络请求
- **硬编码数据**：所有节点、状态都是写死的静态数据
- **无状态同步**：前端无法获取后端的Evidence Graph、Event Log、Blackboard等数据

#### ❌ 缺失核心功能
- 命令面板（Cmd+P）：0%
- 节点检查器：0%
- 工具执行监控：0%
- 报告视图：0%
- 设置面板：0%
- 启动模态框：0%
- 预检查界面：0%
- 回放视图：0%
- Targets管理：0%
- Packs管理：0%
- Evidence库：0%
- Runs历史：0%
- Safety检查：0%

#### ❌ 无交互逻辑
- 所有按钮都是摆设
- 没有导航路由
- 没有模态框/抽屉
- 没有表单处理
- 没有错误处理

---

## 三、后端数据层分析

### 3.1 核心数据模型

#### Evidence Graph (`internal/evidence/`)
```go
// 节点类型
NodeTarget     // 目标（URL/Host/IP）
NodeSurface    // 攻击面（端口/路由/参数）
NodeIdentity   // 身份（账号/凭证）
NodeArtifact   // 制品（响应/截图/日志）
NodeFinding    // 发现（候选漏洞）
NodeAttempt    // 尝试（工具执行记录）
NodeHypothesis // 假设
NodeAgent      // 智能体

// 边类型
EdgeObservedBy  // 观察关系
EdgeSupports    // 支持关系
EdgeContradicts // 矛盾关系
EdgeDerivedFrom // 派生关系
EdgeVerifiedBy  // 验证关系
EdgeRequires    // 依赖关系
EdgeUnlocks     // 解锁关系
EdgeExhaustedBy // 耗尽关系
EdgeAssignedTo  // 分配关系
EdgeProducedBy  // 产出关系
```

#### Blackboard (`internal/blackboard/`)
```go
type Fact struct {
    ID         string
    Kind       string
    Statement  string
    Confidence float64
    Source     string
    CreatedAt  time.Time
}

type Intent struct {
    ID          string
    Kind        string
    Goal        string
    SourceFacts []string
    State       IntentState  // Open/Claimed/Concluded/Released
    Lease       *Lease
    ResultFact  string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type Hint struct {
    ID        string
    Kind      string
    Message   string
    Source    string
    CreatedAt time.Time
}
```

#### Event Log (`internal/eventlog/`)
```go
type Event struct {
    ID        int64
    RunID     string
    Type      EventType  // goal/fact/intent/hint/attempt/agent/tool/parser/verification/mutation
    Payload   json.RawMessage
    CreatedAt time.Time
}

// SQLite存储: replay/events.db
// Schema: WAL模式，支持重放
```

### 3.2 数据存储

```
workspace/
├── replay/
│   └── events.db           # SQLite事件日志（WAL模式）
├── artifacts/              # 内容寻址存储（hash索引）
├── evidence/               # 证据图快照
├── attempts/               # 工具执行记录
├── findings/               # 发现报告
└── reports/                # 最终报告
```

### 3.3 CLI命令映射

后端提供的CLI命令（无HTTP API）：
```bash
ouro init <workspace>           # 初始化工作区
ouro doctor <workspace>         # 健康检查
ouro run <workspace>            # 离线运行
ouro run <workspace> --tui      # TUI快照（非交互）
ouro run <workspace> --interactive  # Bubble Tea TUI
ouro replay <workspace> <run-id>    # 重放
ouro chat <workspace>           # AI对话IDE
ouro toolchain health <workspace>   # 工具链健康
ouro workflow-status <workspace>    # 工作流状态
```

---

## 四、差距总结

### 4.1 实现度统计

| 功能模块 | 设计稿要求 | 当前实现 | 完成度 |
|---------|----------|---------|--------|
| **布局框架** | 4层布局（侧边栏+工具栏+主区+状态栏） | 3层布局（无状态栏） | 75% |
| **侧边栏导航** | 8个功能入口+路由切换 | 8个静态按钮 | 20% |
| **攻击图** | 动态图+交互+缩放+详情 | 静态硬编码6个节点 | 10% |
| **命令面板** | 完整快捷命令系统 | 不存在 | 0% |
| **节点检查器** | 右侧详情面板 | 不存在 | 0% |
| **工具执行监控** | 实时日志+进度 | 不存在 | 0% |
| **报告视图** | 预览+导出 | 不存在 | 0% |
| **设置面板** | 多分类配置 | 不存在 | 0% |
| **启动模态框** | 引导式启动 | 不存在 | 0% |
| **预检查** | 健康检查UI | 不存在 | 0% |
| **回放视图** | 时间轴重放 | 不存在 | 0% |
| **Targets管理** | CRUD界面 | 不存在 | 0% |
| **Packs管理** | 工具包配置 | 不存在 | 0% |
| **Evidence库** | 证据浏览器 | 不存在 | 0% |
| **Runs历史** | 运行记录 | 不存在 | 0% |
| **Safety检查** | 安全边界UI | 不存在 | 0% |
| **前后端通信** | REST API / WebSocket | **完全缺失** | 0% |

**总体完成度**: 约 **8%**（仅有基础布局和静态样式）

### 4.2 关键缺失

#### 🔴 架构层缺失
1. **前后端通信层**：后端是CLI，没有HTTP API服务器
2. **状态管理**：无Redux/Zustand/Jotai等
3. **路由系统**：无React Router
4. **数据层**：无API Client、数据缓存
5. **WebSocket**：无实时更新通道

#### 🔴 功能层缺失
1. 所有导航按钮无功能
2. 所有工具栏按钮无功能
3. 攻击图无交互
4. 无任何模态框
5. 无任何表单
6. 无任何CRUD操作

#### 🔴 体验层缺失
1. 无加载状态
2. 无错误提示
3. 无快捷键
4. 无工具提示
5. 无动画过渡
6. 无响应式适配

---

## 五、后端数据库检查

### 5.1 SQLite Schema

```sql
-- events表 (internal/eventlog/sqlite.go)
CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL,
    type TEXT NOT NULL,
    payload TEXT NOT NULL,  -- JSON
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_run_id ON events(run_id);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);

PRAGMA journal_mode=WAL;  -- 高性能写入
PRAGMA foreign_keys=ON;
```

### 5.2 数据访问模式

```go
// 追加事件
store.Append(ctx, Event{
    RunID:   "run-20260702-001",
    Type:    EventTool,
    Payload: json.RawMessage(`{"tool":"nmap","target":"10.0.0.1"}`),
})

// 重放事件
events := store.Replay(ctx, runID)

// 构建快照
snapshot := eventlog.BuildReplaySnapshot(events)
// → 包含: Blackboard状态、Agent状态、Timeline、Toolchain状态
```

### 5.3 数据持久化路径

```
workspace/
├── replay/events.db        # 事件源（append-only）
├── artifacts/              # 大文件（content-addressed）
│   ├── <hash>.json
│   ├── <hash>.png
│   └── <hash>.log
├── evidence/
│   └── graph-snapshot.json # 定期快照
└── reports/
    └── final-report.md     # 生成报告
```

### 5.4 前端需要的数据接口

如果要实现GUI，后端需要暴露这些API：

```
GET  /api/workspace/status          # 工作区状态
GET  /api/runs                       # 运行历史
GET  /api/runs/:id                   # 单次运行详情
GET  /api/runs/:id/events            # 事件流
GET  /api/graph/nodes                # 证据图节点
GET  /api/graph/edges                # 证据图边
GET  /api/blackboard/facts           # 事实列表
GET  /api/blackboard/intents         # 意图列表
GET  /api/toolchain/health           # 工具链健康
POST /api/run/start                  # 启动渗透测试
POST /api/run/stop                   # 停止运行
WS   /api/events/stream              # 实时事件流（WebSocket）
```

**当前状态**: 这些API **全部不存在**

---

## 六、问题根因

### 6.1 产品定位模糊

设计稿显示的是**GUI产品**，但设计书（`Ouroboros_平台架构设计书.md`）明确说：
> "主产品不是 WebUI，而是一个类似 Codex、Claude Code 的**终端 IDE**"
> "WebUI 不作为第一版目标"
> "技术选型：Go + Bubble Tea + Lip Gloss + Bubbles（TUI框架）"

这导致：
- 后端完全按TUI架构实现（Bubble Tea）
- 没有HTTP服务器
- 前端成了无根之木

### 6.2 实施失误

某个时间点有人做了GUI尝试，但：
1. 没有建立后端API层
2. 只实现了视觉框架
3. 没有连接任何真实数据
4. 项目被搁置

### 6.3 当前状态

- 后端：功能完整的CLI工具 ✅
- 前端：精美的设计稿 ✅
- 集成：**完全断裂** ❌

---

## 七、下一步计划

见 `refactor-plan.md`
