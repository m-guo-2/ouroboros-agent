## 1. 数据层：delayed_tasks 表

- [x] 1.1 在 `agent/internal/storage/db.go` 的 `runSchema` 中添加 `delayed_tasks` 表的 CREATE TABLE 语句和 `(status, execute_at)` 索引
- [x] 1.2 在 `agent/internal/storage/` 中新增 `delayed_tasks.go`，实现函数：`CreateDelayedTask`, `QueryDueTasks`, `MarkTaskDispatched`

## 2. 内置工具：set_delayed_task

- [x] 2.1 在 `agent/internal/runner/processor.go` 的 `processOneEvent` 中注册 `set_delayed_task` 工具，输入 `task` + `execute_at`，自动注入当前 session/agent/channel 上下文，调用 `storage.CreateDelayedTask`

## 3. 后台调度器

- [x] 3.1 新增 `agent/internal/runner/scheduler.go`，实现 `StartDelayedTaskScheduler(ctx context.Context)` 函数：每 30s 扫描到期任务，构造 `ProcessRequest` 并调用 `EnqueueProcessRequest` 投递
- [x] 3.2 到期事件 Content 使用结构化格式：以 `【系统事件：定时任务到期】` 开头，包含 task_id、创建时间和任务描述，末尾添加执行指令
- [x] 3.3 在 `agent/cmd/agent/main.go` 中：DB 初始化后启动 scheduler 协程，shutdown 时通过 context cancel 停止 scheduler

## 4. System Prompt 增强

- [x] 4.1 编写"主动能力"prompt 段落，包含：后续任务发现指引 + 到期事件处理指引
- [x] 4.2 通过 SQL seed 或文档说明，将 prompt 段落添加到 agent_configs.system_prompt 中（建议新增一个 migration SQL 文件）

## 5. 验证

- [ ] 5.1 启动 agent，确认 `delayed_tasks` 表自动创建（需手动验证）
- [ ] 5.2 通过对话触发 agent 调用 `set_delayed_task`，确认 DB 记录正确写入（需手动验证）
- [ ] 5.3 等待任务到期（或设定一个已过期的时间），确认 scheduler 扫描并投递事件，agent 收到后结合上下文正确执行（需手动验证）
