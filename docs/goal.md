# Ouroboros 长期 Goal

把 Ouroboros 打造成我们自己真正可用、具备真实能力、符合我们预设产品思路的桌面端 AI 安全 IDE + 多智能体安全操作系统。

Ouroboros 不是 Web UI，不是 TUI，不是演示项目，不是普通聊天软件，也不是套壳页面。它的最终形态是完整桌面软件，体验接近 VSCode + Claude/Codex 插件、Cursor/Windsurf、Codex 桌面端软件，以及面向安全操作的 AI Agent IDE。

核心不是“给安全工具加聊天框”，而是让 AI Agent 成为 IDE 内嵌主操作员和安全任务编排器，能理解当前工作区、读写文件、运行命令、调用工具链、调度多智能体、使用黑板算法、查看执行结果、整理证据、更新报告，并持续推进真实安全工作。

## 产品形态

Ouroboros 是：

- 桌面端 AI 安全 IDE
- AI Agent 工作台
- 安全工具链调度器
- 多智能体编排系统
- 黑板算法驱动的证据/任务协作系统
- 自动执行、回放、报告和复盘平台
- Prompt engineering 与 harness engineering 工作台
- 安全任务运行时、评测和迭代平台

它不是 TUI 版 Claude Code，而是桌面端 AI IDE，体验接近 VSCode / Cursor / Codex Desktop 中的 AI Agent 插件，但能力绑定 Ouroboros 自己的工作区、工具链、执行系统、证据图谱、回放、报告和多智能体系统。

## 软件架构定位

- Electron/React 提供中文桌面 IDE 前端。
- Go 后端提供真实能力核心。
- AI Agent 是嵌入式 IDE 操作员，类似 VSCode 中 Claude/Codex 插件。
- Go 后端负责 workspace、eventlog、execution、evidence、toolchain、provider、report、replay、AI provider runtime、AI agent API、multi-agent orchestration、blackboard、harness、prompt registry、evaluation 和 API projection。
- Electron 负责桌面生命周期、workspace 选择、backend sidecar、preload bridge、菜单、窗口状态、文件管理器定位、打包资源和桌面原生能力。
- React 负责中文 IDE 交互和渲染，不在前端伪造业务事实。
- CLI/TUI 可以保留为辅助、测试、自动化和回退能力，但不能主导 GUI 产品结构。

## 核心原则

1. 前端所有可见 UI 必须使用中文。
2. 不使用演示数据、mock 数据、fake fixture、硬编码业务状态。
3. 不伪造 AI 回复、图谱节点、报告、运行记录、执行结果、证据或配置状态。
4. 所有状态必须来自真实工作区、真实文件、真实 eventlog、真实执行记录、真实配置、真实工具链、真实模型调用、真实多智能体状态、真实 harness 结果或真实后端 DTO。
5. 不做产品级安全门控、授权门控、范围门控、审批门控或功能限制流程。
6. `authorization`、`scope` 等文件只能作为普通工作区文件出现，不能作为阻断状态。
7. 不要为了“安全感”限制能力。策略、授权边界和功能限制以后由我们自己手动设计。
8. AI 动作必须通过 Go 后端真实 API 执行，不能在 React 中假装完成。
9. 所有关键 AI 消息、动作、工具调用、执行结果、证据引用、prompt 版本、harness 结果都要可追溯、可回放、可复盘。
10. 每次修改都要优先推进真实可用能力，而不是漂亮但假的界面。
11. 可以大胆改多个相关文件，但必须围绕真实产品能力闭环。
12. 不要回滚用户或已有脏树改动。
13. 不要做无关重构。
14. 不设置任何固定子 agent 数量上限；不要把“单次两个子 agent”或任何数字当作默认规则。
15. 每轮 goal 推进必须优先形成真实产品能力闭环，目标是让整体真实可用度至少推进 5%。
16. 测试和文档只做最小必要护栏，不能替代真实功能推进。

## 桌面 IDE 体验

用户打开的是完整桌面应用，而不是浏览器页面或终端 TUI：

- 左侧是真实工作区文件树、搜索和导航。
- 中间是文件编辑器、执行视图、图谱、回放、报告、设置、harness、prompt、agent 状态等 IDE 工作区。
- 右侧或底部是 AI Agent 工作台。
- AI Agent 能感知当前文件、选中执行、日志、产物、图谱节点、回放事件、报告 finding、prompt 版本、harness 结果、agent 状态和配置状态。
- 用户通过自然语言驱动 IDE 完成真实安全工作。
- 所有动作都有真实后端状态、真实文件或真实 eventlog 支撑。

## AI Agent 工作台

AI Agent 工作台不是普通聊天框，而是 Ouroboros 的主操作入口。

它必须能：

- 使用真实 `providers.yaml` provider/model 配置调用模型。
- 没有 provider 时显示真实配置缺失状态，不伪造回复。
- 持久化用户消息、AI 回复、上下文引用、动作请求和动作结果。
- 读取当前工作区摘要。
- 读取、搜索、解释、创建、编辑、删除、重命名工作区文件。
- 生成文件修改建议，展示 diff，并能应用到真实文件。
- 运行命令、启动任务、调用工具链动作。
- 查看执行状态、退出码、日志、产物和重试关系。
- 根据失败结果继续推理下一步。
- 读取 eventlog 并解释真实回放时间线。
- 分析 evidence graph、report findings、执行记录和产物。
- 生成、整理、更新报告文件。
- 在文件、执行、图谱、回放、报告、prompt、harness、agent 状态之间建立真实引用。
- 所有动作都通过 Go 后端真实 API 执行。

## AI 安全能力与多智能体调度

Ouroboros 的 AI Agent 不能只像普通代码助手一样读写文件和运行 shell。它必须能调用 Ouroboros 底层已经设计和实现的安全能力，包括：

- 安全工具链
- 工具 manifest
- 任务执行系统
- eventlog
- evidence graph
- replay
- report projection
- provider runtime
- worker / swarm / multi-agent 架构
- 黑板算法
- frontier / budget allocator
- goal dependency / priority queue
- sisyphus failure analyzer / mutator / strategy selector
- ant colony / explore-plan-execute / ralph loop 等策略模式
- 未来继续加入的安全分析、自动化编排和多智能体能力

AI Agent 必须成为这些能力的自然语言操作层和智能编排层。

它应该能：

1. 根据用户目标选择合适的底层安全工具。
2. 根据当前 workspace / evidence / eventlog 判断下一步该调用什么能力。
3. 自动调用工具链 manifest 中的真实工具动作。
4. 自动发起命令执行和工具执行。
5. 自动读取日志、产物和退出码。
6. 自动把执行结果写回 evidence / replay / report。
7. 自动调用多智能体架构分派任务。
8. 自动使用黑板算法汇总不同 agent / worker / tool 的发现。
9. 自动基于 frontier / budget allocator 决定探索优先级。
10. 自动基于 goal dependency / priority queue 拆分和推进任务。
11. 自动基于 sisyphus failure analyzer 识别失败原因，并用 mutator / strategy selector 生成下一步尝试。
12. 自动把多智能体执行过程投影到 GUI：谁在执行、执行什么、产出什么、失败在哪里、下一步是什么。
13. 自动把这些过程落到真实 eventlog、execution records、evidence graph 和 reports。

## Harness Engineering 工程

Ouroboros 必须内置 harness engineering 能力，用于评测、验证、回放和迭代 AI、安全工具、多智能体策略与 prompt。

Harness 不是测试脚本的零散集合，而是产品的一部分。

Harness 系统必须支持：

- 定义 harness case。
- 定义输入工作区、任务目标、文件状态、工具链状态、provider 配置和期望结果。
- 执行 AI Agent 工作流。
- 执行安全工具链工作流。
- 执行多智能体编排工作流。
- 执行 prompt 评测。
- 执行 regression replay。
- 比较不同 prompt、不同模型、不同策略、不同 agent 配置的结果。
- 记录成功、失败、成本、耗时、token、工具调用、文件变更、执行记录、证据质量、报告质量。
- 将 harness 结果写入真实工作区或专门的 harness 输出目录。
- 在 GUI 中展示 harness 列表、运行状态、结果、diff、失败原因、回放和指标。
- 支持从真实失败案例一键生成 harness case。
- 支持把一个历史 eventlog 作为 harness replay 输入。
- 支持把一个 workspace snapshot 作为 harness fixture，但不能作为产品运行 fake 数据。
- 支持 CLI 自动化和桌面 GUI 运行。

Harness 工程目标：

- 保证 AI Agent 改动不会破坏真实工作流。
- 评估 prompt 和策略是否真的提升能力。
- 固定多智能体调度和工具调用行为。
- 让每次产品推进都有可重复验证证据。
- 为后续模型、prompt、工具、策略迭代提供实验平台。

## Prompt Engineering 工程

Ouroboros 必须有系统化 prompt engineering 工程，而不是把 prompt 写死在代码里。

Prompt 系统必须支持：

- Prompt registry。
- Prompt versioning。
- System prompt、developer prompt、task prompt、tool prompt、reflection prompt、planner prompt、reviewer prompt、report writer prompt 等分类。
- Prompt 模板变量。
- 工作区上下文注入。
- 文件上下文注入。
- 执行上下文注入。
- evidence / replay / report 上下注入。
- 多智能体角色 prompt。
- 工具调用 prompt。
- 失败分析 prompt。
- mutation / retry prompt。
- report generation prompt。
- prompt diff 和版本历史。
- prompt 评测结果关联。
- prompt 与 harness case 绑定。
- prompt 与模型/provider 配置绑定。
- prompt 变更可回滚、可比较、可审计。
- GUI 中可查看、编辑、复制、比较、运行 prompt。
- Prompt 不能只硬编码在 Go 或 React 里；默认 prompt 可以内置，但必须能被工作区配置覆盖或版本化管理。

Prompt engineering 目标：

- 让 AI 行为可控、可迭代、可评测。
- 让不同 agent 角色有清晰职责。
- 让工具调用、文件编辑、报告生成、失败修复、多智能体调度都有可维护 prompt。
- 让 prompt 变更通过 harness 验证，而不是凭感觉改。

## Agent / Tool / Action 架构

Ouroboros 必须清晰区分：

- AI message：用户和模型的自然语言消息。
- AI action：模型请求执行的结构化动作。
- Tool action：工具链 manifest 中定义的真实工具动作。
- Execution：后端实际执行的命令或工具。
- Agent task：分配给某个 agent / worker 的任务。
- Blackboard item：多智能体共享状态、发现、假设、问题、结果。
- Evidence item：可追溯的证据对象。
- Replay event：可回放的时间线事件。
- Report finding：报告中的发现对象。
- Harness result：评测运行结果。
- Prompt version：参与本次推理或动作的 prompt 版本。

这些对象之间必须有真实 ID 和引用关系，不能靠前端字符串拼接伪造。

## 黑板算法与协作状态

黑板系统必须成为多智能体协作的核心共享状态。

它应支持：

- 记录当前目标、子目标、假设、发现、待验证项、失败项、证据、工具结果。
- 多 agent 写入和读取。
- AI Agent 根据黑板状态决定下一步。
- GUI 展示黑板当前状态。
- 黑板条目能关联文件、执行、证据、图谱节点、回放事件、报告 finding。
- 黑板状态能进入 eventlog 和 replay。
- 黑板不是临时内存，而是可持久化、可恢复、可审计的工作区状态。

## 多智能体系统

多智能体系统必须支持：

- Agent 角色定义。
- Agent 能力定义。
- Agent 任务分派。
- Agent 状态展示。
- Agent 之间通过黑板共享状态。
- Agent 输出结构化结果。
- Agent 失败分析。
- Agent retry / mutation / strategy switch。
- Agent 与工具链执行系统连接。
- Agent 与 evidence graph / report / replay 连接。
- GUI 展示 agent 拓扑、状态、当前任务、产物和失败原因。
- Harness 能评测多智能体策略。

可用角色可以包括：

- Planner
- Operator
- Tool Runner
- Evidence Analyst
- Report Writer
- Reviewer
- Failure Analyzer
- Strategy Mutator
- Prompt Engineer
- Harness Evaluator

角色可以演进，但不能只是 UI 标签，必须能映射到真实后端行为和 eventlog。

## 安全工具链调度

AI 必须能调度底层安全工具链，而不只是让用户手动点按钮。

工具链系统必须支持：

- 从 manifest 读取工具能力。
- AI 根据任务和上下文选择工具。
- AI 生成工具参数。
- 后端执行工具动作。
- 记录 execution。
- 收集日志和产物。
- 解析工具结果。
- 写入 evidence graph。
- 写入 replay。
- 更新 report findings。
- 失败时进入 failure analyzer。
- 可在 GUI 中查看工具执行状态和结果。
- 可在 harness 中复现工具执行或使用历史执行结果回放。

## 工作区与持久化

工作区必须成为所有真实状态的根。

必须持久化：

- AI conversation
- AI actions
- Tool actions
- Executions
- Eventlog
- Replay
- Evidence graph
- Reports
- Findings
- Prompt versions
- Harness cases
- Harness results
- Blackboard state
- Agent tasks
- Agent outputs
- Provider config
- Toolchain manifests
- Workspace settings
- File edits and diffs

不能只存在 React state 或进程内临时变量中。

## GUI 工作区模块

桌面 GUI 最终应包含这些模块：

- AI Agent 工作台
- 文件树和搜索
- 文件编辑器
- Diff / patch 视图
- 命令和工具执行面板
- 多智能体状态面板
- 黑板状态面板
- Evidence graph
- Replay timeline
- Report editor / preview
- Settings
- Provider / model 配置
- Toolchain manifest 管理
- Prompt registry / prompt editor
- Harness case / harness run / harness result 面板
- Eventlog inspector
- Package / runtime status
- Desktop integration 状态

每个模块都必须读取真实后端 DTO，不伪造业务状态。

## 用户应该能完成的真实工作

用户打开软件后，应该能：

- 打开或新建 Ouroboros 工作区。
- 自动启动 Go 后端 sidecar。
- 配置 provider、模型、工具链和工作区状态。
- 打开 AI Agent 工作台，通过自然语言驱动 IDE。
- 查看、搜索、创建、编辑、删除、重命名工作区文件。
- 让 AI 读取和理解当前文件或整个工作区。
- 让 AI 修改文件并展示真实 diff。
- 让 AI 创建任务、保存任务、启动任务。
- 让 AI 选择和调用安全工具链动作。
- 让 AI 分派多智能体任务。
- 让 AI 使用黑板状态推进协作。
- 直接运行命令和工具链动作。
- 查看执行状态、退出码、日志、产物、重试关系。
- 让 AI 根据执行结果总结、修正和继续推进。
- 从 eventlog 中查看真实回放时间线。
- 从证据和 findings 中查看真实图谱、发现和报告。
- 让 AI 整理证据并更新报告。
- 设计、编辑、运行和评测 prompt。
- 创建和运行 harness case。
- 对比模型、prompt、策略和多智能体配置效果。
- 在 AI 对话、文件、执行、图谱、回放、报告、prompt、harness、agent、黑板、设置之间互相跳转。
- 打包成可以直接运行的桌面软件。

## 当前最高优先级

1. 建立真实 AI Agent 工作台，而不是普通聊天框。
2. 后端提供真实 AI chat / agent API，使用 `providers.yaml` 中的真实 provider 配置。
3. AI 消息、动作历史、上下文引用和动作结果必须持久化到工作区或 eventlog。
4. AI 能读取真实 workspace context，包括文件、执行、日志、产物、图谱、回放、报告和设置。
5. AI 能发起真实动作：编辑文件、运行命令、启动任务、调用工具链、打开/定位文件、生成报告内容。
6. AI 能调度安全工具链、多智能体架构和黑板系统。
7. 建立 prompt registry 和 prompt versioning 基础。
8. 建立 harness case / harness result 基础。
9. AI 能展示 diff、动作结果、执行结果和下一步建议。
10. 继续深化 graph / replay / report 的真实数据投影和交互。
11. 继续强化 Electron 桌面体验。
12. 完善打包和 smoke。
13. 清理剩余硬编码、英文文案、旧 Web UI 痕迹和无意义占位。

## 推进原则

我们要大胆推进，不要过度保守。

当前问题是每次只推进一个很小的点，比如一个按钮、一个桥接、一个局部 smoke。这样虽然稳定，但效率太低，无法快速把 Ouroboros 推到真正可用的 AI 桌面 IDE。

后续工作必须按“产品能力块”推进，而不是按“微小 UI 点”推进。

每一轮继续推进 goal 时，必须把真实可用度至少推进 5% 作为默认交付目标。这里的 5% 不是按文件数、测试数或文档页数计算，而是按用户能实际使用到的产品能力闭环计算。不能用大量测试、文档、计划、审计、调研、局部重命名或内部铺垫来冒充 5% 进度。如果一轮工作没有形成用户能感知、能运行、能复盘的新能力，就不算完成了 5% 推进。每轮交付必须明确对应一个或多个真实能力闭环，并且这些闭环要能在桌面 IDE、后端 API、持久化状态或打包运行时中被实际使用。

全面解除“单次最多两个子 agent”之类的协作限制。不要设置 2 个、3 个、5 个、8 个或任何固定数量的默认上限，不要设置单轮 agent 配额，不要把“两个 agent”当作默认工作方式，也不要因为旧习惯把并行度压到保守水平。只要任务可以拆成互不冲突的写入范围，就应该大胆并行使用多个子 agent；agent 数量由真实任务拆分、文件写入冲突、集成复杂度、工具可用性和当前推进目标动态决定，而不是由固定规则决定。子 agent 默认承担可写实现任务、GUI 实现、后端实现、Electron/打包实现、smoke/验证实现、集成修复等真实推进工作，而不是只做只读审计。

从现在开始，goal 推进默认采用高并行实现模式。只要存在多个可以独立推进的实现块，就应该按实际需要分派对应数量的可写 agent。限制并行度的理由只能是明确的文件冲突、集成风险、工具不可用或任务本身无法拆分，不能是习惯性保守、担心改动太大、想先审计一轮，或为了让单次提交看起来更小。不要为了降低表面风险而人为压缩 agent 数量；正确做法是拆清楚写入边界、同步接口契约，然后并行推进。

每轮推进时，主 agent 应该先识别可并行的产品能力块，并尽可能同时分派多个可写子任务。后端、前端、Electron、打包、smoke、数据模型、prompt、harness、多智能体和黑板投影都可以在互不覆盖写入范围时并行推进。主 agent 负责关键路径实现、冲突协调、集成、必要验证和提交；不能把子 agent 当成只读 reviewer 使用，也不能把“只读审计完成”当成主要进度。

测试和文档只作为必要护栏，默认压缩到能证明真实能力没有回退的最小范围：

- 优先写真实功能代码、真实后端能力、真实桌面 GUI 能力、真实持久化和真实集成链路。
- 少写泛化测试，少写重复文档，少做只证明已有行为的低价值验证。
- 不为凑流程扩张测试矩阵，不把文档同步当成主要工作量。
- 对高风险链路保留 focused test / smoke，确保真实能力不伪造、不回退、不丢数据。
- 文档只更新会影响后续执行方向、用户使用方式或架构事实的内容。
- 不要让测试和文档成为主要产出；主要产出必须是可运行、可感知、可复盘的产品能力。
- 如果测试或文档工作开始挤占实现时间，优先收敛验证范围，把时间投入真实产品能力推进。
- 不要为了追求测试覆盖率或文档完整度拖慢核心产品推进；本阶段优先把软件做成真实可用的 AI IDE。

每一轮应该优先完成一个可以明显提升真实可用性的能力闭环，例如：

- AI 工作台第一版：真实 provider 调用 + 消息持久化 + 工作区上下文 + 中文 UI。
- AI 文件操作闭环：AI 读取文件 + 生成修改建议 + 应用文件修改 + 显示 diff + 保存结果。
- AI 执行闭环：AI 生成命令 + 运行命令 + 读取日志/产物 + 总结结果 + 给出下一步。
- AI 工具链闭环：AI 选择工具 + 运行工具 + 解析产物 + 写入证据。
- AI 多智能体闭环：AI 拆分任务 + 分派 agent + 黑板汇总 + 结果回写。
- AI 证据闭环：AI 读取 eventlog/执行记录/产物 + 生成证据说明 + 关联图谱/报告。
- AI 报告闭环：AI 根据 findings/证据/执行记录生成或更新真实报告文件。
- Prompt 工程闭环：编辑 prompt + 运行 harness + 比较结果 + 版本化保存。
- Harness 工程闭环：创建 case + 执行 workflow + 记录结果 + 回放失败。
- 桌面 IDE 工作区闭环：文件树、编辑器、执行面板、AI 面板、图谱、报告之间完整联动。

允许一次修改多个相关文件、多个组件、多个 API，只要它们服务于同一个真实产品能力闭环。

不要因为害怕改动大，就只做局部小补丁。  
不要把“容易验证”放在“真实产品进展”前面。  
不要为了保持每次 commit 很小而牺牲整体推进速度。  
不要只做铺垫、审计、计划、按钮、文案、桥接，而不形成用户可用的能力。

默认推进方式：

1. 选一个真实能力闭环。
2. 拆成多个互不冲突的可写任务，尽可能并行交给所有有必要的子 agent 推进，不设固定数量上限。
3. 同时修改前端、后端、Electron、API、smoke、prompt、harness 和计划文件中必要部分。
4. 子 agent 不只做只读审计，要拆分成可写实现任务。
5. 主 agent 负责整合、必要验证和提交。
6. 每轮必须尽量产出用户能感知到的新能力，而不是只产出内部结构变化。
7. 除非用户明确要求只做审计、只写计划或只写文档，否则每轮 goal 推进都必须落到真实代码、真实 API、真实 GUI、真实运行时、真实持久化或真实打包能力上。
8. 如果某轮推进预计达不到至少 5% 的真实可用度提升，应该扩大能力块、增加并行子任务或合并相邻闭环，而不是用测试、文档、审计或计划填充工作量。

## AI 工作台验收标准

Ouroboros 必须至少能做到：

- 中文 AI Agent 工作台可用。
- AI 对话使用真实 provider/model 配置。
- 没有 provider 时显示真实配置缺失状态，不伪造回复。
- 用户消息持久化。
- AI 回复持久化。
- AI 动作请求和动作结果持久化。
- AI 能读取当前工作区摘要。
- AI 能引用真实文件路径。
- AI 能引用真实执行 ID、日志路径、产物路径。
- AI 能触发真实命令执行。
- AI 能触发真实文件编辑或文件创建。
- AI 能选择并调用真实工具链动作。
- AI 能创建或推进 goal / subgoal。
- AI 能调用多智能体任务分派能力。
- AI 能显示多智能体执行状态。
- AI 能使用黑板状态汇总发现。
- AI 能基于失败结果自动提出下一步 mutation / retry / strategy。
- AI 能展示真实 diff 或文件变更摘要。
- AI 动作结果能在 UI 中回显。
- AI 结果能跳转到对应文件、执行、图谱、回放、报告、prompt、harness、agent 或黑板对象。
- 打包后的桌面 app 中 AI 工作台仍然可用。

## Prompt / Harness 验收标准

Prompt engineering 必须至少能做到：

- 查看 prompt registry。
- 查看 prompt version。
- 编辑 workspace-level prompt。
- 运行 prompt harness。
- 比较 prompt 输出。
- 将 prompt 与模型/provider 配置、agent 角色、harness case 关联。
- 保存 prompt 变更和评测结果。

Harness engineering 必须至少能做到：

- 创建 harness case。
- 运行 harness case。
- 查看 harness result。
- 记录输入、输出、执行、AI 消息、工具调用、文件 diff、eventlog 引用和指标。
- 从失败工作流生成 harness case。
- 在 CLI 和 GUI 中都能运行基础 harness。
- 打包桌面 app 中能查看 harness 结果。

## 通用验证

每个能力块至少根据影响范围运行：

```bash
npm --prefix gui run test:desktop-contract
npm --prefix gui run test:contract
npm --prefix gui run test:product-contract
sh scripts/check_no_gui_mock_literals.sh
GOPROXY=https://goproxy.cn,direct go test ./internal/api -count=1
npm --prefix gui run build
GOPROXY=https://goproxy.cn,direct ./scripts/smoke_desktop_gui.sh
npm --prefix gui run package:desktop:dir
GOPROXY=https://goproxy.cn,direct ./scripts/smoke_desktop_package.sh
GOPROXY=https://goproxy.cn,direct ./scripts/smoke_desktop_package_runtime.sh
```

如果某一轮影响 AI provider、执行、文件写入、eventlog、prompt、harness、多智能体、黑板或报告，还要补对应 focused test / smoke，不能只依赖宽泛构建通过。

## 最终判断

不要问“这个界面看起来像不像产品”。

要问：

这个 AI IDE 是否真的能像 VSCode/Codex/Claude 插件一样，帮助我们理解工作区、修改文件、运行任务、调用工具、调度多智能体、使用黑板算法、读取结果、保存证据、评测 prompt、运行 harness、回放过程、查看报告、管理配置，并持续推进真实安全工作？

只要答案还不是 100% 是，就继续大胆、成块、系统性地推进。
