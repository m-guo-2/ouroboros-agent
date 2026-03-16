## ADDED Requirements

### Requirement: 取消时构建进度摘要
当子 agent 被主 agent 取消（`context.Canceled`）时，系统 SHALL 从已有的 `loopResult.Messages` 和 `Job.Impacts` 构建一份结构化进度报告，填入 `Job.Result`。系统 SHALL NOT 为此发起额外的 LLM 调用。

#### Scenario: 子 agent 执行了部分工具后被取消
- **WHEN** 子 agent 已执行 2 个工具调用（产生 2 条 Impact），主 agent 调用 `cancel_subagent`
- **THEN** `Job.Result` 包含"[子任务被中断]"前缀、2 条 Impact 的 summary 列表、最后一条 assistant 文本（如有）
- **THEN** `Job.Status` 为 `canceled`

#### Scenario: 子 agent 尚未开始执行就被取消
- **WHEN** 子 agent 在第一次 LLM 调用前被取消（`loopResult` 为 nil，`Impacts` 为空）
- **THEN** `Job.Result` 包含"[子任务被中断]"前缀和"尚未开始执行"说明
- **THEN** `Job.Status` 为 `canceled`

#### Scenario: RunAgentLoop 返回非 nil loopResult 和 Canceled error
- **WHEN** `RunAgentLoop` 因 `context.Canceled` 返回，`loopResult` 非 nil 且 `loopResult.Messages` 包含 assistant 文本
- **THEN** 进度报告中包含最后一条 assistant 文本内容（截断到 500 字符以内）

### Requirement: 统一回调 OnDone 替代三回调
`StartRequest` SHALL 使用单个 `OnDone func(*Job)` 回调替代 `OnCompleted` / `OnFailed` / `OnCanceled`。`run()` 在所有终态（completed / canceled / failed）均 SHALL 调用 `OnDone`。

#### Scenario: 子 agent 正常完成
- **WHEN** 子 agent 产出 FinalText 正常完成
- **THEN** `Job.Status` 为 `completed`，`OnDone` 被调用

#### Scenario: 子 agent 被取消
- **WHEN** 子 agent 被主 agent 取消且进度摘要已构建
- **THEN** `Job.Status` 为 `canceled`，`Job.Result` 包含进度摘要，`OnDone` 被调用

#### Scenario: 子 agent 执行失败
- **WHEN** 子 agent 因 LLM 错误或其他异常失败
- **THEN** `Job.Status` 为 `failed`，`Job.Error` 包含错误信息，`OnDone` 被调用

### Requirement: notifyMain 按 Job.Status 格式化通知
`notifyMain` SHALL 根据 `Job.Status` 使用不同的消息前缀，陈述事实，不包含引导语。

#### Scenario: completed 状态的通知消息
- **WHEN** `Job.Status` 为 `completed`
- **THEN** 通知消息以"【subagent完成】"为前缀，包含 result 和 impacts

#### Scenario: canceled 状态的通知消息
- **WHEN** `Job.Status` 为 `canceled`
- **THEN** 通知消息以"【subagent中断】"为前缀，包含 result（进度摘要）和 impacts

#### Scenario: failed 状态的通知消息
- **WHEN** `Job.Status` 为 `failed`
- **THEN** 通知消息以"【subagent失败】"为前缀，包含 error 和 impacts

### Requirement: 子 agent 不传入主 agent 完整历史
子 agent 启动时 SHALL NOT 接收主 agent 的完整消息历史。子 agent 的初始 messages SHALL 由可选的 context 消息和 task 消息组成。

#### Scenario: 子 agent 启动时仅有 task
- **WHEN** 主 agent 调用 `run_subagent_async` 且未提供 `context` 参数
- **THEN** 子 agent 的初始 messages 仅包含一条由 `taskMessage(req.Task)` 构造的 user 消息
- **THEN** 不包含主 agent 的历史对话消息

#### Scenario: 子 agent 启动时带有 context hint
- **WHEN** 主 agent 调用 `run_subagent_async` 且提供了 `context` 参数（非空字符串）
- **THEN** 子 agent 的初始 messages 包含两条消息：第一条为 context 消息（role=user，以"[背景信息]"为前缀），第二条为 `taskMessage(req.Task)`
- **THEN** 不包含主 agent 的历史对话消息

### Requirement: run_subagent_async 支持可选 context 参数
`run_subagent_async` 工具 schema SHALL 包含一个可选的 `context` 字符串参数，用于主 agent 向子 agent 传递精炼后的背景信息。

#### Scenario: 工具 schema 包含 context 字段
- **WHEN** 查看 `run_subagent_async` 的工具定义
- **THEN** `properties` 中包含 `context` 字段，类型为 string，不在 `required` 列表中

### Requirement: 子 agent 不可启动子 agent
子 agent 的可用工具列表 SHALL NOT 包含 `run_subagent_async`、`get_subagent_status`、`cancel_subagent`。此排除 SHALL 作为全局硬约束，对所有 profile 生效。

#### Scenario: 子 agent 尝试调用 subagent 工具
- **WHEN** 子 agent 的工具列表经过 `filterToolsByProfile` 过滤
- **THEN** 结果中不包含 `run_subagent_async`、`get_subagent_status`、`cancel_subagent`
- **THEN** 无论 profile 的 `allowedToolsForProfile` 返回什么，这三个工具始终被排除
