## ADDED Requirements

### Requirement: 压缩前任务状态提取
压缩流程 SHALL 在生成摘要之前（或之后），调用 LLM 从被归档的消息中提取未完成任务，返回结构化的任务列表文本。

#### Scenario: 正常提取
- **WHEN** 被归档消息中包含用户要求"帮我做 A、B、C"，其中 A 已完成、B 在进行中
- **THEN** LLM 提取结果包含任务列表，标注完成状态和来源
- **THEN** 提取结果作为文本返回给调用方

#### Scenario: 无未完成任务
- **WHEN** 被归档消息中所有任务均已完成，无进行中或待办任务
- **THEN** 返回空结果
- **THEN** 调用方不注入任务状态层

#### Scenario: 提取 LLM 调用失败
- **WHEN** LLM 调用失败或超时
- **THEN** 返回空结果
- **THEN** 日志记录失败信息
- **THEN** 压缩流程不中断

### Requirement: 任务状态消息格式
任务状态层 SHALL 使用 `[任务状态]` 前缀的 `role: user` 消息，内容包含当前目标和任务列表。

#### Scenario: 任务状态消息内容
- **WHEN** LLM 提取到 3 个未完成任务
- **THEN** 消息 content 格式为：
  ```
  [任务状态]
  当前目标: <目标描述>

  未完成任务:
  - [ ] <task 1> (来源: user)
  - [ ] <task 2> (来源: agent)
  - [x] <task 3> (已完成)
  ```
- **THEN** 紧跟 `role: assistant` 确认消息

### Requirement: 区分任务来源
任务提取 SHALL 区分用户发起的任务（user）和 agent 自主推断的任务（agent）。

#### Scenario: 用户发起的任务
- **WHEN** 用户在对话中明确说"帮我做 X"或"请处理 Y"
- **THEN** 提取的任务标记为 `(来源: user)`

#### Scenario: Agent 推断的任务
- **WHEN** agent 在对话中自行决定"接下来应该检查 X"或"改完后需要跑测试"
- **THEN** 提取的任务标记为 `(来源: agent)`

### Requirement: 任务提取使用便宜模型
任务提取 SHALL 使用与摘要相同的便宜模型（通过 `ResolveCompactModel` 解析），不使用主 agent 模型。

#### Scenario: 模型选择
- **WHEN** 主 agent 使用 claude-sonnet-4-5
- **THEN** 任务提取使用 claude-3-5-haiku-20241022（或其他 compact model 映射）

### Requirement: 任务提取的 LLM prompt
任务提取 SHALL 使用中文 system prompt，user prompt 要求区分任务来源和完成状态。

#### Scenario: prompt 内容
- **WHEN** 系统调用 LLM 提取任务
- **THEN** system prompt 为："你是一个任务提取助手，从对话中识别未完成的任务和目标"
- **THEN** user prompt 要求：
  1. 识别当前用户的主要目标
  2. 列出所有任务及其状态（待办/进行中/已完成）
  3. 标注每个任务的来源（用户明确要求 vs agent 自行推断）
  4. 如果没有未完成任务，回复 NO_TASKS
