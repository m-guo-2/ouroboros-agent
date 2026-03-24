## ADDED Requirements

### Requirement: Manager 提供按 session 查询 job 列表的方法
`subagent.Manager` SHALL 暴露 `ListBySession(sessionID string) []*Job` 方法，返回指定 session 下所有 job 的副本列表，按 `CreatedAt` 降序排列。

#### Scenario: 查询有 subagent job 的 session
- **WHEN** session "s-123" 下有 3 个 subagent job（1 completed、1 running、1 failed）
- **THEN** `ListBySession("s-123")` 返回 3 个 Job 副本，按创建时间降序
- **THEN** 每个 Job 包含 ID、Name、Profile、Status、CreatedAt、UpdatedAt、SubTraceID、Impacts 长度等信息

#### Scenario: 查询无 subagent job 的 session
- **WHEN** session "s-456" 下没有 subagent job
- **THEN** `ListBySession("s-456")` 返回空切片（非 nil）

### Requirement: Session Subagent Jobs 列表 API
系统 SHALL 提供 `GET /api/agent-sessions/{id}/subagent-jobs` 端点，返回指定 session 的所有 subagent job 摘要列表。

#### Scenario: 成功获取 session 的 subagent job 列表
- **WHEN** 客户端调用 `GET /api/agent-sessions/s-123/subagent-jobs`
- **THEN** 响应状态码为 200
- **THEN** 响应体为 JSON 数组，每个元素包含 `id`、`name`、`profile`、`status`、`subTraceId`、`parentTraceId`、`createdAt`、`updatedAt`、`impactCount`（impacts 数量）、`task`（截断到 200 字符）

#### Scenario: session 不存在或无 subagent job
- **WHEN** 客户端调用 `GET /api/agent-sessions/s-unknown/subagent-jobs`
- **THEN** 响应状态码为 200，响应体为空数组 `[]`

### Requirement: 单个 Subagent Job 详情 API
系统 SHALL 提供 `GET /api/subagent-jobs/{jobId}` 端点，返回单个 subagent job 的完整信息。

#### Scenario: 成功获取 job 详情
- **WHEN** 客户端调用 `GET /api/subagent-jobs/subjob-12345`，该 job 存在于 Manager 内存中
- **THEN** 响应状态码为 200
- **THEN** 响应体包含完整 Job 信息：`id`、`name`、`profile`、`task`、`status`、`subTraceId`、`parentTraceId`、`sessionId`、`createdAt`、`updatedAt`、`result`、`error`、`impacts`（完整列表）
- **THEN** 响应体还包含 `events` 字段，为从 `events.jsonl` 读取的事件时间线数组

#### Scenario: job 不存在
- **WHEN** 客户端调用 `GET /api/subagent-jobs/subjob-nonexist`
- **THEN** 响应状态码为 404，响应体包含错误信息

### Requirement: Manager 读取 job 事件时间线
`subagent.Manager` SHALL 暴露 `ReadEvents(jobID string) ([]map[string]interface{}, error)` 方法，从对应 job 的 `events.jsonl` 文件读取所有事件行，解析为 JSON 对象列表返回。

#### Scenario: job 有事件记录
- **WHEN** `subjob-123` 的 `events.jsonl` 包含 5 行有效 JSON
- **THEN** `ReadEvents("subjob-123")` 返回长度为 5 的切片，每个元素为解析后的 map

#### Scenario: job 无事件文件
- **WHEN** `subjob-new` 的 `events.jsonl` 不存在
- **THEN** `ReadEvents("subjob-new")` 返回空切片和 nil error

#### Scenario: events.jsonl 包含损坏行
- **WHEN** `events.jsonl` 中某行不是有效 JSON
- **THEN** 跳过该行继续解析，不返回 error
