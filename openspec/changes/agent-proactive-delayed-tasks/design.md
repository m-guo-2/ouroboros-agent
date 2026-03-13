## Context

当前 agent 处理流程：用户消息 → dispatcher → runner 队列 → processOneEvent → engine.RunAgentLoop → 回复。所有处理都由外部消息触发，agent 没有自主发起动作的能力。

已有"准自主"机制的先例：subagent 完成后，通过 `EnqueueProcessRequest` 将通知作为新消息投递到 session 队列，触发主 agent 继续处理。定时任务可以复用完全相同的投递路径。

技术栈：Go + SQLite (modernc.org/sqlite) + 内存队列，单进程部署。

## Goals / Non-Goals

**Goals:**
- Agent 能在对话中识别并设定延时任务（如提醒、定时检查）
- 延时任务持久化到 SQLite，进程重启不丢失
- 到期任务自动以事件形式投递到对应 session，触发 agent 自主执行
- 到期事件提供完整时间线（创建时间、计划执行时间、实际触发时间），支持 agent 做时效性判断
- Agent 可取消和查询已设定的待执行任务
- Agent 可通过自我续期实现周期性关注类任务

**Non-Goals:**
- 不做 cron 式重复调度（通过自我续期模式在应用层实现周期性）
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
    execute_at TEXT NOT NULL,        -- SQLite datetime 格式 (UTC)，写入时归一化
    status TEXT NOT NULL DEFAULT 'pending',  -- pending / dispatched / cancelled
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_delayed_tasks_status_time ON delayed_tasks(status, execute_at);
```

**execute_at 格式归一化**：LLM 传入 ISO 8601（如 `2025-03-12T10:00:00+08:00`），`CreateDelayedTask` 在写入 DB 前将其解析并转换为 SQLite datetime 格式的 UTC 时间（如 `2025-03-12 02:00:00`）。这确保 `execute_at <= datetime('now')` 的字符串比较在 SQLite 中语义正确，同时索引可正常工作。

**status 合法值**：`pending`（待执行）、`dispatched`（已投递）、`cancelled`（已取消）。

**为什么用 SQLite 而非内存**：进程重启后任务不丢失。与现有数据层一致，无需引入新依赖。

**为什么存 channel 路由信息**：到期时需要还原完整的 `ProcessRequest`（包括 channel、channelUserId、channelConversationId），才能投递到正确的 session 并让 agent 知道向谁发消息。

### D2: 工具设计 — 设定、取消、查询三件套

**`set_delayed_task`**：
- 输入：`task` (string, 必填) + `execute_at` (string, 必填, ISO 8601)
- 由 processor.go 在 `processOneEvent` 中注册为 builtin tool，自动注入当前 session/agent/channel 上下文
- 写入时 `execute_at` 自动归一化为 SQLite UTC 格式
- 返回：`{ taskId, task, executeAt, status: "scheduled" }`

**`cancel_delayed_task`**：
- 输入：`task_id` (string, 必填)
- 将指定任务的 status 从 `pending` 更新为 `cancelled`
- 限定 session 范围，防止跨 session 误取消
- 返回：`{ taskId, status: "cancelled" }`

**`list_delayed_tasks`**：
- 无输入参数
- 查询当前 session 中所有 status 为 `pending` 的任务
- 返回：`{ count, tasks: [{ taskId, task, executeAt, createdAt }] }`

**为什么增加取消和查询工具**：原设计依赖"到期时上下文判断"作为取消机制，但存在三个问题：(1) 上下文窗口有限，取消意图可能被截断；(2) 即使最终不执行，整条链路（调度器投递 → agent loop → LLM 调用）的 token 和算力白白消耗；(3) 对于有副作用的任务（如发消息），误判风险不可接受。显式取消是主路径，到期时的上下文判断作为兜底。

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

### D4: 到期事件格式 — 纯结构化数据，行为指导归 prompt

到期任务进入 runner 队列时的 `Content` 格式：

```
【系统事件：定时任务到期】
task_id: {id}
创建时间: {created_at}
计划执行时间: {execute_at}
实际触发时间: {now}
任务内容：{task description}
```

**三个时间点的作用**：

| 时间 | 来源 | 作用 |
|------|------|------|
| 创建时间 | delayed_tasks.created_at | agent 知道"我当时为什么设这个任务" |
| 计划执行时间 | delayed_tasks.execute_at | agent 知道"我认为什么时候该做" |
| 实际触发时间 | 调度器投递时 time.Now() | agent 知道"现在实际是什么时候"，可判断是否延迟 |

**设计原则**：**数据归 event，行为归 system prompt**。event 只传递结构化数据，不包含行为指令（如"请判断…"）。行为指导统一在 system prompt 中定义，避免每次事件重复浪费 token。

**投递时的 ProcessRequest 字段**：
- `Channel` / `ChannelUserID` / `ChannelConversationID`：从 `delayed_tasks` 表还原
- `SenderName`：空（系统事件，非用户发送）
- `MessageType`：`"text"`
- `MessageID`：`"delayed-task-{taskId}"` —— 方便去重和日志追踪

这个模式与 subagent 完成通知的投递方式完全一致（均通过 `EnqueueProcessRequest` 注入一条新消息），复用了已验证的消息合并和 session 路由逻辑。

### D5: System Prompt 增强 — 四段式主动能力指引

在 agent system prompt 中增加"主动能力"指引，包含四个子章节：

1. **设定任务**：识别场景（明确时间、模糊时间、隐含跟进），使用 `set_delayed_task`
2. **取消任务**：用户明确取消时使用 `cancel_delayed_task`，不确定 ID 时先 `list_delayed_tasks`
3. **任务到期处理**：基于三时间点判断（准时执行、延迟补救、已过期作废）
4. **自我续期**：持续关注类事项到期后可再次 `set_delayed_task`，形成周期性跟进

**为什么放在 prompt 而非硬编码**：prompt 存在 DB 中可由管理员编辑，不同 agent 可以有不同的主动策略。

## Risks / Trade-offs

| 风险 | 缓解 |
|------|------|
| 轮询延迟最大 30s | 对提醒类场景可接受；如需更高精度后续可缩短间隔或改用 time.AfterFunc 缓存 |
| 模型滥用 set_delayed_task | 通过 prompt 约束使用场景；后续可增加每 session 任务数上限 |
| 进程长期运行后大量已完成/已取消任务占 DB | 可定期清理 `status IN ('dispatched','cancelled')` 且 `updated_at` 超过 7 天的记录（不在本次实现） |
| session 在任务到期前被 evict | 到期投递时 `EnqueueProcessRequest` 会自动创建或恢复 SessionWorker，不影响执行 |
| 到期时原始 session 上下文已压缩或丢失 | 任务描述本身包含完整意图，不依赖历史上下文即可执行 |
| ISO 8601 格式与 SQLite datetime 不兼容导致查询失败 | 写入时归一化为 SQLite UTC 格式，从根源消除格式不一致 |

## Open Questions

- 是否需要在 admin UI 中展示/管理延时任务列表？（建议后续迭代）
- 是否需要限制每 session 的 pending 任务数上限？（观察使用情况后决定）
