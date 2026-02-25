# 决策记录 003：Agent-Server 边界重构

| 项目 | 内容 |
|------|------|
| 日期 | 2026-02-09 |
| 状态 | 已实施，待集成测试 |
| 类型 | 架构重构 |
| 决策者 | guomiao |
| 设计文档 | [AGENT_REFACTOR_DESIGN.md](../AGENT_REFACTOR_DESIGN.md) |

---

## 背景

系统目前有两个核心服务：`agent`（端口 1996）和 `server`（端口 1997）。两者的职责边界不清晰，导致：

1. **agent 的"灵魂"被撕成两半**：Server 持有记忆加载、指令构建、会话策略、流事件处理等 agent 认知逻辑（`channel-router.ts` 627 行），orchestrator 只是个 SDK wrapper
2. **server 不像 server**：它做了太多"agent 怎么思考"的事
3. **agent 不像 agent**：它对"我是谁、我记得什么、我该怎么回"一无所知，全靠 server 喂
4. **无法安全演进**：agent 行为分布在两个进程中，改一个功能要同时改两处

## 讨论过程

### 第一轮：识别问题

分析现有代码发现 `channel-router.ts` 中约 70% 的逻辑属于 "agent 的认知过程"：

- `loadMemoryContext()` — 加载用户记忆
- `buildInstructionWithMemory()` — 拼接对话历史、记忆、渠道上下文
- `buildChannelContext()` — 构建渠道信息
- 流事件处理（tool_call 状态机、content 累积、observation 发射）
- 会话创建/复用策略

这些逻辑不应该在"邮局"（server）里，应该在"人"（agent）自己那里。

### 第二轮：确定方向

**提出的方向**：
- Agent 应该是完整独立的应用，包含所有业务逻辑
- Server 退化为薄适配层（数据存储 + 渠道收发）
- Agent 支持滚动更新（类 k8s 策略）

**判断标准**：
> "这个逻辑是关于 agent 怎么思考的，还是关于消息怎么送达的？"

前者属于 Agent，后者属于 Server。

### 第三轮：初版方案（被修正）

初版设计了一套自建方案：
- 自建 `agent-loop.ts`（agent 循环）
- 自建 `checkpoint.ts`（执行检查点，含 phase 状态机）
- 自建 `session-strategy.ts`（会话管理策略）
- 自建 `prompt-builder.ts`、`memory-loader.ts`（上下文构建）

这套方案预估 ~8.5 天，800+ 行新代码。

### 第四轮：SDK-First 修正（最终方案）

关键反馈：**agent 的核心应该围绕 Claude Agent SDK 来构建，只有 SDK 没有的能力才自建。**

重新审视 SDK 能力后发现：

| SDK 已提供 | 之前方案多余的自建 |
|-----------|----------------|
| Agent loop (`query()` / `session.send()+stream()`) | ~~agent-loop.ts~~ |
| 工具执行 (`claude_code` preset) | ~~tools/ 目录~~ |
| 会话管理 (`createSession` / `resumeSession`) | ~~session-strategy.ts~~ |
| 断点续传 (JSONL 磁盘存储 + `resumeSession`) | ~~checkpoint.ts + 状态机~~ |
| 多轮对话 (V2 `send()`/`stream()`) | ~~历史拼接逻辑~~ |

修正后 Agent App 只需要 ~200 行核心代码，预估 5.5 天。

### 关于断点续传的讨论

核心发现：**SDK 的 session 状态存在磁盘（`~/.claude/projects/{slug}/{session-id}.jsonl`），`resumeSession()` 可以跨进程恢复。**

这意味着：
- 不需要自建 checkpoint 系统
- Agent 进程挂了 → 新进程 `resumeSession(sdkSessionId)` → SDK 从磁盘恢复
- `agent_sessions.sdk_session_id` 字段是断点续传的唯一需要持久化的状态
- Server 通过超时检测将 `execution_status` 从 `processing` 改为 `interrupted`

### 关于自定义工具的讨论

SDK preset 不含渠道发消息能力。三个选项：

1. **systemPrompt + Bash curl**（当前最实际）— agent 通过 Bash 工具 curl 调 server API
2. **MCP Server**（更优雅，等 SDK 稳定后）— 原生工具集成
3. **skill-loader 注入**（当前代码已有）— 本质同 1

选择选项 1，因为零自建代码、SDK 原生支持 Bash。

## 最终决策

### 1. Agent App = SDK 薄封装

Agent App 的核心就一个函数：
1. 从 Server 拉记忆 + 配置
2. 拼入 systemPrompt
3. 调 SDK `createSession`/`resumeSession`
4. 流式转发事件
5. 回写 Server

### 2. Server = 数据 + 渠道适配

Server 只做：去重 → 用户解析 → 存消息 → 派发给 Agent endpoint。
`channel-router.ts`（627行）→ `channel-dispatcher.ts`（~50行）。

### 3. 断点续传 = SDK 原生 + DB 一个字段

- `agent_sessions.sdk_session_id` — 持久化 SDK 会话 ID
- `agent_sessions.execution_status` — idle/processing/interrupted/completed
- 恢复靠 `resumeSession(sdkSessionId)`

### 4. 滚动更新 = 蓝绿部署

- `agent_registry` 表管理 Agent 实例
- 新版本 ready → 切流量 → 旧版本 drain → 退出
- 新旧进程共享文件系统，SDK JSONL 天然跨进程

## 影响范围

### 新增
- `agent/` — 新目录，~200 行核心代码
- `server/src/routes/data.ts` — Data API 路由
- `server/src/services/channel-dispatcher.ts` — 替换 channel-router
- `server/src/services/agent-registry.ts` — Agent 实例注册表
- `agent_registry` 表 — 数据库 schema

### 修改
- `agent_sessions` 表 — 新增 `execution_status` 字段

### 删除/废弃
- `agent/` — 被 `agent/` 替代
- `server/src/services/channel-router.ts` — 被 channel-dispatcher 替代
- `server/src/services/orchestrator-client.ts` — 不再需要
- `server/src/services/memory-manager.ts` — prompt 构建逻辑移走，CRUD 保留

## 风险

1. **SDK V2 尚不稳定**：`unstable_v2_createSession` 前缀说明 API 可能变化。可先用 V1 的 `query({ resume })` 方案兜底
2. **SDK resume bug**：GitHub Issue #2778 报告 TypeScript SDK 的 resume 参数曾被忽略。需要在 Phase 4 充分验证
3. **非 Anthropic 模型兼容**：SDK 通过 api-proxy 支持 OpenAI 格式模型，proxy 需要一并迁入 agent
