## ADDED Requirements

### Requirement: session_events 表记录外部事件到达顺序

系统 SHALL 维护 `session_events` 表，schema 为：
- `seq INTEGER PRIMARY KEY AUTOINCREMENT` — 全局单调递增序号
- `session_id TEXT NOT NULL`
- `message_id TEXT NOT NULL` — 引用 messages 表

Dispatcher 在保存用户消息到 messages 表后，SHALL 同时向 session_events 插入一条记录。

#### Scenario: 外部消息到达时写入 session_events
- **WHEN** dispatcher 收到用户消息并保存到 messages 表（message_id = `msg-1`），session 为 `sess-A`
- **THEN** 系统 SHALL 向 session_events 插入 `(session_id='sess-A', message_id='msg-1')`
- **AND** seq SHALL 自动递增

#### Scenario: 仅外部事件写入 session_events
- **WHEN** engine 处理过程中生成 assistant 回复或 tool_result 并保存到 messages 表
- **THEN** 系统 SHALL 不向 session_events 插入记录
- **AND** session_events 仅包含外部到达的事件（用户消息、定时任务触发等）

#### Scenario: 不同 session 的事件隔离
- **WHEN** 事件分别发往 session `sess-A`（seq=10）和 session `sess-B`（seq=11）
- **THEN** 查询 `sess-A` 的事件 SHALL 不返回 seq=11 的记录

### Requirement: EventLog 消费模型

系统 SHALL 提供 `EventLog` 结构，基于 DB 查询封装消费操作，暴露三个正交接口：
- `DrainNew() []Event` — 查询 `session_events JOIN messages WHERE seq > cursor`，一次性返回全部未消费事件，内存中推进 cursor
- `HasNew() bool` — 查询 `SELECT EXISTS(... WHERE seq > cursor)`，不推进 cursor
- cursor 仅在 session context 保存时持久化到 DB

`DrainNew` 的核心语义是"批量全取"：每次调用 SHALL 返回当前所有未消费事件。

#### Scenario: DrainNew 一次性返回全部未消费事件
- **WHEN** session_events 中有 seq=5, 6, 7 三条未消费事件（cursor=4），调用 `DrainNew()`
- **THEN** SHALL 返回 seq=5, 6, 7 对应的全部 3 条事件
- **AND** 内存 cursor 推进到 7
- **AND** 再次调用 `DrainNew()` SHALL 返回空

#### Scenario: HasNew 不推进 cursor
- **WHEN** session_events 中有未消费事件，调用 `HasNew()`
- **THEN** SHALL 返回 `true`
- **AND** cursor 不变
- **AND** 再次调用 `HasNew()` SHALL 仍返回 `true`

#### Scenario: 无新事件时的行为
- **WHEN** session_events 中没有 seq > cursor 的记录
- **THEN** `HasNew()` SHALL 返回 `false`
- **AND** `DrainNew()` SHALL 返回空切片（不阻塞）

### Requirement: cursor 与 context 原子持久化

`agent_sessions` 表 SHALL 新增 `event_cursor INTEGER DEFAULT 0` 列。cursor 与 session context SHALL 在同一个 DB 写入中更新。

#### Scenario: 正常持久化
- **WHEN** processor 保存 session context
- **THEN** 系统 SHALL 在同一条 UPDATE 语句中同时更新 `context` 和 `event_cursor`

#### Scenario: 崩溃恢复——cursor 未持久化
- **WHEN** 进程在 `DrainNew()` 之后、context 保存之前崩溃并重启
- **THEN** 加载的 `event_cursor` SHALL 为崩溃前最后一次持久化的值
- **AND** `DrainNew()` SHALL 重新返回这些事件（因为 cursor 未推进）
- **AND** 事件被重新处理（安全，因为 context 也未包含这些事件的处理结果）

### Requirement: 进程重启后 session 恢复

Agent 启动时 SHALL 扫描需要恢复的 session 并自动恢复处理。

#### Scenario: 有未处理事件的 session 恢复
- **WHEN** agent 启动，发现 session `sess-A` 的 `execution_status = 'processing'` 且 `session_events` 中有 `seq > event_cursor` 的记录
- **THEN** 系统 SHALL 创建 SessionWorker 并启动 drainWorker 恢复处理

#### Scenario: 无未处理事件的 session 标记中断
- **WHEN** agent 启动，发现 session `sess-B` 的 `execution_status = 'processing'` 但无未处理事件
- **THEN** 系统 SHALL 将 `execution_status` 更新为 `interrupted`

### Requirement: drainWorker 退出时的竞态安全

drainWorker 在决定退出之前，SHALL 在持有 mutex 的情况下执行 `HasNew` 双重检查。

#### Scenario: 退出前双重检查
- **WHEN** `DrainNew()` 返回空，drainWorker 准备退出
- **THEN** 系统 SHALL 加锁后再次调用 `HasNew()`
- **AND** 若 `HasNew()` 返回 true → 释放锁，继续处理
- **AND** 若 `HasNew()` 返回 false → 设置 `Processing = false`，释放锁，退出

#### Scenario: 退出窗口内的新事件
- **WHEN** dispatcher 在 `DrainNew()` 返回空之后、drainWorker 加锁之前写入新事件并调用 `EnqueueProcessRequest`
- **THEN** 两种情况均 SHALL 保证事件被处理：
  - drainWorker 的 `HasNew` 双重检查发现新事件 → 继续处理
  - drainWorker 已设置 `Processing = false` → `EnqueueProcessRequest` 启动新的 drainWorker
