## ADDED Requirements

### Requirement: tool_result 携带 subTraceId
当 `buildTrace` 处理 `tool_result` 事件且 `tool` 为 `run_subagent_async` 时，系统 SHALL 从 `toolResult` 中提取 `subTraceId` 字段并写入 `executionStep.SubTraceID`。

#### Scenario: run_subagent_async 的 tool_result 包含 subTraceId
- **WHEN** trace 事件序列中存在 `traceEvent=tool_result`、`tool=run_subagent_async`，且 `toolResult` 为 JSON 对象包含 `subTraceId: "subtrace-999"`
- **THEN** 对应的 `executionStep` 的 `SubTraceID` 字段为 `"subtrace-999"`

#### Scenario: run_subagent_async 的 tool_result 不含 subTraceId
- **WHEN** `toolResult` 中不包含 `subTraceId` 字段
- **THEN** `executionStep.SubTraceID` 为空字符串，不影响其他字段

### Requirement: executionStep 类型扩展
`executionStep` 结构体 SHALL 新增 `SubTraceID string` 字段（JSON key: `subTraceId`），用于 `tool_result` 步骤关联 subagent trace。

#### Scenario: JSON 序列化包含 subTraceId
- **WHEN** `executionStep.SubTraceID` 为非空字符串
- **THEN** JSON 输出中包含 `"subTraceId": "subtrace-999"` 字段

#### Scenario: subTraceId 为空时 omitempty
- **WHEN** `executionStep.SubTraceID` 为空字符串
- **THEN** JSON 输出中不包含 `subTraceId` 字段

### Requirement: subagent_reentry 事件处理
`buildTrace` SHALL 处理 `traceEvent=subagent_reentry` 事件，将其映射为 type 为 `subagent_reentry` 的 `executionStep`，包含 `jobId` 和 `reentry` 轮次信息。

#### Scenario: subagent trace 包含 reentry 事件
- **WHEN** 通过 `subtrace-*` 查询 trace，事件序列中包含 `traceEvent=subagent_reentry`，`jobId=subjob-123`，`reentry=2`
- **THEN** 生成一个 `executionStep`，`Type` 为 `"subagent_reentry"`，`Content` 包含 jobId 和 reentry 轮次信息

#### Scenario: 主 agent trace 不含 subagent_reentry
- **WHEN** 主 agent 的 trace 事件中不包含 `subagent_reentry`
- **THEN** 不生成额外步骤，行为不变

### Requirement: tool_call 步骤携带 subagent job 元信息
当 `buildTrace` 处理 `tool_call` 事件且 `tool` 为 `run_subagent_async` 时，系统 SHALL 从 `toolInput` 中提取 `name`、`profile`、`task` 写入 `executionStep`，方便前端在不额外请求的情况下展示 subagent 基本信息。

#### Scenario: run_subagent_async 的 tool_call 包含 subagent 参数
- **WHEN** trace 事件中 `traceEvent=tool_call`、`tool=run_subagent_async`，`toolInput` 包含 `name: "web-research-subagent"`、`profile: "web_research"`、`task: "搜索最新 Go 1.23 变更"`
- **THEN** 对应 `executionStep` 的 `ToolInput` 正常包含这些字段（现有行为），前端可直接从中读取
