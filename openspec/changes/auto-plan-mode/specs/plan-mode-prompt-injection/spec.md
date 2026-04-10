## ADDED Requirements

### Requirement: plan 模式下注入只读约束指令
当 SessionWorker.Mode 为 `"plan"` 时，`BuildSystemPrompt` SHALL 在 system prompt 末尾追加一段 plan 模式约束指令。当 Mode 为 `"normal"` 时 SHALL 不追加任何额外内容。

约束指令 SHALL 包含以下语义：
- 明确告知 LLM 当前处于计划模式
- 列出允许使用的只读工具
- 禁止使用任何会产生副作用的工具
- 指引 LLM 收集信息后制定计划
- 指引 LLM 使用 exit_plan_mode 工具提交计划

#### Scenario: plan 模式下 system prompt 包含约束指令
- **WHEN** SessionWorker.Mode 为 `"plan"` 且 processSession 构建 system prompt
- **THEN** 传给 RunAgentLoop 的 SystemPrompt SHALL 在原始 prompt 基础上追加 plan 模式约束指令

#### Scenario: normal 模式下 system prompt 不变
- **WHEN** SessionWorker.Mode 为 `"normal"` 且 processSession 构建 system prompt
- **THEN** 传给 RunAgentLoop 的 SystemPrompt SHALL 与无 plan 模式时完全相同

### Requirement: plan 约束指令不改变工具注册
plan 模式下的行为约束 SHALL 完全通过 prompt injection 实现。工具注册（ToolRegistry）在 plan 模式和 normal 模式下 SHALL 完全相同——不移除、不替换、不拦截任何工具。

#### Scenario: plan 模式下工具列表不变
- **WHEN** SessionWorker.Mode 为 `"plan"` 且 RunAgentLoop 获取工具列表
- **THEN** 工具列表 SHALL 与 normal 模式下完全相同
