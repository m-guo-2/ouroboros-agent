## MODIFIED Requirements

### Requirement: 子 agent 外层循环与上下文压缩
`run()` SHALL 实现外层循环：当 `RunAgentLoop` 因命中 MaxIterations 退出时，对 messages 执行 `CompactContext`（LLM 摘要 + 工具结果截断 + 任务提取 + 四层上下文组装），然后带着压缩后的 messages 重新进入 `RunAgentLoop`。

#### Scenario: 子 agent 单轮内完成任务
- **WHEN** 子 agent 在 25 次迭代内产出 FinalText
- **THEN** 外层循环仅执行一轮，不触发压缩，正常完成

#### Scenario: 子 agent 命中 MaxIterations 触发压缩和 re-entry
- **WHEN** 子 agent 执行了 25 次迭代仍未完成（`HitMaxIterations=true`），且 token 估算超过触发阈值
- **THEN** 系统执行 `CompactContext` 对 messages 进行压缩
- **THEN** 压缩使用"最近 5 个用户 text turn"锚点保留策略
- **THEN** 压缩结果采用四层组装结构（摘要层 + 近期消息层 + 任务状态层 + agent 记忆层）
- **THEN** 带着压缩后的 messages 重新进入 `RunAgentLoop`

#### Scenario: 子 agent 命中 MaxIterations 但 token 未超阈值
- **WHEN** 子 agent 命中 MaxIterations，但 token 估算未超过触发阈值（ratio ≤ 0.60）
- **THEN** 不执行 `CompactContext`，直接带当前 messages 重新进入 `RunAgentLoop`

#### Scenario: 子 agent 达到最大 re-entry 次数
- **WHEN** 子 agent 已 re-entry 3 次（共执行 4 轮 RunAgentLoop），第 4 轮仍命中 MaxIterations
- **THEN** 外层循环终止，子 agent 以当前已有结果完成任务

### Requirement: 子 agent 压缩不执行 FlushMemoryBeforeCompact
子 agent 的压缩流程 SHALL NOT 调用 `FlushMemoryBeforeCompact`。子 agent 的"记忆"通过 `Job.Result` 返回给主 agent，不写入 session memory。

#### Scenario: 子 agent 压缩流程
- **WHEN** 子 agent 外层循环触发 `CompactContext`
- **THEN** 仅执行 `CompactContext`（含任务提取），不执行 `FlushMemoryBeforeCompact`

### Requirement: 子 agent 压缩的 agent 记忆层
子 agent 的 `CompactContext` 中 agent 记忆层 SHALL 加载主 session 的 session facts（使用主 session 的 `sessionID`）。

#### Scenario: 子 agent 压缩后注入 session facts
- **WHEN** 子 agent 触发压缩，主 session 有 session facts
- **THEN** 压缩后的四层结构中，第四层从主 session 的 session_facts 加载
- **THEN** facts 格式与主 agent 一致

#### Scenario: 子 agent 压缩但主 session 无 facts
- **WHEN** 子 agent 触发压缩，主 session 的 session_facts 为空
- **THEN** 压缩后该层不注入
