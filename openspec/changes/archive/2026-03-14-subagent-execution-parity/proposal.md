## Why

当前子 agent（subagent）的执行逻辑与主 agent 存在结构性差异：子 agent 直接调用 `RunAgentLoop` 但不具备主 agent 的上下文压缩和事件驱动能力，取消时丢弃所有中间进度。随着 `data_report` 等耗时子 agent 上线，这些缺口会导致长任务 token 溢出、取消后信息丢失、主 agent 无法获知子 agent 已完成的工作。

## What Changes

- **子 agent 执行循环对齐主 agent**：在 `manager.run()` 中引入与主 agent 一致的外层循环 + `CompactContext` LLM 摘要压缩，MaxIterations 对齐为 25，支持最多 3 次 re-entry（总迭代上限 100），使长程任务不会因 token 超限而失败。
- **统一 `OnDone` 回调 + 按状态格式化通知**：将三个回调合并为 `OnDone func(*Job)`，所有终态（completed / canceled / failed）走统一 event 通道。`notifyMain` 根据 `Job.Status` 选择消息前缀（完成/中断/失败），陈述事实。取消时从已有 `loopResult` 和 `Impacts` 构建进度摘要填入 `Job.Result`。
- **子 agent 不拥有子 agent**：在工具过滤阶段显式排除 `run_subagent_async` / `get_subagent_status` / `cancel_subagent`，从根本上防止递归嵌套。
- **仅传递 task + 可选 context hint**：子 agent 启动时不再接收主 agent 的完整消息历史，改为只传递 `task` 描述 + 可选的 `context` 背景摘要。主 agent 的 LLM 自行决定是否需要、传哪些背景信息，兼顾上下文隔离与信息充分性。

## Capabilities

### New Capabilities
- `subagent-cancelation-progress`: 统一 OnDone 回调、取消时构建进度摘要、按状态格式化通知
- `subagent-context-compression`: 子 agent 外层循环 + 与主 agent 一致的 LLM 摘要压缩能力

### Modified Capabilities

## Impact

- `agent/internal/subagent/manager.go`：`run()` 逻辑重构（压缩 + 取消进度构建），`Start()` 不再传递 `req.Messages`，新增 `Context` 字段
- `agent/internal/runner/processor.go`：`registerSubagentTools` 不再传入 `messages`，`run_subagent_async` schema 新增可选 `context` 参数，子 agent 工具从注册中排除
- `agent/internal/engine/loop.go`：无修改（已支持 `context.Canceled` 返回中间 messages）
- `agent/internal/runner/compact.go`：需导出相关函数供 `subagent` 包调用
