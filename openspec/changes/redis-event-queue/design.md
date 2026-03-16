## Context

当前 agent 的消息处理架构：

- **入口**：HTTP `POST /api/channels/incoming` → `dispatcher.Dispatch`（goroutine）→ `runner.EnqueueProcessRequest`
- **队列**：进程内 `map[string]*SessionWorker`，每个 worker 持有 `[]QueuedRequest` 切片
- **消费**：`drainWorker` 逐条出队 → `processOneEvent` → `engine.RunAgentLoop`
- **吸纳**：`processOneEvent` 内的 absorb 循环，在 engine 完成一整轮后才检查新消息
- **存储**：SQLite（会话、消息、去重）
- **部署**：单实例

核心问题：LLM 在做决策和执行 tool 期间看不到新到达的用户消息，可能基于过时信息产生副作用。进程崩溃后，内存中的队列和消费进度丢失。

## Goals / Non-Goals

**Goals:**
- LLM 每次被调用前都能看到当前所有已知的外部事件（完整世界视图）
- Tool 执行前若有新事件到达，放弃执行并重新决策（防止基于过时信息产生副作用）
- Event 到来即存库，消费进度（cursor）持久化，进程崩溃后可恢复
- 零新依赖，纯 DB 方案

**Non-Goals:**
- 不引入外部依赖
- 不支持多实例部署（后续可加）
- 不改变外部 HTTP API 接口
- 不实现 tool 级别的细粒度事件相关性判断（v1 采用全量放弃策略）

## Decisions

### Decision 1: session_events 表作为事件索引

**选择**：新增 `session_events` 表，`seq INTEGER PRIMARY KEY AUTOINCREMENT`，记录每个外部事件的到达顺序。实际消息内容在已有的 `messages` 表中，`session_events` 只存 `session_id` + `message_id` 引用。

**替代方案**：
- 直接在 messages 表加 `source` 列区分外部/内部消息：messages 表是通用对话记录，混入事件跟踪语义会模糊职责
- 用 messages 表的 `rowid` 做 cursor：rowid 是 SQLite 实现细节，不是显式 schema，语义不够清晰

**理由**：AUTOINCREMENT 的 `seq` 提供全局单调递增的事件序号，cursor 语义天然清晰。表结构极其轻量（三列），只做索引不做存储，职责分明。

### Decision 2: EventLog 消费模型（cursor 查 DB，批量全取）

**选择**：`EventLog` 结构封装对 `session_events` + `messages` 的消费操作：
- `DrainNew()` — `SELECT ... FROM session_events se JOIN messages m ON se.message_id = m.id WHERE se.session_id = ? AND se.seq > ? ORDER BY se.seq`，一次性返回所有未消费事件
- `HasNew()` — `SELECT EXISTS(SELECT 1 FROM session_events WHERE session_id = ? AND seq > ?)`

**关键语义**：`DrainNew` 每次调用必须返回当前所有未消费事件，不逐条消费。Agent 在每个决策点需要看到完整的世界状态。

cursor（内存中为 `lastSeq int64`）在 `DrainNew` 后立即推进（内存），但只在 session context 保存时一起持久化到 DB，保证 cursor 和 context 的一致性。

### Decision 3: cursor 与 context 原子持久化

**选择**：`event_cursor` 存储在 `agent_sessions` 表中。在 processor 保存 session context 时，cursor 一并更新：

```sql
UPDATE agent_sessions SET context = ?, event_cursor = ? WHERE id = ?
```

**理由**：cursor 表示"agent 已经看到了哪些事件"，context 表示"agent 的对话历史"。两者必须一致——如果 cursor 推进了但 context 没保存，重启后 agent 会跳过未处理的事件；如果 context 保存了但 cursor 没推进，重启后 agent 会重复处理事件。原子更新消除这个不一致窗口。

**崩溃恢复保证**：如果进程在 `DrainNew` 之后、context 保存之前崩溃，cursor 未持久化，重启后 `DrainNew` 会重新返回这些事件——这是正确的，因为它们确实还没被处理。

### Decision 4: Engine 双 Checkpoint 机制

**选择**：在 `AgentLoopConfig` 中新增两个回调：
- `DrainNewEvents func() []types.AgentMessage` — Checkpoint 1，每次 LLM 调用前调用
- `HasNewEvents func() bool` — Checkpoint 2，tool 执行前调用

Engine 循环行为：
1. **Checkpoint 1**（循环顶部）：`DrainNewEvents` 查 DB 获取新事件，合并为一条 user message，追加到 messages
2. **Checkpoint 2**（tool 执行前）：`HasNewEvents` 查 DB 探测，有新事件则放弃所有 tool，记录 abandoned tool_result，`continue` 回到 checkpoint 1
3. 活锁防护：`maxConsecutivePreemptions = 3`

### Decision 5: 进程重启后的 session 恢复

**选择**：agent 启动时扫描 `execution_status = 'processing'` 的 session，加载 `event_cursor`，检查是否有 `seq > cursor` 的事件。若有，启动 `drainWorker` 恢复处理。

**恢复流程**：
1. `SELECT id, event_cursor FROM agent_sessions WHERE execution_status = 'processing'`
2. 对每个 session：`SELECT EXISTS(SELECT 1 FROM session_events WHERE session_id = ? AND seq > ?)`
3. 有未处理事件 → 创建 `SessionWorker`，启动 `drainWorker`
4. 无未处理事件 → 将 `execution_status` 更新为 `interrupted`

### Decision 6: 进程内通知机制

**选择**：沿用现有的 `workerMutex` + `Processing` flag 模式。dispatcher 写 DB 后调用 `EnqueueProcessRequest`，该函数在 mutex 保护下检查 worker 状态并按需启动 `drainWorker`。

**竞态安全**：`drainWorker` 退出前在 mutex 保护下执行 `HasNew` 双重检查：
1. `DrainNew` 返回空 → 加锁
2. 加锁后再次 `HasNew` → 仍为 false → 安全退出
3. 若在步骤 1-2 之间有新事件写入 DB 且 `EnqueueProcessRequest` 被调用 → 两种情况都安全：
   - 若 `HasNew` 看到新事件 → 继续处理
   - 若 `HasNew` 未看到（极小窗口）→ `Processing` 设为 false → `EnqueueProcessRequest` 看到 false → 启动新 `drainWorker`

## Risks / Trade-offs

**[活锁风险]** → 高频消息场景下 agent 反复放弃 tool 执行。
→ Mitigation: `maxConsecutivePreemptions = 3` 硬限制。

**[废弃 tool_result 占用上下文 token]** → 反复放弃会累积 abandoned tool_result。
→ Mitigation: 放弃原因单行清晰描述，token 开销可控。

**[DB 查询频率]** → `HasNew` 在每次 tool 执行前都查 DB，高频 tool call 场景可能增加 DB 负载。
→ Mitigation: 单条 `SELECT EXISTS` 查询极轻量，SQLite 本地文件无网络开销。若需优化，可在进程内用 atomic counter 缓存事件计数，避免每次查 DB。

## Open Questions

1. `maxConsecutivePreemptions` 的最优值：3 是初始值，需要根据实际使用场景调优。
