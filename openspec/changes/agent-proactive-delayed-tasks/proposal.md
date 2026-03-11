## Why

当前 agent 是纯被动响应式的——只有用户发消息才会触发处理。这限制了 agent 在实际工作场景中的价值，例如：用户说"明天上午 10 点提醒我开会"，agent 无法自主在到期时间发起动作。增加"主动能力"是 agent 从"问答工具"进化为"协作伙伴"的关键一步。

## What Changes

- **系统提示词增强**：在 agent system prompt 中增加"后续任务发现"指引，让模型在对话结束前主动识别是否存在需要延后执行的任务，并使用工具设定延时任务。
- **新增 `set_delayed_task` 内置工具**：agent 可调用该工具，传入任务描述和期望执行时间，将延时任务持久化到 SQLite。
- **延时任务调度器**：agent 进程启动时启动后台扫描协程，定期扫描已到期的延时任务，将其以 `ProcessRequest` 的形式投递到对应 session 的队列中，触发 agent 自主执行。
- **到期事件格式**：到期任务以结构化的系统事件格式进入 agent 上下文，与用户消息、subagent 通知形成明确区分，避免模型歧义。

## Capabilities

### New Capabilities
- `delayed-task-tool`: agent 内置工具 `set_delayed_task`，支持设定延时任务并持久化到 DB（不设取消工具，由模型在任务到期时根据上下文自行判断是否执行）
- `delayed-task-scheduler`: 后台调度器，扫描到期任务并投递到 runner 队列
- `proactive-prompt`: system prompt 中的"后续任务发现"指引段落

### Modified Capabilities
<!-- 无需修改现有 spec -->

## Impact

- **新增 DB 表**：`delayed_tasks`（存储延时任务元数据）
- **新增代码**：
  - `agent/internal/storage/` — delayed_tasks 表的 CRUD
  - `agent/internal/runner/` — scheduler 协程 + `set_delayed_task` 工具注册
  - system prompt 模板中的 proactive 段落
- **修改代码**：
  - `agent/cmd/agent/main.go` — 启动 scheduler，shutdown 时停止
  - `agent/internal/runner/processor.go` — 注册新工具
  - `agent/internal/storage/db.go` — 新增 delayed_tasks 表 schema
- **依赖**：无新外部依赖，复用现有 SQLite + runner 队列
- **风险**：scheduler 扫描频率需要平衡及时性和 DB 负载（建议 30s 间隔）
