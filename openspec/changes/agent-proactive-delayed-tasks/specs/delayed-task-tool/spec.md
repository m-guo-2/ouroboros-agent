## ADDED Requirements

### Requirement: Agent 可通过 set_delayed_task 工具设定延时任务

系统 SHALL 提供名为 `set_delayed_task` 的内置工具，允许 agent 在对话过程中设定延时任务。工具接受 `task`（任务描述，string，必填）和 `execute_at`（执行时间，ISO 8601 格式，string，必填）两个参数。调用成功后 SHALL 将任务持久化到 `delayed_tasks` 表，状态为 `pending`，并返回 `{ taskId, task, executeAt, status: "scheduled" }`。

工具 SHALL 自动注入当前请求的上下文信息（session_id, agent_id, user_id, channel, channel_user_id, channel_conversation_id），调用方无需手动传递这些字段。

#### Scenario: Agent 成功设定一个延时任务
- **WHEN** agent 调用 `set_delayed_task`，传入 `task="提醒张三准备周会材料"` 和 `execute_at="2025-03-12T10:00:00+08:00"`
- **THEN** 系统在 `delayed_tasks` 表创建一条记录，status 为 `pending`，execute_at 为指定时间，并返回包含 taskId 的成功响应

#### Scenario: 缺少必填参数时返回错误
- **WHEN** agent 调用 `set_delayed_task`，未传入 `task` 或 `execute_at`
- **THEN** 系统返回错误信息，不创建任何记录

#### Scenario: execute_at 为过去时间时仍接受
- **WHEN** agent 调用 `set_delayed_task`，`execute_at` 为过去的时间
- **THEN** 系统仍创建记录（调度器下次扫描时会立即投递）

### Requirement: 延时任务不设显式取消工具

系统 SHALL NOT 提供取消延时任务的工具。任务一旦设定，始终在到期时投递。模型在收到到期事件时，SHALL 根据当前对话上下文自行判断任务是否仍然适用——如果用户意图已变化（如用户在对话中表示"不需要提醒了"），模型应选择不执行或告知用户情况已变，而非机械执行原始任务。

#### Scenario: 用户在到期前已表达不需要
- **WHEN** 用户在设定定时任务后、到期前说"那个提醒不用了"
- **THEN** 任务仍然在到期时投递到 agent，agent 读取上下文后判断用户已不需要，选择不执行或简要告知

#### Scenario: 用户意图发生变化
- **WHEN** 用户设定"周五提醒我交方案"，但后续对话中说"方案已经提前交了"
- **THEN** 任务到期时 agent 结合上下文判断任务已完成，不再重复提醒

### Requirement: 延时任务存储模型

系统 SHALL 在 SQLite 中维护 `delayed_tasks` 表，包含以下字段：id, session_id, agent_id, user_id, channel, channel_user_id, channel_conversation_id, task, execute_at, status, created_at, updated_at。

status 字段的合法值 SHALL 为：`pending`（待执行）、`dispatched`（已投递）。

SHALL 在 `(status, execute_at)` 上建立索引以支持调度器高效查询。

#### Scenario: 表在 DB 初始化时自动创建
- **WHEN** agent 进程启动并初始化数据库
- **THEN** `delayed_tasks` 表及索引已存在（通过 schema migration）

#### Scenario: 任务记录包含完整路由信息
- **WHEN** 一条延时任务被创建
- **THEN** 记录中 session_id, agent_id, user_id, channel, channel_user_id, channel_conversation_id 均从当前请求上下文填充
