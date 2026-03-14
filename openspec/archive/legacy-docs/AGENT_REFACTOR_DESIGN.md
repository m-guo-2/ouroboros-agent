# Agent 架构重构设计：自定义 ReAct 引擎与读写分离

| 项目 | 内容 |
|------|------|
| 日期 | 2026-02-25 |
| 状态 | 持续演进中（已完成自定义 ReAct 引擎，正在实施 Context 读写分离） |
| 关联决策 | [008-react-agent-engine](./decisions/008-react-agent-engine.md), [011-session-context-and-logging-refactor](./decisions/011-session-context-and-logging-refactor.md) |

---

## 1. 设计目标

将 Agent 和 Server 的职责清晰划分，并彻底解决大模型上下文管理中的“失忆”、“反向幻觉”与“数据泥潭”问题。

- **Server (基础设施层)**：提供数据存储、渠道收发、用户解析、统一 API。
- **Agent (应用层)**：基于纯手写的 ReAct 引擎，负责思考、工具调用与上下文管理。
- **事件驱动 (Event-Driven)**：Agent 不再仅仅是对“用户消息”做出响应的 Chatbot，而是响应各类“事件”的自主智能体。
- **读写分离 (CQRS 思想)**：将大模型的**记忆上下文 (Model Context)** 与发给用户的**展示消息 (UI Messages)** 彻底分离。
- **高可观测性**：将大体积的执行轨迹 (Execution Trace) 下沉到文件系统，提供极佳的调试体验且不拖垮数据库。

---

## 2. 架构总览

```text
┌─────────────────────────────────────────────────────────────┐
│  Server (基础设施层 / Port 1997)                              │
│                                                             │
│  ├─ 渠道适配器 (Feishu / Qiwei / WebUI)                       │
│  ├─ 事件分发器 (派发用户消息 / 系统事件 / 定时任务)             │
│  ├─ Data API (供 Agent 调用的 CRUD 接口)                      │
│  ├─ 数据库 (SQLite)                                          │
│  │   ├─ agent_sessions (含 session_context JSON)            │
│  │   └─ messages (仅存纯净的 Chat UI 对话)                    │
│  └─ Trace 日志系统 (写本地 .jsonl 文件)                       │
└──────────────┬──────────────────────────────────────────────┘
               │  HTTP (事件派发 / 数据读写)
┌──────────────▼──────────────────────────────────────────────┐
│  Agent (应用层 / Port 1996)                                   │
│                                                             │
│  ├─ 接收事件 (POST /process-event)                            │
│  ├─ 上下文组装 (加载 Session Context + 转换 Event 格式)       │
│  ├─ ReAct 引擎 (loop.ts: 纯手写 while 循环)                   │
│  │   ├─ LLM Client (Anthropic / OpenAI 兼容)                │
│  │   └─ Tool Registry (内置工具 / Skills / MCP)             │
│  ├─ 强制工具回复 (必须调用 send_channel_message 才能发消息)   │
│  └─ 状态回写 (全量覆盖 Session Context，流式上报 Trace)       │
└─────────────────────────────────────────────────────────────┘
```

---

## 3. 核心设计范式

### 3.1 事件驱动架构 (Event-Driven Agent)
系统从传统的“Request-Response Chatbot”正式蜕变为**事件驱动的自主智能体**。驱动 Agent 运转的入口不再局限于“用户发送了消息 (`content`)”，而是被统一抽象为 **`Event` (事件)**。

- **泛化的触发源**：
  - `EventType.USER_MESSAGE`：用户发送了聊天消息。
  - `EventType.GROUP_JOINED`：新群建立，或 Agent 被邀请入群（Agent 可主动打招呼）。
  - `EventType.ASYNC_TASK_COMPLETED`：长耗时异步工具执行完毕，系统通知 Agent 处理结果。
  - `EventType.CRON_TICK`：定时任务触发（如每天早上 9 点播报数据）。
- **统一的 LLM 消费格式**：
  无论是用户消息还是系统事件，在存入大模型的 `session_context`（记忆）时，统一转化为大模型能理解的输入格式（通常放置在 `role: "user"` 中，通过 `[System Event: XXX]` 标签进行格式化区分）。
- **流水线一致性**：对 Agent Runner 来说，不再区分消息和通知。工作流统一为：`唤醒 Session` -> `加载 Context` -> `追加 Event 到 Context` -> `触发 ReAct Loop` -> `保存 Context 并休眠`。

### 3.2 自定义 ReAct 引擎 (取代 SDK)
我们抛弃了黑盒的 `@anthropic-ai/claude-agent-sdk`，转而使用纯手写的 `while` 循环 (`loop.ts`)。
- **100% 透明**：每一步 Thought (思考)、Action (工具调用)、Observation (工具结果) 都通过事件流式上报。
- **无副作用**：引擎本身不持有状态，每次调用都是纯函数，方便测试与状态恢复。

### 3.3 强制工具回复模式 (Tool-based Communication)
为了彻底解决大模型的“反向幻觉”（即模型以为自己心里的想法已经被用户看到了）：
1. **Text = Internal Thought**：模型直接输出的纯文本，定义为绝对私密的内部日志，用于思维链 (CoT)，用户永远不可见。
2. **Tool = Action & Communication**：模型必须且只能调用 `send_channel_message` 工具才能将信息发送给用户。
3. **Loop End = No Action**：当模型认为任务完成，只需输出一段内部思考（不附带工具调用），ReAct Loop 就会自然结束。

### 3.4 上下文与日志的读写分离
过去的架构试图用一张 `messages` 表同时满足 UI 展示和 LLM 上下文，导致了大量的补丁代码（如 `ensureAlternation`、`removeOrphanedToolUses`）。现在我们将它们彻底分离：

1. **Model Context (`session_context`)**：
   - 作为大模型的“脑子”。
   - 存储原汁原味、未经任何删改的完整 `AgentMessage[]` 数组（包含 thinking 和 tool_use）。
   - 每次 ReAct Loop 结束后，整体 JSON 覆盖回写。截断时以“完整回合 (Turn)”为原子单位。
2. **Chat UI Messages (`messages` 表)**：
   - 作为 C 端的展示板。
   - 只存客观发生、用户可见的对话（人类提问与 Agent 通过工具发出的显式回复）。
3. **Execution Trace Logs (`.jsonl` 文件)**：
   - 作为开发者/后台的调试与时间线展示。
   - 将高频、大体积的思考和工具调用结果直接追加写入磁盘文件，避免 SQLite 数据库膨胀和锁竞争。

---

## 4. 核心模块与数据流

### 4.1 Agent App 目录结构

```text
agent/src/
├── index.ts                    # Express server，端口 1996
├── routes/
│   ├── process.ts              # POST /process-event — 接收 Server 派发的事件
│   └── health.ts               # 探针与生命周期管理
├── engine/                     # ReAct 引擎核心
│   ├── loop.ts                 # 核心 while 循环
│   ├── runner.ts               # 集成层：加载配置、组装上下文、运行 Loop
│   ├── tool-registry.ts        # 工具注册中心
│   ├── llm-client.ts           # LLM 客户端封装
│   └── types.ts                # 核心类型定义
└── services/
    ├── context-composer.ts     # 组装 System Prompt 与 Event 格式化
    └── server-client.ts        # 调 Server Data API 的 HTTP 客户端
```

### 4.2 一次完整的事件处理流程 (Runner)

1. **接收事件**：`runner.ts` 接收到 `AgentEvent`（可能是用户发消息，也可能是异步任务完成通知）。
2. **加载配置**：从 Server 获取 Agent 配置、模型凭据、Skills。
3. **加载记忆**：直接从 `session_context` 加载 JSON，反序列化为 `AgentMessage[]`（无需复杂的清洗管道）。
4. **追加事件输入**：将当前的 `AgentEvent` 格式化后追加到上下文中。
5. **运行 Loop**：
   - 调用 LLM。
   - 记录 Thinking 到 Trace 日志。
   - 执行 Tool Calls（如查数据库、调用外部 API）。
   - 如果调用了 `send_channel_message`，通过 Server API 实际发送消息，并在 `messages` 表中记录一条 UI 消息。
   - 将 Tool Results 追加到上下文。
   - 循环直到 LLM 不再输出 Tool Calls。
6. **保存记忆**：Loop 结束后，将最终的 `AgentMessage[]` 数组 `JSON.stringify` 存回 `session_context`。

---

## 5. 实施路径与演进

| 阶段 | 内容 | 状态 |
|------|------|------|
| Phase 1 | 剥离 SDK，实现自定义 ReAct 引擎 (`loop.ts`, `runner.ts`) | ✅ 已完成 |
| Phase 2 | 实现 Execution Trace 实时上报与 SQLite 存储 | ✅ 已完成 |
| Phase 3 | 事件驱动重构：引入 `Event` 概念，统一触发入口 | 🚧 进行中 |
| Phase 4 | 存储层重构：新增 `session_context`，精简 `messages` 表 | 🚧 进行中 |
| Phase 5 | 引擎层重构：移除历史清洗代码，保留完整 Thinking | 🚧 进行中 |
| Phase 6 | 日志层重构：Trace 数据下沉至 `.jsonl` 文件系统 | 📅 计划中 |

## 6. 总结

这套新架构通过**“事件驱动”**、**“读写分离”**和**“强制工具回复”**，彻底实现了 Agent 系统从底层数据到顶层概念的全面升维。它不仅解决了历史遗留的代码恶心、模型失忆、反向幻觉等问题，更重要的是打破了“聊天机器人”的桎梏，赋予了系统自主响应复杂场景、异步任务、甚至主动发起会话的能力，为后续超复杂工作流的实现铺平了道路。