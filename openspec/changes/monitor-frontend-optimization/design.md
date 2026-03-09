## Context

### Agent 运行模型

一个 `processOneEvent` 执行周期现在包含 absorb-replan 循环：

```
processOneEvent(request)
  ├─ 加载配置、构建 LLM client、注册 tools（一次性）
  ├─ 加载历史、格式化用户消息
  └─ for absorbRound := 0; ; absorbRound++ {
       ├─ Execute:  RunAgentLoop → FinalText → 追加 assistant message
       ├─ Checkpoint: EstimateTokens → ShouldCompact? → CompactContext → UpdateSession
       └─ Absorb-or-Exit: popAllPending → 有新消息? → 合并到 messages → continue
                                          无新消息? → break
     }
```

关键特征：
- 单个处理周期可能产生多轮 RunAgentLoop（最多 5 轮 absorb）
- 每轮之间可能触发 context compaction
- 被吸纳的消息以 `[以下 N 条消息在处理期间到达]` 格式合并
- 所有轮次共享同一个 traceID（第一条消息的 traceID）

### 压缩数据

`context_compactions` 表已存储：summary、archivedMessageCount、tokenCountBefore/After、compactModel、createdAt。有 `ListCompactions(sessionID)` 查询，但无 HTTP API。

### 当前 Monitor UI

962 行单文件，两栏布局（session list + exchange list）。Exchange = user → collapsed trace → assistant。Trace 藏在两层折叠中。压缩和吸纳不可见。

## Goals / Non-Goals

**Goals:**

- 将"沟通"和"决策"分离为两个独立视觉通道（三栏布局）
- 上下文压缩作为对话时间线中的一等事件
- 吸纳轮次（absorb round）在 trace 中可见
- LLM 思考和工具执行结果在 Decision Inspector 中**默认可见**，不需要层层点击
- 暴露 compaction 数据的 HTTP API
- 统一 LLM I/O 缓存消除重复请求
- 拆分单文件为可维护的组件模块

**Non-Goals:**

- 不做 trace-centric 独立视图（保持 session-centric）
- 不做 session 服务端分页
- 不做 i18n
- 不新增 logs 查看功能
- 不修改 `RunAgentLoop` 内部逻辑

## Decisions

### 1. 三栏布局：Session List + Conversation Timeline + Decision Inspector

```
┌─────────┬──────────────────────┬─────────────────────────────┐
│ Session │   Conversation       │   Decision Inspector        │
│  List   │   Timeline           │                             │
│  (240px)│   (flex)             │   (flex, 可折叠)             │
│         │                      │                             │
│  搜索    │  👤 用户消息          │   📊 Stats                  │
│  过滤    │  🤖 Agent 回复        │   🧠 Thinking               │
│  列表    │  ⚡ 压缩事件          │   📝 Model Output           │
│         │  📨 吸纳事件          │   🔧 Tools                  │
│         │  👤 用户消息          │   📦 Compaction             │
│         │  🤖 Agent 回复        │   📄 Raw LLM I/O           │
└─────────┴──────────────────────┴─────────────────────────────┘
```

**替代方案**：保持两栏，在 exchange 内嵌详情。
**放弃原因**：无法分离沟通和决策两个信息通道。用户要么看对话流被 trace 打断，要么看 trace 时缺少对话上下文。

**右栏可折叠**：默认展开；当用户只想扫对话流时，可以折叠右栏让中栏占满。

### 2. Conversation Timeline 数据模型

Timeline 展示以下事件类型，按时间排序：

| 事件类型 | 数据来源 | 展示形式 |
|---------|---------|---------|
| 用户消息 | `messages` (role=user) | 对话气泡 |
| Agent 回复 | `messages` (role=assistant, text) | 对话气泡 + markdown |
| 处理中 | session.executionStatus === "processing" | 动画指示器 + 简要统计 |
| 上下文压缩 | `compactions` API | 时间线标记：归档数/token 变化 |
| 消息吸纳 | trace 中的 absorb 事件 | 时间线标记：N 条新消息被吸纳 |

点击任何一个"Agent 回复"或"处理中"事件，右栏 Decision Inspector 展示对应 trace 的完整决策过程。

**关键设计**：Timeline 只展示"发生了什么"的概要，不展示 trace 细节。保持干净可扫读。

### 3. Decision Inspector 信息层次

选中一个 exchange 后，Inspector 展示：

**Stats Bar**（顶部，始终可见）：
- 总耗时 | 总 tokens (in/out) | 总成本 | LLM 调用数 | 迭代数
- 如果有 absorb，显示 "N 轮处理" 标签

**Absorb Round Tabs**（如果有多轮）：
- 每轮一个 tab："Round 1"、"Round 2 (+3 条新消息)"
- 每个 tab 内是该轮的完整决策过程

**每轮内容**（按执行顺序展示，thinking 和 tools 默认展开）：
1. **Thinking blocks**：LLM 的思考过程，完整展示
2. **Model Output**：模型原始输出文本（从 LLM I/O response 中提取）
3. **Tool Calls**：每个工具一个卡片，input + result 并排
4. **Errors**：错误信息
5. **Compaction**（如果本轮触发了压缩）：token 变化、摘要

**替代方案**：用手风琴(accordion)代替 tabs。
**选择 tabs 的原因**：absorb round 之间是独立的 agent 执行周期，tab 语义更清晰。accordion 暗示同一个流程的不同部分，与实际语义不符。

### 4. 后端：Compaction API

新增端点：

```
GET /api/agent-sessions/{id}/compactions
→ [{ id, summary, archivedMessageCount, tokenCountBefore, tokenCountAfter, compactModel, createdAt }]
```

实现：调用已有的 `storage.ListCompactions(sessionID)`，包装为 HTTP JSON 响应。约 20 行代码。

### 5. 后端：Trace 中的 Absorb 和 Compact 事件

在 `processOneEvent` 的 absorb 和 checkpoint 阶段增加 business 日志，带 `traceEvent` 字段：

```go
// Absorb
logger.Business(ctx, "消息吸纳", "traceEvent", "absorb",
    "absorbRound", absorbRound, "absorbedCount", len(pending))

// Compact
logger.Business(ctx, "上下文压缩", "traceEvent", "compact",
    "tokensBefore", result.TokensBefore, "tokensAfter", result.TokensAfter,
    "archivedCount", result.ArchivedCount)
```

`traces.go` 的 `buildTrace` 增加对这两个事件类型的处理，生成新的 step 类型：

```typescript
// 新增 step.type
type: "absorb"   // absorbRound, absorbedCount
type: "compact"  // tokensBefore, tokensAfter, archivedCount, summary
```

### 6. 前端组件架构

```
monitor/
├── monitor-page.tsx              # 三栏布局 + 顶层状态
├── components/
│   ├── session-list.tsx          # 左栏：session 列表 + 搜索过滤
│   ├── conversation-timeline.tsx # 中栏：对话时间线
│   ├── timeline-event.tsx        # 时间线中的单个事件（消息/压缩/吸纳）
│   ├── decision-inspector.tsx    # 右栏：决策检查器
│   ├── round-detail.tsx          # Inspector 中单轮的详情
│   ├── thinking-view.tsx         # 思考步骤展示
│   ├── tool-card.tsx             # 工具调用卡片（input + result）
│   ├── model-output-view.tsx     # 模型输出展示
│   ├── trace-stats-bar.tsx       # 统计汇总条
│   ├── compaction-event.tsx      # 压缩事件展示
│   ├── llm-io-viewer.tsx         # LLM I/O 弹窗
│   └── exchange-skeleton.tsx     # 加载骨架屏
├── hooks/
│   ├── use-llm-io.ts            # 统一的 LLM I/O hook
│   └── use-session-compactions.ts # 压缩数据 hook
└── lib/
    ├── build-timeline.ts         # 构建时间线事件（合并 messages + compactions + trace events）
    └── types.ts                  # Monitor 局部类型
```

### 7. 时间线事件构建逻辑

```typescript
type TimelineEvent =
  | { type: "user-message"; message: MessageData; exchangeIndex: number }
  | { type: "assistant-message"; message: MessageData; traceId?: string; exchangeIndex: number }
  | { type: "processing"; traceId: string; exchangeIndex: number }
  | { type: "compaction"; data: CompactionData }
  | { type: "absorb"; round: number; count: number; traceId: string }

function buildTimeline(
  messages: MessageData[],
  compactions: CompactionData[],
  traces: Record<string, ExecutionTrace>,
): TimelineEvent[]
```

Messages 和 compactions 按 `createdAt` 时间排序后交错合并。吸纳事件从 trace steps 中提取（type === "absorb"）。

### 8. LLM I/O 去重

抽取 `useLLMIO(traceId, ref)` hook：

```typescript
function useLLMIO(traceId: string, ref: string) {
  return useQuery({
    queryKey: ["llm-io", traceId, ref],
    queryFn: () => tracesApi.getLLMIO(traceId, ref),
    staleTime: Infinity,
    enabled: !!traceId && !!ref,
  })
}
```

`ThinkingView`、`ModelOutputView`、`LLMIOViewer` 全部通过此 hook 获取数据，React Query 自动去重。

## Risks / Trade-offs

- **[三栏在窄屏幕上的适配]** → Inspector 可折叠；中等屏幕下默认折叠，点击 exchange 时展开。极窄屏幕下退化为两栏（session list 变为抽屉）。

- **[Compaction API 新增后端工作]** → 约 20 行 Go 代码，复用已有 storage 函数。风险极低。

- **[Trace 事件类型新增的向后兼容]** → 新增的 "absorb" 和 "compact" step 类型。旧 JSONL 中不存在这些事件，前端需兼容缺失的情况。使用 optional 字段处理。

- **[Timeline 排序精度]** → Messages 和 compactions 的时间戳可能精度不同（messages 是毫秒级 SQLite timestamp，compactions 是 RFC3339 字符串）。统一转为 Unix ms 后排序。

- **[多轮 absorb 的 tab 体验]** → 大多数情况下只有 1 轮（无 absorb）。单轮时不显示 tabs，直接平铺内容。只有多轮时才出现 tab 切换。
