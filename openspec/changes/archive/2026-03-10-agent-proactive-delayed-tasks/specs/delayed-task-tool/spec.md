## ADDED Requirements

### Requirement: Agent 可通过 set_delayed_task 工具设定延时任务

系统 SHALL 提供名为 `set_delayed_task` 的内置工具，允许 agent 在对话过程中设定延时任务。工具接受 `task`（任务描述，string，必填）和 `execute_at`（执行时间，ISO 8601 格式，string，必填）两个参数。调用成功后 SHALL 将任务持久化到 `delayed_tasks` 表，状态为 `pending`，并返回 `{ taskId, task, executeAt, status: "scheduled" }`。

工具 SHALL 自动注入当前请求的上下文信息（session_id, agent_id, user_id, channel, channel_user_id, channel_conversation_id），调用方无需手动传递这些字段。

`execute_at` 在写入 DB 前 SHALL 被归一化为 SQLite datetime 格式的 UTC 时间（如 `2025-03-12 02:00:00`），以确保与 `datetime('now')` 的字符串比较语义正确。支持多种输入格式的容错解析（RFC 3339、带 T 分隔符、带空格分隔符等）。

#### Scenario: Agent 成功设定一个延时任务
- **WHEN** agent 调用 `set_delayed_task`，传入 `task="提醒张三准备周会材料"` 和 `execute_at="2025-03-12T10:00:00+08:00"`
- **THEN** 系统在 `delayed_tasks` 表创建一条记录，status 为 `pending`，execute_at 为归一化后的 UTC 时间 `2025-03-12 02:00:00`，并返回包含 taskId 的成功响应

#### Scenario: 缺少必填参数时返回错误
- **WHEN** agent 调用 `set_delayed_task`，未传入 `task` 或 `execute_at`
- **THEN** 系统返回错误信息，不创建任何记录

#### Scenario: execute_at 为过去时间时仍接受
- **WHEN** agent 调用 `set_delayed_task`，`execute_at` 为过去的时间
- **THEN** 系统仍创建记录（调度器下次扫描时会立即投递）

### Requirement: Agent 可通过 cancel_delayed_task 取消待执行任务

系统 SHALL 提供名为 `cancel_delayed_task` 的内置工具，允许 agent 取消一个尚未执行的延时任务。工具接受 `task_id`（string，必填）参数。

调用时 SHALL 验证任务属于当前 session 且 status 为 `pending`，满足条件则将 status 更新为 `cancelled`，返回 `{ taskId, status: "cancelled" }`。不满足条件时返回错误。

#### Scenario: Agent 成功取消一个待执行任务
- **WHEN** agent 调用 `cancel_delayed_task`，传入一个属于当前 session 且 status 为 `pending` 的 task_id
- **THEN** 系统将该任务 status 更新为 `cancelled`，调度器后续扫描不会投递该任务

#### Scenario: 取消不属于当前 session 的任务时返回错误
- **WHEN** agent 调用 `cancel_delayed_task`，传入一个属于其他 session 的 task_id
- **THEN** 系统返回错误，不修改任何记录

#### Scenario: 取消已投递或已取消的任务时返回错误
- **WHEN** agent 调用 `cancel_delayed_task`，传入一个 status 为 `dispatched` 或 `cancelled` 的 task_id
- **THEN** 系统返回错误，不修改任何记录

### Requirement: Agent 可通过 list_delayed_tasks 查询待执行任务

系统 SHALL 提供名为 `list_delayed_tasks` 的内置工具，允许 agent 查看当前 session 中所有 status 为 `pending` 的延时任务。工具无需输入参数。

返回 `{ count, tasks: [{ taskId, task, executeAt, createdAt }] }`，按 execute_at 升序排列。

#### Scenario: 当前 session 有待执行任务
- **WHEN** agent 调用 `list_delayed_tasks`，当前 session 有 2 条 pending 任务
- **THEN** 系统返回 count 为 2，tasks 包含两条任务的详细信息

#### Scenario: 当前 session 无待执行任务
- **WHEN** agent 调用 `list_delayed_tasks`，当前 session 无 pending 任务
- **THEN** 系统返回 count 为 0，tasks 为空数组

### Requirement: 延时任务存储模型

系统 SHALL 在 SQLite 中维护 `delayed_tasks` 表，包含以下字段：id, session_id, agent_id, user_id, channel, channel_user_id, channel_conversation_id, task, execute_at, status, created_at, updated_at。

status 字段的合法值 SHALL 为：`pending`（待执行）、`dispatched`（已投递）、`cancelled`（已取消）。

execute_at 字段 SHALL 存储为 SQLite datetime 格式的 UTC 时间（`YYYY-MM-DD HH:MM:SS`），写入时由 `NormalizeToSQLiteDatetime` 函数从 ISO 8601 输入转换。

SHALL 在 `(status, execute_at)` 上建立索引以支持调度器高效查询。

#### Scenario: 表在 DB 初始化时自动创建
- **WHEN** agent 进程启动并初始化数据库
- **THEN** `delayed_tasks` 表及索引已存在（通过 schema migration）

#### Scenario: 任务记录包含完整路由信息
- **WHEN** 一条延时任务被创建
- **THEN** 记录中 session_id, agent_id, user_id, channel, channel_user_id, channel_conversation_id 均从当前请求上下文填充
