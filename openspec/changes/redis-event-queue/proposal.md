## Why

当前 agent 使用进程内 `[]QueuedRequest` 切片作为每个 session 的消息队列。这导致两个核心问题：

1. **决策基于过时信息**：LLM 在 tool 执行期间无法感知新到达的用户消息，可能基于已失效的上下文执行副作用操作（如发送邮件后用户才说"别发了"）。
2. **进程中断丢失状态**：内存中的待处理消息和消费进度在进程崩溃后全部丢失，重启后无法恢复未完成的 session。

## What Changes

- 新增 `session_events` 表，记录外部事件的到达顺序（append-only，AUTOINCREMENT seq）
- 在 `agent_sessions` 表新增 `event_cursor` 字段，持久化消费进度，与 session context 原子更新
- 引入 EventLog 消费模型，基于 DB 查询提供 `DrainNew`（批量全取消费）和 `HasNew`（peek）两种语义
- 重构 engine loop（`loop.go`），增加两个 checkpoint：
  - **Checkpoint 1**（LLM 调用前）：消费所有新 event，合并为一条 user message，让 LLM 看到完整世界
  - **Checkpoint 2**（tool 执行前）：peek 是否有新 event，有则放弃所有 tool 执行（保持 tool_use/tool_result 配对），回到 checkpoint 1 重新决策
- 实现进程重启后的 session 恢复：扫描 `execution_status = 'processing'` 的 session，检查 `event_cursor` 之后是否有未处理事件，恢复处理
- 移除现有的 absorb 循环（`processOneEvent` 中的 `MaxAbsorbRounds` 机制），由新的 checkpoint 机制替代

## Capabilities

### New Capabilities
- `event-log`: 基于 DB 的 session 级事件消费模型（session_events 表、cursor、DrainNew、HasNew），支持崩溃恢复
- `engine-event-checkpoint`: Engine loop 的双 checkpoint 机制——LLM 调用前合并 event、tool 执行前 peek 守卫

### Modified Capabilities

## Impact

- **核心改动文件**：`agent/internal/runner/worker.go`、`agent/internal/runner/processor.go`、`agent/internal/engine/loop.go`、`agent/internal/storage/db.go`
- **新增依赖**：无（纯 DB 方案，不引入 Redis）
- **DB Schema 变化**：新增 `session_events` 表；`agent_sessions` 新增 `event_cursor` 列
- **API 变化**：`AgentLoopConfig` 新增 `DrainNewEvents` 和 `HasNewEvents` 字段；`AgentLoopResult` 新增 `EventPreempted` 字段
- **移除逻辑**：`popAllPending`、`MaxAbsorbRounds`、absorb 循环、`SessionWorker.Queue` 切片
- **部署影响**：无新依赖，DB migration 自动执行
- **向后兼容**：内部重构，对外 HTTP API（`/api/channels/incoming`）无变化
