## Context

当前 agent 处理流程：用户消息 → dispatcher → runner 队列 → processOneEvent → engine.RunAgentLoop → 回复。所有处理都由外部消息触发，agent 没有自主发起动作的能力。

已有"准自主"机制的先例：subagent 完成后，通过 `EnqueueProcessRequest` 将通知作为新消息投递到 session 队列，触发主 agent 继续处理。定时任务可以复用完全相同的投递路径。

技术栈：Go + SQLite (modernc.org/sqlite) + 内存队列，单进程部署。

## Goals / Non-Goals

**Goals:**
- Agent 能在对话中识别并设定延时任务（如提醒、定时检查）
- 延时任务持久化到 SQLite，进程重启不丢失
- 到期任务自动以事件形式投递到对应 session，触发 agent 自主执行
- 到期事件格式与用户消息、subagent 通知明确区分，不产生模型歧义

**Non-Goals:**
- 不做 cron 式重复调度（只支持一次性延时任务）
- 不做跨实例分布式调度（当前单进程架构）
- 不做复杂的时区/自然语言时间解析（由模型负责转换为 ISO 8601）
- 不做任务优先级或依赖编排

## Decisions

### D1: 延时任务存储 — SQLite 新表 `delayed_tasks`

新增表结构：

```sql
CREATE TABLE IF NOT EXISTS delayed_tasks (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    channel TEXT NOT NULL DEFAULT '',
    channel_user_id TEXT NOT NULL DEFAULT '',
    channel_conversation_id TEXT NOT NULL DEFAULT '',
    task TEXT NOT NULL,
    execute_at TEXT NOT NULL,        -- ISO 8601 UTC
    status TEXT NOT NULL DEFAULT 'pending',  -- pending / dispatched
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_delayed_tasks_status_time ON delayed_tasks(status, execute_at);
```

**为什么用 SQLite 而非内存**：进程重启后任务不丢失。与现有数据层一致，无需引入新依赖。

**为什么存 channel 路由信息**：到期时需要还原完整的 `ProcessRequest`（包括 channel、channelUserId、channelConversationId），才能投递到正确的 session 并让 agent 知道向谁发消息。

### D2: 工具设计 — 仅 `set_delayed_task`，不设取消工具

**`set_delayed_task`**：
- 输入：`task` (string, 必填) + `execute_at` (string, 必填, ISO 8601)
- 由 processor.go 在 `processOneEvent` 中注册为 builtin tool，自动注入当前 session/agent/channel 上下文
- 返回：`{ taskId, task, executeAt, status: "scheduled" }`

**为什么不做 `cancel_delayed_task`**：取消是一个"预测未来"的动作——在设定和到期之间，用户意图可能反复变化。与其让模型在对话中途猜测是否该取消（还需要记住 taskId），不如让任务始终到期投递，由模型在到期时读取完整的对话上下文来判断是否仍需执行。这把"取消"从一个显式的工具调用变成了到期时自然的上下文理解，更符合对话的自然流，也消除了"忘记取消"或"误取消"的问题。

**为什么不做 `list_delayed_tasks`**：首版保持简单。agent 可以通过 recall_context 或对话记忆回溯自己设过什么任务。后续按需添加。

### D3: 调度器设计 — 轮询式后台协程

启动一个 `delayedTaskScheduler` 协程：
- 每 30 秒查询一次 `SELECT * FROM delayed_tasks WHERE status='pending' AND execute_at <= datetime('now')`
- 对每条到期任务：
  1. 将 status 更新为 `dispatched`
  2. 构造 `ProcessRequest` 并调用 `runner.EnqueueProcessRequest`
- 通过 `context.WithCancel` 控制生命周期，`main.go` shutdown 时取消

**为什么用轮询而非 time.AfterFunc**：
- 进程重启后 time.AfterFunc 丢失；轮询从 DB 恢复
- 任务可能在 agent 不同次运行中创建和到期
- 30s 间隔对"分钟级"延时任务场景足够；对需要秒级精度的场景不适用（non-goal）

**为什么 30s 间隔**：平衡及时性与 DB 负载。SQLite 单写者模式下，频繁扫描会增加锁竞争。30s 对提醒类场景延迟可接受。

### D4: 到期事件格式 — 结构化系统事件

到期任务进入 runner 队列时的 `Content` 格式：

```
【系统事件：定时任务到期】
task_id: {id}
创建时间: {created_at}

任务内容：
{task description}

这是你此前主动设定的定时任务，现已到期。
请结合当前对话上下文和用户现状，判断该任务是否仍然适用，然后采取相应行动。
如果情况已发生变化，请灵活调整执行方式或告知用户任务已到期但现状可能有变。
```

**设计考量**：

| 歧义风险 | 缓解措施 |
|---------|---------|
| 模型误认为是用户消息 | 用户消息格式为 `senderName: content`（纯文本，有发送者名称前缀）；系统事件使用 `【...】` 全角方括号前缀，两者在结构上天然区分 |
| 模型盲目执行过时任务 | 明确要求"结合当前对话上下文和用户现状，判断该任务是否仍然适用"——先判断再行动 |
| 模型不理解这是自己设的 | "你此前主动设定的定时任务"——强调主语和主动性 |
| 与 subagent 通知混淆 | subagent 用 `【subagent完成通知】`，定时任务用 `【系统事件：定时任务到期】`，前缀不同但同属 `【...】` 系统事件族，模型可统一识别为非用户消息 |

**投递时的 ProcessRequest 字段**：
- `Channel` / `ChannelUserID` / `ChannelConversationID`：从 `delayed_tasks` 表还原
- `SenderName`：空（系统事件，非用户发送）
- `MessageType`：`"text"`
- `MessageID`：`"delayed-task-{taskId}"` —— 方便去重和日志追踪

这个模式与 subagent 完成通知的投递方式完全一致（均通过 `EnqueueProcessRequest` 注入一条新消息），复用了已验证的消息合并和 session 路由逻辑。

### D5: System Prompt 增强 — 后续任务发现指引

在 agent system prompt 中增加一段"主动能力"指引（作为 DB seed 或文档建议）：

```
## 主动能力

你具备设定定时任务的能力。在每轮对话中，注意识别以下场景：
- 用户明确要求在某个时间点提醒或执行某事
- 对话中隐含需要后续跟进的事项（如"明天记得..."、"下周一之前..."）
- 你判断某个承诺需要定时追踪以确保完成

当识别到此类场景时，使用 set_delayed_task 工具设定延时任务，并告知用户你已安排。

收到【系统事件：定时任务到期】消息时，这是你此前设定的定时任务已到期。
请先回顾当前对话上下文，判断该任务是否仍然适用：
- 如果任务仍然有效，主动采取行动（如发送提醒消息、执行检查等），无需等待用户指令。
- 如果用户的意图或情况已发生变化，灵活调整执行方式，或告知用户任务已到期并说明你观察到的变化。
```

**为什么放在 prompt 而非硬编码**：prompt 存在 DB 中可由管理员编辑，不同 agent 可以有不同的主动策略。此处只提供建议文案，由实施者添加到对应 agent_config。

## Risks / Trade-offs

| 风险 | 缓解 |
|------|------|
| 轮询延迟最大 30s | 对提醒类场景可接受；如需更高精度后续可缩短间隔或改用 time.AfterFunc 缓存 |
| 模型滥用 set_delayed_task | 通过 prompt 约束使用场景；后续可增加每 session 任务数上限 |
| 进程长期运行后大量已完成任务占 DB | 可定期清理 `status='dispatched'` 且 `updated_at` 超过 7 天的记录（不在本次实现） |
| session 在任务到期前被 evict | 到期投递时 `EnqueueProcessRequest` 会自动创建或恢复 SessionWorker，不影响执行 |
| 到期时原始 session 上下文已压缩或丢失 | 任务描述本身包含完整意图，不依赖历史上下文即可执行 |

## Open Questions

- 是否需要在 admin UI 中展示/管理延时任务列表？（建议后续迭代）
