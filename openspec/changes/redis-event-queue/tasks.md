## 1. DB Schema 变更

- [x] 1.1 在 `storage/db.go` 新增 `session_events` 表（`seq INTEGER PRIMARY KEY AUTOINCREMENT`, `session_id TEXT NOT NULL`, `message_id TEXT NOT NULL`），创建 `(session_id, seq)` 索引
- [x] 1.2 在 `agent_sessions` 表新增 `event_cursor INTEGER DEFAULT 0` 列（migration）
- [x] 1.3 在 `storage/` 包新增 `AppendSessionEvent(sessionId, messageId)` 和 `GetSessionEvents(sessionId, afterSeq) []EventRow` 查询函数

## 2. EventLog 消费模型

- [x] 2.1 创建 `agent/internal/eventlog/eventlog.go`，定义 `EventLog` 结构体（持有 sessionId、cursor int64）
- [x] 2.2 实现 `DrainNew() []Event` — 查询 `session_events JOIN messages WHERE seq > cursor ORDER BY seq`，内存推进 cursor，一次性返回全部
- [x] 2.3 实现 `HasNew() bool` — `SELECT EXISTS(... WHERE session_id = ? AND seq > ?)`
- [x] 2.4 实现 `Cursor() int64` — 返回当前内存 cursor 值，供 processor 持久化
- [x] 2.5 实现 `NewEventLog(sessionId, initialCursor)` 构造函数，支持从持久化 cursor 恢复
- [x] 2.6 为 EventLog 编写单元测试（DrainNew 批量全取、HasNew/DrainNew 正交性、cursor 恢复）

## 3. Engine Checkpoint 机制

- [x] 3.1 在 `AgentLoopConfig` 中新增 `DrainNewEvents func() []types.AgentMessage` 和 `HasNewEvents func() bool` 字段
- [x] 3.2 在 `AgentLoopResult` 中新增 `EventPreempted bool` 字段
- [x] 3.3 实现 Checkpoint 1：engine loop 每次 LLM 调用前调用 `DrainNewEvents`，将返回的事件合并为一条 user message 追加到 messages
- [x] 3.4 实现 Checkpoint 2：LLM 返回 tool_use 后、执行 tool 前调用 `HasNewEvents`；为 true 时生成 abandoned tool_result 并 continue
- [x] 3.5 实现活锁防护：`maxConsecutivePreemptions` 计数器，达到阈值后跳过 Checkpoint 2；tool 成功执行后归零
- [x] 3.6 确保 `DrainNewEvents` 和 `HasNewEvents` 为 nil 时 engine 行为与改动前完全一致
- [x] 3.7 为 checkpoint 逻辑编写单元测试（正常 drain、preempt、活锁防护、nil 回调兼容）

## 4. Processor 层重构

- [x] 4.1 新建 `processSession` 函数，替代 `processOneEvent`，接收 EventLog 作为事件源
- [x] 4.2 实现事件合并逻辑 `mergeEventsToMessage`：多条合并为一条 user message（单条不加前缀，多条加 `[以下 N 条消息同时到达]`）
- [x] 4.3 在 `processSession` 中设置 `DrainNewEvents` 和 `HasNewEvents` 回调，连接 EventLog
- [x] 4.4 实现 processor 外层循环：engine 退出后检查 `EventLog.HasNew()`，有则 drain 合并后重新调 engine
- [x] 4.5 cursor 持久化：保存 session context 时同时更新 `event_cursor = eventLog.Cursor()`
- [x] 4.6 移除旧的 absorb 循环（`MaxAbsorbRounds`、`popAllPending` 在 processor 中的调用）

## 5. Worker 层重构

- [x] 5.1 将 `SessionWorker.Queue []QueuedRequest` 替换为 `EventLog *eventlog.EventLog`
- [x] 5.2 重构 `drainWorker`：使用 `EventLog.DrainNew()` 获取事件，调用 `processSession`；退出前在 mutex 下双重检查 `HasNew`
- [x] 5.3 重构 `EnqueueProcessRequest`：不再操作内存队列，仅在 mutex 下检查 worker 状态并按需启动 `drainWorker`（DB 写入已由 dispatcher 完成）
- [x] 5.4 移除 `popAllPending` 函数
- [x] 5.5 更新 `GracefulShutdown`：优雅关闭时确保正在处理的 session 完成当前 tool 执行

## 6. Dispatcher 层适配

- [x] 6.1 在 `dispatcher.Dispatch` 中，`SaveMessage` 之后调用 `storage.AppendSessionEvent(sessionId, messageId)` 写入事件索引
- [x] 6.2 确保 DB 写入（SaveMessage + AppendSessionEvent）在 `EnqueueProcessRequest` 之前完成

## 7. 启动恢复

- [x] 7.1 在 `agent/cmd/agent/main.go` 启动流程中，添加 session 恢复逻辑：扫描 `execution_status = 'processing'` 的 session
- [x] 7.2 对有未处理事件（`seq > event_cursor`）的 session，创建 SessionWorker + EventLog 并启动 drainWorker
- [x] 7.3 对无未处理事件的 session，将 `execution_status` 更新为 `interrupted`

## 8. 集成与清理

- [ ] 8.1 端到端测试：模拟多条消息快速到达，验证 checkpoint 1 合并、checkpoint 2 preempt、tool 放弃与重决策的完整流程
- [ ] 8.2 崩溃恢复测试：模拟进程中断后重启，验证 cursor 恢复和 session 恢复
- [x] 8.3 移除已废弃的代码：`MaxAbsorbRounds` 常量、旧 `processOneEvent` 函数、`popAllPending` 函数
