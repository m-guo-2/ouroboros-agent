## Why

当前 agent 是纯被动响应式的——只有用户发消息才会触发处理。这限制了 agent 在实际工作场景中的价值，例如：用户说"明天上午 10 点提醒我开会"，agent 无法自主在到期时间发起动作。增加"主动能力"是 agent 从"问答工具"进化为"协作伙伴"的关键一步。

## What Changes

- **系统提示词增强**：在 agent system prompt 中增加"主动能力"四段式指引（设定、取消、到期处理、自我续期），让模型在对话中主动识别需要延后执行的任务，并管理其生命周期。
- **新增三个内置工具**：
  - `set_delayed_task`：设定延时任务，execute_at 写入时归一化为 SQLite UTC 格式
  - `cancel_delayed_task`：取消尚未执行的延时任务
  - `list_delayed_tasks`：查看当前 session 的待执行任务
- **延时任务调度器**：agent 进程启动时启动后台扫描协程，定期扫描已到期的延时任务，将其以 `ProcessRequest` 的形式投递到对应 session 的队列中，触发 agent 自主执行。
- **到期事件格式**：到期任务以纯结构化数据格式进入 agent 上下文，包含创建时间、计划执行时间、实际触发时间三个时间锚点，不包含行为指令（行为指导统一在 system prompt 中定义）。

## Capabilities

### New Capabilities
- `delayed-task-tool`: agent 内置工具 `set_delayed_task`（设定延时任务）、`cancel_delayed_task`（取消任务）、`list_delayed_tasks`（查询待执行任务）
- `delayed-task-scheduler`: 后台调度器，扫描到期任务并投递到 runner 队列
- `proactive-prompt`: system prompt 中的"主动能力"四段式指引（设定、取消、到期处理、自我续期）

### Modified Capabilities
<!-- 无需修改现有 spec -->

## Impact

- **新增 DB 表**：`delayed_tasks`（存储延时任务元数据，status 支持 pending/dispatched/cancelled）
- **新增代码**：
  - `agent/internal/storage/` — delayed_tasks 表的 CRUD + 取消 + 按 session 查询
  - `agent/internal/runner/` — scheduler 协程 + 三个工具注册
  - system prompt 模板中的 proactive 段落
- **修改代码**：
  - `agent/cmd/agent/main.go` — 启动 scheduler，shutdown 时停止
  - `agent/internal/runner/processor.go` — 注册三个工具
  - `agent/internal/storage/db.go` — 新增 delayed_tasks 表 schema
- **依赖**：无新外部依赖，复用现有 SQLite + runner 队列
- **风险**：scheduler 扫描频率需要平衡及时性和 DB 负载（建议 30s 间隔）
