## ADDED Requirements

### Requirement: 后台调度器定期扫描并投递到期任务

系统 SHALL 在 agent 进程启动时启动一个后台调度器协程，以固定间隔（默认 30 秒）扫描 `delayed_tasks` 表中 status 为 `pending` 且 `execute_at <= 当前时间` 的记录。

对每条到期任务，调度器 SHALL：
1. 将 status 更新为 `dispatched`
2. 构造 `ProcessRequest` 并调用 `runner.EnqueueProcessRequest` 投递到对应 session 队列

#### Scenario: 到期任务被自动投递
- **WHEN** 一条 `delayed_tasks` 记录的 execute_at 已过期，status 为 `pending`
- **THEN** 调度器在下次扫描周期内将其 status 更新为 `dispatched`，并以 `ProcessRequest` 形式投递到 runner 队列

#### Scenario: 已投递的任务不会被重复处理
- **WHEN** 一条 `delayed_tasks` 记录的 status 为 `dispatched`
- **THEN** 调度器扫描时跳过该记录

#### Scenario: 已取消的任务不会被投递
- **WHEN** 一条 `delayed_tasks` 记录的 status 为 `cancelled`
- **THEN** 调度器扫描时跳过该记录

#### Scenario: 进程重启后恢复待执行任务
- **WHEN** agent 进程重启，DB 中存在 status 为 `pending` 且已过期的记录
- **THEN** 调度器首次扫描即投递这些任务

### Requirement: 调度器生命周期与进程绑定

调度器 SHALL 通过 `context.WithCancel` 管理生命周期。`main.go` 在收到 SIGTERM/SIGINT 时 SHALL 取消该 context，调度器 SHALL 在当前扫描周期结束后停止。

#### Scenario: 优雅关闭时调度器停止
- **WHEN** agent 进程收到 SIGTERM 信号
- **THEN** 调度器停止扫描，不再投递新任务

### Requirement: 到期事件以纯结构化数据格式进入 agent 上下文

到期任务投递到 runner 队列时，`ProcessRequest.Content` SHALL 使用以下格式：

```
【系统事件：定时任务到期】
task_id: {id}
创建时间: {created_at}
计划执行时间: {execute_at}
实际触发时间: {now}
任务内容：{task description}
```

事件 SHALL 包含三个时间锚点：
- **创建时间**：任务设定时的时间，表示决策上下文
- **计划执行时间**：agent 设定的预期触发时间
- **实际触发时间**：调度器投递时的 `time.Now()` UTC 时间

事件 SHALL NOT 包含行为指令（如"请判断…""请结合上下文…"）。行为指导统一在 system prompt 中定义，遵循"数据归 event，行为归 prompt"原则。

`ProcessRequest` 的路由字段 SHALL 从 `delayed_tasks` 记录还原：
- `Channel`：delayed_tasks.channel
- `ChannelUserID`：delayed_tasks.channel_user_id
- `ChannelConversationID`：delayed_tasks.channel_conversation_id
- `AgentID`：delayed_tasks.agent_id
- `UserID`：delayed_tasks.user_id
- `SessionID`：delayed_tasks.session_id
- `SenderName`：空字符串（系统事件）
- `MessageType`：`"text"`
- `MessageID`：`"delayed-task-{task_id}"`

#### Scenario: 到期事件内容不与用户消息混淆
- **WHEN** 到期事件进入 agent 处理流程
- **THEN** 事件内容以 `【系统事件：定时任务到期】` 开头，与用户消息的 `senderName: content` 格式在结构上明确区分

#### Scenario: 到期事件包含完整时间线供 agent 判断
- **WHEN** agent 收到到期事件
- **THEN** 事件内容包含创建时间、计划执行时间和实际触发时间，agent 可据此判断任务是准时、延迟还是已过时

#### Scenario: 到期事件投递到正确的 session
- **WHEN** 一条延时任务设定时属于 session A
- **THEN** 到期投递时 `ProcessRequest.SessionID` 为 session A 的 ID，事件进入 session A 的 worker 队列
