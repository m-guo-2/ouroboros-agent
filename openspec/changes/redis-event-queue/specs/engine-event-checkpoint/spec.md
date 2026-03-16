## ADDED Requirements

### Requirement: Checkpoint 1 — LLM 调用前合并所有新事件

Engine loop 在每次调用 LLM 之前，SHALL 调用 `DrainNewEvents` 回调消费所有新到达的外部事件，并将它们合并为一条 user message 追加到 messages 列表。

#### Scenario: LLM 调用前有新事件
- **WHEN** engine 准备调用 LLM，且 `DrainNewEvents()` 返回 2 条新事件
- **THEN** 系统 SHALL 将这 2 条事件合并为一条 user message（格式：`[以下 2 条消息同时到达]\n\n<msg1>\n\n<msg2>`）
- **AND** 追加到 messages 列表末尾
- **AND** 然后正常调用 LLM

#### Scenario: LLM 调用前无新事件
- **WHEN** engine 准备调用 LLM，且 `DrainNewEvents()` 返回空
- **THEN** 系统 SHALL 不修改 messages 列表
- **AND** 正常调用 LLM

#### Scenario: 单条事件不添加合并前缀
- **WHEN** engine 准备调用 LLM，且 `DrainNewEvents()` 返回 1 条新事件
- **THEN** 系统 SHALL 将该事件作为一条 user message 追加
- **AND** 不添加 `[以下 N 条消息同时到达]` 前缀

### Requirement: Checkpoint 2 — Tool 执行前 peek 守卫

Engine loop 在 LLM 返回 tool_use 后、实际执行任何 tool 之前，SHALL 调用 `HasNewEvents` 回调检查是否有新事件到达。

#### Scenario: Tool 执行前发现新事件
- **WHEN** LLM 返回了 3 个 tool_use 请求，且 `HasNewEvents()` 返回 `true`
- **THEN** 系统 SHALL 放弃所有 3 个 tool 的执行
- **AND** 为每个 tool_use 生成一条 tool_result，content 为 `"新消息到达，工具未执行。将基于最新信息重新决策。"`，`isError` 为 `true`
- **AND** 将 assistant message（含 tool_use）和 user message（含 abandoned tool_result）追加到 messages
- **AND** `continue` 回到循环顶部（Checkpoint 1 将消费并合并新事件）

#### Scenario: Tool 执行前无新事件
- **WHEN** LLM 返回了 tool_use 请求，且 `HasNewEvents()` 返回 `false`
- **THEN** 系统 SHALL 正常执行所有 tool

#### Scenario: 所有 tool 作为一个原子单位放弃
- **WHEN** LLM 返回了 tool_use A 和 tool_use B，且 `HasNewEvents()` 返回 `true`
- **THEN** tool A 和 tool B SHALL 都不执行
- **AND** 不存在 A 执行了而 B 没执行的中间状态

### Requirement: Tool_use 与 tool_result 配对完整性

无论 tool 是否实际执行，每个 tool_use block SHALL 有对应的 tool_result block，保证 LLM API 的消息格式约束。

#### Scenario: 放弃执行时的配对
- **WHEN** LLM 返回 2 个 tool_use（id=A, id=B），因新事件放弃执行
- **THEN** messages 中 SHALL 包含 tool_result(toolUseId=A) 和 tool_result(toolUseId=B)
- **AND** 两个 tool_result 的 content SHALL 说明放弃原因

### Requirement: 活锁防护

系统 SHALL 设置 `maxConsecutivePreemptions` 阈值（默认 3），连续被新事件抢占超过该阈值后，Checkpoint 2 SHALL 跳过检查，直接执行 tool。

#### Scenario: 连续抢占达到阈值
- **WHEN** 连续 3 轮 Checkpoint 2 检测到新事件并放弃了 tool 执行
- **THEN** 第 4 轮 Checkpoint 2 SHALL 跳过 `HasNewEvents` 检查
- **AND** 直接执行 LLM 返回的 tool

#### Scenario: 成功执行 tool 后计数器归零
- **WHEN** 连续 2 轮被抢占后，第 3 轮成功执行了 tool
- **THEN** `consecutivePreemptions` 计数器 SHALL 重置为 0

### Requirement: AgentLoopConfig 新增事件回调

`AgentLoopConfig` SHALL 新增两个可选字段：
- `DrainNewEvents func() []types.AgentMessage` — 供 Checkpoint 1 调用
- `HasNewEvents func() bool` — 供 Checkpoint 2 调用

当这两个字段为 nil 时，engine SHALL 保持原有行为（无 checkpoint 逻辑），确保向后兼容。

#### Scenario: 回调为 nil 时的向后兼容
- **WHEN** `DrainNewEvents` 和 `HasNewEvents` 均为 nil
- **THEN** engine loop SHALL 与改动前行为完全一致
- **AND** 不执行任何 checkpoint 逻辑

### Requirement: processSession 替代 processOneEvent

Processor 层 SHALL 用 `processSession` 替代 `processOneEvent`，在内部循环中反复调用 `RunAgentLoop`，直到无新事件且 LLM 给出最终回复。

#### Scenario: Engine 退出后仍有事件
- **WHEN** `RunAgentLoop` 正常退出（LLM 给出最终回复），但 `EventLog.HasNew()` 返回 true
- **THEN** `processSession` SHALL 调用 `DrainNew()` 消费事件、合并为 user message
- **AND** 再次调用 `RunAgentLoop`

#### Scenario: Session 处理完成的判定
- **WHEN** `RunAgentLoop` 正常退出且 `EventLog.HasNew()` 返回 false
- **THEN** `processSession` SHALL 退出循环
- **AND** 将 session executionStatus 更新为 `completed`
