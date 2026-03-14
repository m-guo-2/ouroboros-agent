## Why

Monitor 页面是系统可观测性的唯一入口，但它的设计模型与 Agent 的实际运行模型存在根本性的不匹配。

**运行模型已变化**：`processOneEvent` 不再是简单的"用户消息 → 一次 LLM 调用 → 回复"。引入 absorb-replan 循环后，一个处理周期包含：
1. 初始用户消息
2. 多轮 Execute → Checkpoint → Absorb 循环（最多 5 轮）
3. 每轮可能触发上下文压缩（compact）
4. 处理期间到达的新消息被吸纳

**当前 UI 无法表达这些**：
- 对话流和决策过程挤在同一个纵向列表中，相互干扰
- 上下文压缩事件**完全不可见**（数据在 `context_compactions` 表中，但无 API、无展示）
- 吸纳的新消息（absorb）不可见——用户不知道 Agent 在一轮处理中看到了多少条新消息
- LLM 思考和输出藏在两层折叠后面，需要多次点击才能看到
- 同一 LLM I/O 被 `LLMIOInline` 和 `ModelOutputInline` 两个组件重复请求

**本质问题**：当前设计用一个视图同时服务"沟通视角"（用户和 Agent 说了什么）和"决策视角"（Agent 为什么这样做），导致两边都做不好。

## What Changes

**信息架构重设计** — 三栏布局分离"沟通"与"决策"：
- 左栏：Session 列表（导航）
- 中栏：Conversation Timeline（纯净的对话流 + 压缩/吸纳事件作为一等时间线事件）
- 右栏：Decision Inspector（选中 exchange 的完整决策过程）

**后端补充**：
- 新增 `GET /api/agent-sessions/{id}/compactions` API，暴露压缩事件数据
- Trace JSONL 中增加 absorb round 和 compaction 事件类型，使 trace 数据能反映完整的处理周期

**前端重构**：
- 拆分 962 行的 `monitor-page.tsx` 为独立组件
- Decision Inspector 中 thinking 和 model output 默认展开
- 工具执行结果以清晰的卡片形式展示
- 统一 LLM I/O 缓存，消除重复请求
- 清理死代码（`logsApi`、`useRecentTraces`、`LogEntry`）

## Capabilities

### New Capabilities

- `monitor-three-panel-layout`: 三栏布局信息架构——Session List + Conversation Timeline + Decision Inspector
- `conversation-timeline`: 中栏对话时间线，纯净展示用户消息、Agent 回复、压缩事件、吸纳事件
- `decision-inspector`: 右栏决策检查器，展示选中 exchange 的 stats/thinking/output/tools/context 全过程
- `compaction-visibility`: 上下文压缩事件可见化——后端 API + 前端时间线事件 + Inspector 详情
- `absorb-round-visibility`: 吸纳轮次可见化——trace 中标记 absorb round，前端展示 Agent 在一个处理周期中看到的所有新消息
- `llm-io-dedup`: 统一 LLM I/O 数据请求，消除重复
- `dead-code-cleanup`: 清理未使用的 logsApi、useRecentTraces、LogEntry

### Modified Capabilities

## Impact

- **后端 API**：新增 1 个 HTTP 端点（compactions），修改 trace 事件格式（新增 absorb/compact 事件类型）
- **前端文件结构**：`monitor/` 目录从 1 个文件重构为三栏组件架构
- **前端依赖**：可能新增 `@tanstack/react-virtual`（如需虚拟化长对话）
- **hooks**：新增 `useSessionCompactions`、`useLLMIO`，修改 `use-monitor.ts`
- **清理**：删除 `admin/src/api/logs.ts`，清理 `types.ts` 中的 `LogEntry`，移除 `useRecentTraces`
