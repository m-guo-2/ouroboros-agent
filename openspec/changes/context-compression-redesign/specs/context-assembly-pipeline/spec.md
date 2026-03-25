## ADDED Requirements

### Requirement: 保留策略——最近 N 个用户事件锚点
`CompactContext` SHALL 使用"最近 N 个用户 text turn"作为保留锚点（默认 N=5），保留这些 turn 及其之后的所有 assistant/tool 消息。保留消息总数 SHALL NOT 超过压缩前总消息数的 50%。

#### Scenario: 正常压缩——保留最近 5 个用户 turn
- **WHEN** 对话包含 20 个用户 text turn，token ratio 超过 0.60 触发压缩
- **THEN** 系统保留第 16-20 个用户 turn 及其后续的 assistant/tool 消息
- **THEN** 前 15 个 turn 及其后续消息被归档

#### Scenario: 50% 上限约束
- **WHEN** 对话总计 8 条消息，最近 5 个用户 turn 对应 6 条消息（超过 50%）
- **THEN** 系统从最近的 turn 开始向前收集，直到保留消息数达到总数的 50%（4 条）

#### Scenario: 用户 turn 不足 N 个
- **WHEN** 对话仅包含 3 个用户 text turn
- **THEN** 保留全部 3 个 turn 及其后续消息，不触发归档（因为没有可归档的内容）

### Requirement: 四层上下文组装
压缩完成后，`context.messages` SHALL 被替换为四层结构，按以下顺序组装：
1. 摘要层：5 维度结构化摘要（`role: user`）+ assistant 确认消息
2. 近期消息层：经 orphan 检测和 tool_result 截断后的保留消息，按原顺序排列
3. 任务状态层：LLM 提取的未完成任务（`role: user`）+ assistant 确认消息
4. Agent 记忆层：重新加载的 session facts（`role: user`）+ assistant 确认消息

#### Scenario: 完整四层组装
- **WHEN** 压缩触发，归档消息已生成摘要，有未完成任务被提取，session facts 中有 3 条事实
- **THEN** 新 context 的消息序列为：`[摘要 user, ack, ...retained..., task user, ack, facts user, ack]`

#### Scenario: 无未完成任务时退化为三层
- **WHEN** 压缩触发，但 LLM 未提取到任何未完成任务
- **THEN** 新 context 跳过任务状态层：`[摘要 user, ack, ...retained..., facts user, ack]`

#### Scenario: 无 session facts 时退化
- **WHEN** 压缩触发，但 session facts 为空
- **THEN** 新 context 跳过 agent 记忆层

#### Scenario: 任务提取失败时退化
- **WHEN** 任务提取的 LLM 调用失败
- **THEN** 新 context 跳过任务状态层，不影响其他层的正常组装

#### Scenario: 保留消息的 orphan 检测
- **WHEN** 保留的消息中存在孤儿 tool_use（对应的 tool_result 在归档区）或孤儿 tool_result（对应的 tool_use 在归档区）
- **THEN** 这些孤儿块 SHALL 被移除
- **THEN** 移除孤儿后若某条消息的 content 为空，该消息 SHALL 被整体移除

### Requirement: 结构化摘要合约（5 维度）
摘要 LLM SHALL 按 5 个维度生成结构化摘要，每个维度对应一个明确的信息类别。

#### Scenario: 摘要 prompt 的 5 维度
- **WHEN** 系统调用 LLM 生成摘要
- **THEN** user prompt 要求按以下 5 个维度逐项输出：
  1. 目标/项目：用户在做什么
  2. 当前状态：做到哪一步了
  3. 关键决策：选了什么方案、为什么
  4. 失败/回退：什么试过不行
  5. 未完成事项：归档区有什么没做完的
- **THEN** system prompt 为中文："你是一个对话摘要助手，负责将长对话压缩成结构化的摘要"
- **THEN** 摘要上限为 200 词

#### Scenario: 摘要不包含用户偏好
- **WHEN** 对话中出现用户偏好信息（如"用中文回复"、"代码要简洁"）
- **THEN** 摘要 SHALL NOT 将偏好作为独立维度记录
- **THEN** 偏好信息由 session_facts（或未来的用户画像）负责持久化

#### Scenario: 摘要 LLM 失败降级
- **WHEN** LLM 调用失败或返回空内容
- **THEN** 使用 fallback 摘要（统计信息 + 用户消息片段），与现有降级逻辑一致

### Requirement: 摘要层消息格式
摘要消息 SHALL 以 `[历史上下文摘要]` 为前缀，包含 LLM 生成的结构化摘要文本和归档统计信息。

#### Scenario: 摘要消息内容
- **WHEN** 归档了 15 条消息，LLM 生成了摘要文本
- **THEN** 摘要消息的 content 为：`[历史上下文摘要]\n之前的对话（{N} 条消息已归档）：\n\n{summary}\n\n---\n完整历史可通过 recall_context 工具检索。`

#### Scenario: 摘要后的 assistant 确认
- **WHEN** 摘要消息已插入
- **THEN** 紧跟一条 `role: assistant` 消息，content 为确认文本，维持 user/assistant 交替

### Requirement: Agent 记忆层消息格式
Agent 记忆层 SHALL 重新从 `session_facts` 加载事实，而非保留压缩前已存在的 facts 消息。

#### Scenario: session facts 重新加载
- **WHEN** 压缩触发时 session_facts 表中有 5 条事实
- **THEN** agent 记忆层从 session_facts 表重新读取全部事实
- **THEN** 构造 `role: user` 消息，content 以 `[Agent Memory]` 为前缀
- **THEN** 紧跟 `role: assistant` 确认消息

#### Scenario: facts token 预算控制
- **WHEN** session facts 总量超过 context window 的 10%
- **THEN** 从最新的 fact 开始向前取，直到 token 预算耗尽
- **THEN** 较老的 facts 被省略

### Requirement: 大 tool_result 截断
保留在近期消息层的 tool_result 内容超过 1024 字节时 SHALL 被截断。

#### Scenario: tool_result 截断
- **WHEN** 保留消息中某 tool_result 的 content 长度超过 1024
- **THEN** content 截断为前 1024 字节 + 提示文本 `\n...[truncated, use recall_context to retrieve full content]`
