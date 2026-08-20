# Ouroboros GUI 重构执行摘要

## 🔍 核心问题

你的前端"跟 shit 一样"的**根本原因**：

```
❌ 前端 (Electron + React) ⚡ 后端 (Go CLI)
   
   前端只有空壳，没有连接到后端数据！
```

**当前状态**：
- ✅ 后端：功能完整的 Go 应用，数据模型齐全
- ✅ 设计稿：13张精美的UI设计图
- ❌ **集成层：完全缺失**（没有HTTP API，前端无法获取数据）

**实现度**: 仅 **8%**（只有布局框架和静态样式）

---

## 📊 详细分析结果

已生成两份详细报告：

### 1. 全面分析报告 (`analysis.md`)
- ✅ 13张设计稿逐一分析
- ✅ 当前3个组件实现对比
- ✅ 后端数据模型完整梳理
- ✅ 差距统计和问题根因

### 2. 重构执行计划 (`refactor-plan.md`)
- ✅ 7个里程碑，3-4周工期
- ✅ 详细任务清单和验收标准
- ✅ 技术选型和架构设计
- ✅ 风险控制和质量保障

文档位置：
```
/home/zss/ouroboros/Ouroboros/.planning/2026-07-02-gui-refactor/
├── analysis.md        # 分析报告（5000+字）
└── refactor-plan.md   # 重构计划（6000+字）
```

---

## 🎯 重构策略

### 核心思路

```
阶段1: 建立通信层（M1-M2）
  → 后端加HTTP API服务器
  → 前端建立状态管理和API Client

阶段2: 实现核心视图（M3-M4）
  → 攻击图交互重构
  → 节点检查器 + 导航系统

阶段3: 完善功能（M5-M7）
  → 命令面板 + 模态框
  → 次要视图（Targets/Packs/Evidence/Runs/Safety）
  → 高级功能（报告/回放/实时更新）
```

### 技术选型

| 层次 | 技术 | 理由 |
|------|------|------|
| **后端API** | Go net/http + WebSocket | 轻量，与现有Go代码集成 |
| **状态管理** | Zustand | 轻量级，适合Electron |
| **路由** | React Router v6 | 标准方案 |
| **数据请求** | TanStack Query | 服务端状态管理 |
| **图可视化** | Cytoscape.js | 专业图库，性能优 |

### 里程碑时间线

```
Week 1: M1 (API服务器) + M2 (前端架构) + M3 (攻击图 50%)
Week 2: M3 (攻击图 100%) + M4 (检查器+导航) + M5 (命令面板)
Week 3: M5 (核心模态框) + M6 (次要视图 50%)
Week 4: M6 (次要视图 100%) + M7 (高级功能) + 测试
```

---

## 🚀 立即可开始的工作

### M1第一步：创建HTTP API服务器

**位置**: `internal/api/server.go`

**核心端点**:
```
GET  /api/health                # 健康检查
GET  /api/evidence/graph        # 证据图数据
GET  /api/evidence/nodes/:id    # 节点详情
GET  /api/runs                  # 运行历史
WS   /api/events/stream         # 实时事件流
```

**CLI命令**:
```bash
ouro serve <workspace> --port 8080
# 同时启动API服务器和Electron GUI
```

---

## ✅ 验收标准

### 功能完整性
- [ ] 13个设计稿场景 100% 实现
- [ ] 所有按钮功能可用
- [ ] 前后端数据完全同步

### 性能指标
- [ ] 图渲染（1000节点）< 2秒
- [ ] 界面响应 < 100ms
- [ ] 实时更新延迟 < 500ms

### 代码质量
- [ ] TypeScript 编译 0 错误
- [ ] 测试覆盖率 ≥ 70%
- [ ] 注释密度 ≥ 30%

---

## 📋 关键发现

### 设计稿内容（ui-designs/）

1. **main_ui.png** - 主界面布局（侧边栏+工具栏+攻击图）
2. **command_palette.png** - 快捷命令系统
3. **node_inspector.png** - 节点详情面板
4. **tool_execution.png** - 工具运行监控
5. **report_view.png** - 报告预览
6. **settings_panel.png** - 设置界面
7. **start_pentest_modal.png** - 启动向导
8. **empty_state.png** - 空状态引导
9. **preflight_check.png** - 预检查界面
10. **replay_view.png** - 事件回放
11. **ouro_attack_graph.png** - 攻击图详细设计

**配色方案**（已完整提取）：
- 背景：`#0a0a0a`
- 强调色：`#2dd4bf`（青绿）、`#f97316`（橙色）
- 所有CSS变量已在当前App.tsx中定义

### 后端数据结构（完整）

#### Evidence Graph
- 8种节点类型：Target / Surface / Identity / Artifact / Finding / Attempt / Hypothesis / Agent
- 10种边类型：observed_by / supports / contradicts / derived_from / verified_by / requires / unlocks / exhausted_by / assigned_to / produced_by

#### Blackboard
- Fact：已确认事实
- Intent：探索意图（Open/Claimed/Concluded/Released）
- Hint：策略提示

#### Event Log
- SQLite存储（WAL模式）
- 10种事件类型：goal / fact / intent / hint / attempt / agent / tool / parser / verification / mutation

### 当前实现（极简）

- `Sidebar.tsx`（49行）：8个静态按钮，无功能
- `Toolbar.tsx`（31行）：5个静态按钮，无功能
- `AttackGraph.tsx`（95行）：6个硬编码节点，无交互
- `App.tsx`（377行）：布局框架 + CSS-in-JS

**缺失功能清单**（16项）：
1. 命令面板 - 0%
2. 节点检查器 - 0%
3. 工具执行监控 - 0%
4. 报告视图 - 0%
5. 设置面板 - 0%
6. 启动模态框 - 0%
7. 预检查界面 - 0%
8. 回放视图 - 0%
9. Targets管理 - 0%
10. Packs管理 - 0%
11. Evidence库 - 0%
12. Runs历史 - 0%
13. Safety检查 - 0%
14. 导航路由 - 0%
15. 状态管理 - 0%
16. **前后端通信 - 0%** ⚠️

---

## 🤔 需要你确认的问题

1. **是否认可这个重构计划？**
   - 3-4周工期
   - 7个里程碑
   - 按优先级实现

2. **是否立即开始M1？**
   - 先建立后端API服务器
   - 然后前端才能连接数据

3. **技术选型是否有异议？**
   - Zustand（状态管理）
   - Cytoscape.js（图可视化）
   - TanStack Query（数据请求）

4. **是否需要调整优先级？**
   - 当前：攻击图 > 检查器 > 命令面板 > 次要视图
   - 可根据实际需求调整

---

## 📞 下一步行动

**请告诉我**：
1. 你是否认可这个计划？
2. 是否有需要调整的部分？
3. 是否立即开始实施M1（后端API服务器）？

我会严格按照AGENT.md的"八荣八耻"执行：
- ✅ 以认真查询为荣（已完整扫描所有设计稿和代码）
- ✅ 以寻求确认为荣（现在等待你的确认）
- ✅ 以复用现有为荣（使用现有后端数据结构）
- ✅ 以主动测试为荣（每个里程碑都有测试要求）
- ✅ 以遵循规范为荣（TypeScript严格模式+注释规范）
- ✅ 以谨慎重构为荣（增量实现，每个里程碑可独立验收）
