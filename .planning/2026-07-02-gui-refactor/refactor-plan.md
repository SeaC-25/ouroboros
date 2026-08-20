# Ouroboros GUI 重构计划

**制定时间**: 2026-07-02  
**优先级**: P0 - 架构性重构  
**预估工期**: 3-4周（分7个里程碑）

---

## 战略方向确认

根据用户明确要求：**主界面是GUI**，因此本计划按照 Electron + React GUI 方向制定。

---

## 总体策略

### 核心原则

1. **后端优先**：先建立Go HTTP API服务器，后端数据已完整
2. **增量实现**：按优先级逐个视图实现，每个里程碑可独立交付
3. **严格遵循AGENT.md**：认真查询、寻求确认、复用现有、主动测试
4. **参考设计稿**：100%还原ui-designs中的视觉和交互

### 技术选型

```yaml
后端API层:
  框架: Go net/http (轻量) 或 Gin (快速开发)
  协议: REST API + WebSocket
  端口: 8080 (可配置)
  认证: 本地应用暂不需要

前端增强:
  状态管理: Zustand (轻量级，适合Electron)
  路由: React Router v6
  数据请求: TanStack Query (React Query)
  WebSocket: native WebSocket API
  图可视化: Cytoscape.js 或 ReactFlow
  图标: lucide-react (已有)
  样式: CSS Modules 或 Tailwind CSS (替换inline CSS)
  
工具链:
  TypeScript: 严格模式
  ESLint + Prettier: 代码规范
  Vitest: 单元测试
```

---

## 里程碑划分

### M1: 后端API服务器基础 (3天)

**目标**: 建立前后端通信层

#### 任务清单

1. **创建HTTP服务器模块** (`internal/api/`)
   ```go
   // internal/api/server.go
   type Server struct {
       workspace string
       store     *eventlog.SQLiteStore
       graph     *evidence.Graph
       // ...
   }
   
   func (s *Server) Start(port int) error
   ```

2. **实现核心API端点**
   ```
   GET  /api/health                    # 健康检查
   GET  /api/workspace/info            # 工作区信息
   GET  /api/evidence/graph            # 证据图数据
   GET  /api/evidence/nodes            # 节点列表
   GET  /api/evidence/nodes/:id        # 节点详情
   GET  /api/runs                      # 运行历史
   GET  /api/runs/:id                  # 运行详情
   GET  /api/toolchain/health          # 工具链状态
   ```

3. **实现WebSocket事件流**
   ```
   WS   /api/events/stream             # 实时事件推送
   消息格式: {type: "node_added", payload: {...}}
   ```

4. **修改CLI启动命令**
   ```bash
   ouro serve <workspace> --port 8080
   # 同时启动Electron和API服务器
   ```

5. **CORS配置**
   ```go
   // 允许Electron renderer访问
   AllowOrigin: "http://127.0.0.1:5173"
   ```

#### 验证标准
- [ ] `curl http://localhost:8080/api/health` 返回200
- [ ] `/api/evidence/graph` 返回真实数据
- [ ] WebSocket连接成功并能接收事件

---

### M2: 前端基础架构升级 (2天)

**目标**: 建立现代化前端架构

#### 任务清单

1. **安装依赖**
   ```bash
   npm install zustand react-router-dom @tanstack/react-query
   npm install cytoscape cytoscape-fcose react-cytoscapejs
   npm install clsx tailwindcss
   ```

2. **创建API Client** (`src/lib/api.ts`)
   ```typescript
   class OuroAPI {
     private baseURL = 'http://localhost:8080/api';
     
     async getGraph(): Promise<EvidenceGraph> { ... }
     async getNodes(): Promise<Node[]> { ... }
     async getRuns(): Promise<Run[]> { ... }
     
     subscribeEvents(callback: (event: Event) => void): () => void { ... }
   }
   
   export const api = new OuroAPI();
   ```

3. **状态管理Store** (`src/stores/`)
   ```typescript
   // graphStore.ts
   interface GraphState {
     nodes: Node[];
     edges: Edge[];
     selectedNode: Node | null;
     setNodes: (nodes: Node[]) => void;
     selectNode: (id: string) => void;
   }
   
   export const useGraphStore = create<GraphState>(...);
   
   // appStore.ts - 全局状态
   // runsStore.ts - 运行历史
   // settingsStore.ts - 设置
   ```

4. **路由配置** (`src/router.tsx`)
   ```typescript
   <Routes>
     <Route path="/" element={<Layout />}>
       <Route index element={<Navigate to="/graph" />} />
       <Route path="graph" element={<AttackGraphView />} />
       <Route path="targets" element={<TargetsView />} />
       <Route path="packs" element={<PacksView />} />
       <Route path="evidence" element={<EvidenceView />} />
       <Route path="runs" element={<RunsView />} />
       <Route path="runs/:id" element={<RunDetailView />} />
       <Route path="safety" element={<SafetyView />} />
       <Route path="settings" element={<SettingsView />} />
     </Route>
   </Routes>
   ```

5. **TypeScript类型定义** (`src/types/`)
   ```typescript
   // evidence.ts
   export type NodeType = 
     | 'Target' | 'Surface' | 'Identity' | 'Artifact' 
     | 'Finding' | 'Attempt' | 'Hypothesis' | 'Agent';
   
   export interface Node {
     id: string;
     type: NodeType;
     label: string;
     sourceKind: string;
   }
   
   export interface Edge {
     from: string;
     to: string;
     type: EdgeType;
   }
   
   // ... 其他类型定义
   ```

#### 验证标准
- [ ] API Client能成功调用所有端点
- [ ] 状态管理能正确更新和订阅
- [ ] 路由切换正常
- [ ] TypeScript无编译错误

---

### M3: 攻击图视图重构 (4天)

**目标**: 实现完整的攻击图交互

#### 任务清单

1. **集成Cytoscape.js**
   ```tsx
   // components/AttackGraph/GraphCanvas.tsx
   import CytoscapeComponent from 'react-cytoscapejs';
   
   export function GraphCanvas() {
     const { nodes, edges } = useGraphStore();
     const elements = useMemo(() => convertToElements(nodes, edges), [nodes, edges]);
     
     return (
       <CytoscapeComponent
         elements={elements}
         layout={{ name: 'fcose' }}  // 力导向布局
         stylesheet={cytoscapeStyle}
         onNodeClick={handleNodeClick}
         onNodeHover={handleNodeHover}
       />
     );
   }
   ```

2. **节点样式映射**
   ```typescript
   const cytoscapeStyle = [
     {
       selector: 'node[type="Target"]',
       style: {
         'background-color': '#2dd4bf',
         'border-color': '#1f6f68',
         'shape': 'roundrectangle',
       }
     },
     {
       selector: 'node[type="Finding"]',
       style: {
         'background-color': '#f97316',
         'border-color': '#7c2d12',
       }
     },
     // ... 其他节点类型
   ];
   ```

3. **节点交互**
   - 点击节点 → 打开右侧检查器
   - 拖拽节点 → 更新位置
   - 双击节点 → 展开详情模态框
   - 右键菜单 → 操作菜单（验证、标记、导出）

4. **工具栏功能实现**
   - 搜索 → 高亮匹配节点
   - 重置视图 → 重新布局
   - 过滤器 → 按节点类型/状态过滤
   - 保存 → 保存布局状态

5. **图布局算法配置**
   ```typescript
   const layoutOptions = {
     name: 'fcose',
     quality: 'proof',
     animate: true,
     animationDuration: 1000,
     fit: true,
     padding: 30,
     nodeSeparation: 75,
   };
   ```

6. **缩放和平移**
   - 鼠标滚轮缩放
   - 拖拽画布平移
   - 适应视口按钮
   - 缩放级别显示

#### 验证标准
- [ ] 从API加载真实节点数据
- [ ] 节点布局美观且无重叠
- [ ] 所有交互响应流畅
- [ ] 工具栏功能完整可用

---

### M4: 节点检查器 + 侧边栏导航 (3天)

**目标**: 实现详情面板和导航系统

#### 任务清单

1. **右侧抽屉组件** (`components/Inspector/`)
   ```tsx
   // Inspector.tsx
   export function Inspector() {
     const selectedNode = useGraphStore(s => s.selectedNode);
     
     if (!selectedNode) return <EmptyInspector />;
     
     return (
       <aside className="inspector">
         <InspectorHeader node={selectedNode} />
         <Tabs defaultValue="overview">
           <TabsList>
             <TabsTrigger value="overview">Overview</TabsTrigger>
             <TabsTrigger value="evidence">Evidence</TabsTrigger>
             <TabsTrigger value="history">History</TabsTrigger>
             <TabsTrigger value="actions">Actions</TabsTrigger>
           </TabsList>
           <TabsContent value="overview">
             <NodeOverview node={selectedNode} />
           </TabsContent>
           {/* ... */}
         </Tabs>
       </aside>
     );
   }
   ```

2. **节点详情组件**
   - `NodeOverview`: 元数据展示
   - `EvidenceList`: 关联证据
   - `HistoryTimeline`: 变更历史
   - `ActionButtons`: 操作按钮

3. **侧边栏导航激活状态**
   ```tsx
   // components/Sidebar.tsx
   export function Sidebar() {
     const location = useLocation();
     
     const items = [
       { path: '/graph', label: 'Attack graph', icon: GitBranch },
       { path: '/targets', label: 'Targets', icon: RadioTower },
       // ...
     ];
     
     return items.map(item => (
       <NavButton
         key={item.path}
         to={item.path}
         active={location.pathname === item.path}
         icon={item.icon}
         label={item.label}
       />
     ));
   }
   ```

4. **路由联动**
   - 点击侧边栏 → 切换视图
   - 更新浏览器历史
   - 保持状态持久化

#### 验证标准
- [ ] 点击节点弹出检查器
- [ ] 检查器展示真实数据
- [ ] Tab切换流畅
- [ ] 侧边栏导航正常工作

---

### M5: 命令面板 + 核心模态框 (3天)

**目标**: 实现快捷操作系统

#### 任务清单

1. **命令面板组件** (`components/CommandPalette/`)
   ```tsx
   // CommandPalette.tsx
   export function CommandPalette() {
     const [open, setOpen] = useState(false);
     const [query, setQuery] = useState('');
     
     useHotkey('cmd+k', () => setOpen(true));
     
     const commands = useMemo(() => filterCommands(allCommands, query), [query]);
     
     return (
       <Dialog open={open} onOpenChange={setOpen}>
         <DialogContent className="command-palette">
           <CommandInput value={query} onChange={setQuery} />
           <CommandList>
             {commands.map(cmd => (
               <CommandItem
                 key={cmd.id}
                 icon={cmd.icon}
                 label={cmd.label}
                 shortcut={cmd.shortcut}
                 onSelect={() => executeCommand(cmd)}
               />
             ))}
           </CommandList>
         </DialogContent>
       </Dialog>
     );
   }
   ```

2. **命令注册系统**
   ```typescript
   // commands.ts
   export const commandRegistry = [
     {
       id: 'graph.reset',
       label: 'Reset graph layout',
       category: 'Graph',
       shortcut: 'cmd+r',
       execute: () => { /* ... */ },
     },
     {
       id: 'run.start',
       label: 'Start new pentest',
       category: 'Workspace',
       icon: Play,
       execute: () => openStartModal(),
     },
     // ... 50+ 命令
   ];
   ```

3. **启动渗透测试模态框** (`components/Modals/StartPentest/`)
   ```tsx
   export function StartPentestModal() {
     const [step, setStep] = useState(1);
     
     return (
       <Modal size="large">
         <ModalHeader>Start New Penetration Test</ModalHeader>
         <ModalBody>
           {step === 1 && <TargetInput />}
           {step === 2 && <ScopeDefinition />}
           {step === 3 && <PackSelection />}
           {step === 4 && <SafetyConfirmation />}
           {step === 5 && <PreflightCheck />}
         </ModalBody>
         <ModalFooter>
           <Button onClick={handleBack}>Back</Button>
           <Button variant="primary" onClick={handleNext}>
             {step === 5 ? 'Launch' : 'Next'}
           </Button>
         </ModalFooter>
       </Modal>
     );
   }
   ```

4. **设置面板** (`views/SettingsView/`)
   - 分类Tab：General / Providers / Toolchain / Safety
   - 表单验证
   - 保存到后端

5. **快捷键系统**
   ```typescript
   // hooks/useHotkeys.ts
   const hotkeyBindings = {
     'cmd+k': 'Open command palette',
     'cmd+p': 'Search nodes',
     'cmd+,': 'Open settings',
     'g': 'Focus graph',
     'a': 'Focus agents',
     '1-9': 'Switch view',
   };
   ```

#### 验证标准
- [ ] Cmd+K打开命令面板
- [ ] 命令搜索和执行正常
- [ ] 启动模态框表单验证完整
- [ ] 设置保存成功
- [ ] 所有快捷键生效

---

### M6: 次要视图实现 (5天)

**目标**: 完成剩余功能视图

#### 任务清单

1. **Targets管理视图** (`views/TargetsView/`)
   - 目标列表（表格）
   - 添加/编辑/删除目标
   - Scope配置
   - 批量导入

2. **Packs管理视图** (`views/PacksView/`)
   - Pack卡片展示（Web API / Infra / CTF）
   - Pack配置编辑
   - 工具清单展示
   - 启用/禁用开关

3. **Evidence库视图** (`views/EvidenceView/`)
   - 证据列表（按类型分类）
   - 搜索和过滤
   - 证据详情预览
   - 导出功能

4. **Runs历史视图** (`views/RunsView/`)
   - 运行记录表格
   - 状态指示器（运行中/成功/失败）
   - 快速重放按钮
   - 删除/归档操作

5. **Safety检查视图** (`views/SafetyView/`)
   - 授权范围展示
   - 安全边界配置
   - 审计日志
   - 紧急停止按钮

6. **空状态组件** (`components/EmptyState/`)
   - 统一的空状态样式
   - 引导文案
   - 快速开始按钮

#### 验证标准
- [ ] 所有CRUD操作可用
- [ ] 数据与后端同步
- [ ] 表单验证完整
- [ ] 空状态友好

---

### M7: 高级功能 + 抛光 (4天)

**目标**: 实现报告、回放、实时更新

#### 任务清单

1. **报告视图** (`views/ReportView/`)
   ```tsx
   export function ReportView() {
     const { runId } = useParams();
     const { data: report } = useQuery(['report', runId], () => api.getReport(runId));
     
     return (
       <div className="report-view">
         <ReportToolbar onExport={handleExport} />
         <div className="report-preview">
           <MarkdownRenderer content={report.content} />
         </div>
         <ReportToc sections={report.sections} />
       </div>
     );
   }
   ```

2. **回放视图** (`views/ReplayView/`)
   ```tsx
   export function ReplayView() {
     const { runId } = useParams();
     const [currentTime, setCurrentTime] = useState(0);
     const { data: events } = useQuery(['events', runId], () => api.getEvents(runId));
     
     return (
       <div className="replay-view">
         <Timeline
           events={events}
           currentTime={currentTime}
           onSeek={setCurrentTime}
         />
         <EventLanes events={filteredEvents} />
         <PlaybackControls
           onPlay={handlePlay}
           onPause={handlePause}
           onStep={handleStep}
           speed={playbackSpeed}
         />
       </div>
     );
   }
   ```

3. **工具执行监控** (`components/ToolExecution/`)
   - 实时日志流（WebSocket）
   - 彩色输出渲染
   - 进度条
   - 停止/重试按钮

4. **实时更新系统**
   ```typescript
   // hooks/useRealtimeUpdates.ts
   export function useRealtimeUpdates() {
     const { refetch: refetchGraph } = useGraphStore();
     
     useEffect(() => {
       const unsubscribe = api.subscribeEvents((event) => {
         switch (event.type) {
           case 'node_added':
             refetchGraph();
             break;
           case 'edge_added':
             refetchGraph();
             break;
           case 'tool_started':
             showToolExecutionPanel(event.payload);
             break;
         }
       });
       
       return unsubscribe;
     }, []);
   }
   ```

5. **UI抛光**
   - 加载骨架屏
   - 错误边界处理
   - Toast通知
   - 动画过渡（Framer Motion）
   - 响应式布局优化

6. **性能优化**
   - 虚拟滚动（大列表）
   - 图节点懒加载
   - 防抖/节流
   - Memo化组件

#### 验证标准
- [ ] 报告渲染正确
- [ ] 回放流畅无卡顿
- [ ] 实时更新延迟<500ms
- [ ] 错误处理完善
- [ ] 性能达标（60fps）

---

## 实施细节

### 遵循AGENT.md规范

#### 文件创建规范
```typescript
// ✅ 使用.ts/.tsx文件
// ✅ 代码使用TypeScript严格模式
// ✅ 注释使用中文，变量名用英文
// ✅ 保持代码注释密度≥30%
```

#### 代码编写规范
```typescript
// 命名约定
const userName = '';           // 变量：camelCase
function getUserById() {}      // 函数：camelCase
class UserService {}           // 类：PascalCase
const MAX_RETRIES = 3;        // 常量：UPPER_CASE

// 文件头注释
/**
 * 文件名：graphStore.ts
 * 功能：攻击图状态管理，提供节点、边的增删改查
 * 作者：Ouroboros Team
 * 创建日期：2026-07-02
 * 最后修改：2026-07-02
 */

// 函数注释（Google风格）
/**
 * 根据节点ID查找节点详情
 * 
 * @param nodeId - 节点唯一标识符
 * @returns 节点对象，不存在返回undefined
 * @throws {Error} 当nodeId为空时
 */
function getNodeById(nodeId: string): Node | undefined {
  // 参数验证：防止空ID查询导致全量扫描
  if (!nodeId.trim()) {
    throw new Error('Node ID is required');
  }
  
  return nodesMap.get(nodeId);
}
```

#### 测试规范
```typescript
// ✅ 每个核心功能模块必须有测试
// ✅ 测试覆盖率目标：≥70%
// ✅ 使用Vitest + React Testing Library

// graphStore.test.ts
describe('GraphStore', () => {
  it('should add node to store', () => {
    const { result } = renderHook(() => useGraphStore());
    act(() => {
      result.current.addNode({ id: '1', type: 'Target', label: 'test' });
    });
    expect(result.current.nodes).toHaveLength(1);
  });
});
```

### 目录结构规范

```
gui/src/
├── main/
│   └── main.js                 # Electron主进程
├── renderer/
│   ├── main.tsx                # React入口
│   └── App.tsx                 # 根组件
├── components/                 # 通用组件
│   ├── AttackGraph/
│   │   ├── GraphCanvas.tsx
│   │   ├── NodeCard.tsx
│   │   └── index.ts
│   ├── Inspector/
│   ├── CommandPalette/
│   ├── Modals/
│   └── ui/                     # 基础UI组件（Button/Input/Modal等）
├── views/                      # 页面视图
│   ├── GraphView.tsx
│   ├── TargetsView.tsx
│   ├── PacksView.tsx
│   ├── EvidenceView.tsx
│   ├── RunsView.tsx
│   ├── SafetyView.tsx
│   ├── SettingsView.tsx
│   ├── ReportView.tsx
│   └── ReplayView.tsx
├── stores/                     # Zustand状态管理
│   ├── graphStore.ts
│   ├── appStore.ts
│   ├── runsStore.ts
│   └── settingsStore.ts
├── lib/                        # 工具库
│   ├── api.ts                  # API Client
│   ├── websocket.ts            # WebSocket管理
│   └── utils.ts
├── hooks/                      # 自定义Hooks
│   ├── useRealtimeUpdates.ts
│   ├── useHotkeys.ts
│   └── useGraphLayout.ts
├── types/                      # TypeScript类型
│   ├── evidence.ts
│   ├── event.ts
│   ├── blackboard.ts
│   └── api.ts
├── styles/                     # 样式
│   ├── globals.css
│   └── themes.css
└── router.tsx                  # 路由配置
```

---

## 风险控制

### 技术风险

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| 图布局性能差（节点>1000） | 高 | 使用Web Worker + 分层渲染 |
| WebSocket断连 | 中 | 自动重连 + 离线队列 |
| Electron打包体积大 | 低 | 代码分割 + 按需加载 |
| 跨平台兼容性 | 中 | 在Linux/macOS/Windows测试 |

### 时间风险

- 每个里程碑预留1天buffer
- 优先实现核心功能（M1-M4）
- M5-M7可根据情况调整优先级

### 质量风险

- 每个里程碑完成后进行code review
- 遵循AGENT.md的"以主动测试为荣"原则
- 建立自动化测试覆盖

---

## 验收标准

### 功能完整性
- [ ] 13个设计稿场景100%实现
- [ ] 所有导航按钮可用
- [ ] 所有工具栏功能可用
- [ ] 前后端数据完全同步

### 性能指标
- [ ] 图渲染（1000节点）：<2s
- [ ] 界面响应：<100ms
- [ ] WebSocket延迟：<500ms
- [ ] 内存占用：<500MB

### 代码质量
- [ ] TypeScript编译0错误
- [ ] ESLint 0 warning
- [ ] 测试覆盖率≥70%
- [ ] 注释密度≥30%

### 用户体验
- [ ] 所有交互有反馈
- [ ] 错误提示清晰
- [ ] 快捷键完整
- [ ] 响应式适配

---

## 执行检查点

### Week 1
- [ ] M1: API服务器完成
- [ ] M2: 前端架构升级完成
- [ ] M3: 攻击图视图50%

### Week 2
- [ ] M3: 攻击图视图100%
- [ ] M4: 检查器+导航完成
- [ ] M5: 命令面板完成

### Week 3
- [ ] M5: 核心模态框完成
- [ ] M6: 次要视图50%

### Week 4
- [ ] M6: 次要视图100%
- [ ] M7: 高级功能完成
- [ ] 整体测试和修复

---

## 下一步行动

1. **立即**: 用户确认本计划方向
2. **Day 1**: 启动M1，创建`internal/api/server.go`
3. **Daily**: 更新进度到`progress.md`
4. **Weekly**: 与用户Review当周成果

---

## 附录：关键决策记录

### 为什么选择Zustand而非Redux？
- 更轻量（3KB vs 10KB+）
- API更简单（无boilerplate）
- 适合Electron本地应用
- React Query已处理服务端状态

### 为什么选择Cytoscape而非D3？
- 专为图可视化设计
- 内置布局算法（力导向、层次等）
- 更好的性能（Canvas渲染）
- React集成成熟

### 为什么使用CSS Modules而非Tailwind？
- 可以两者结合：Tailwind for utility，CSS Modules for components
- 保持与设计稿颜色变量的一致性
- 更好的类型提示

### API设计哲学
- RESTful优先，简单直观
- WebSocket仅用于实时推送
- 响应格式统一：`{data, error, meta}`
- 版本化：`/api/v1/...`（为未来预留）
