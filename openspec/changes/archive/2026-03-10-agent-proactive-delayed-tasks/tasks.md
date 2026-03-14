## 1. 数据层：delayed_tasks 表

- [x] 1.1 在 `agent/internal/storage/db.go` 的 `runSchema` 中添加 `delayed_tasks` 表的 CREATE TABLE 语句和 `(status, execute_at)` 索引
- [x] 1.2 在 `agent/internal/storage/delayed_tasks.go` 中实现函数：`CreateDelayedTask`（含 execute_at 归一化）、`QueryDueTasks`、`MarkTaskDispatched`、`CancelDelayedTask`、`ListPendingTasksBySession`
- [x] 1.3 新增 `NormalizeToSQLiteDatetime` 函数：将 ISO 8601 格式解析并转换为 SQLite datetime 格式的 UTC 时间（`YYYY-MM-DD HH:MM:SS`），支持多种输入格式容错

## 2. 内置工具：set_delayed_task / cancel_delayed_task / list_delayed_tasks

- [x] 2.1 在 `agent/internal/runner/processor.go` 的 `processOneEvent` 中注册 `set_delayed_task` 工具，输入 `task` + `execute_at`，自动注入当前 session/agent/channel 上下文，调用 `storage.CreateDelayedTask`
- [x] 2.2 注册 `cancel_delayed_task` 工具，输入 `task_id`，调用 `storage.CancelDelayedTask`，限定当前 session 范围
- [x] 2.3 注册 `list_delayed_tasks` 工具，无参数，调用 `storage.ListPendingTasksBySession`，返回当前 session 的待执行任务列表

## 3. 后台调度器

- [x] 3.1 新增 `agent/internal/runner/scheduler.go`，实现 `StartDelayedTaskScheduler(ctx context.Context)` 函数：每 30s 扫描到期任务，构造 `ProcessRequest` 并调用 `EnqueueProcessRequest` 投递
- [x] 3.2 到期事件 Content 使用纯结构化数据格式：以 `【系统事件：定时任务到期】` 开头，包含 task_id、创建时间、计划执行时间、实际触发时间和任务描述，不包含行为指令
- [x] 3.3 在 `agent/cmd/agent/main.go` 中：DB 初始化后启动 scheduler 协程，shutdown 时通过 context cancel 停止 scheduler

## 4. System Prompt 增强

- [x] 4.1 编写"主动能力"四段式 prompt：设定任务 + 取消任务 + 任务到期处理 + 自我续期
- [x] 4.2 通过 SQL seed 文件（`agent/data/055-proactive-delayed-tasks.sql`）将 prompt 段落添加到 agent_configs.system_prompt 中

## 5. 验证

- [ ] 5.1 启动 agent，确认 `delayed_tasks` 表自动创建（需手动验证）
- [ ] 5.2 通过对话触发 agent 调用 `set_delayed_task`，确认 DB 记录的 execute_at 已归一化为 SQLite UTC 格式（需手动验证）
- [ ] 5.3 等待任务到期，确认 scheduler 扫描并投递事件，事件包含三个时间锚点，agent 收到后正确执行（需手动验证）
- [ ] 5.4 通过对话触发 agent 调用 `cancel_delayed_task`，确认任务 status 变为 cancelled 且不再被投递（需手动验证）
- [ ] 5.5 通过对话触发 agent 调用 `list_delayed_tasks`，确认返回当前 session 的待执行任务列表（需手动验证）
