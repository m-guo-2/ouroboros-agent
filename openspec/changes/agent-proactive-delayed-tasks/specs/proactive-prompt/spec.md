## ADDED Requirements

### Requirement: System prompt 包含后续任务发现指引

Agent 的 system prompt SHALL 包含一段"主动能力"指引，引导模型在对话中主动识别需要延后执行的任务场景，并使用 `set_delayed_task` 工具设定延时任务。

指引 SHALL 覆盖以下场景类型：
- 用户明确要求定时提醒或定时执行
- 对话中隐含的后续跟进事项
- agent 判断需要定时追踪的承诺

#### Scenario: Agent 识别到用户的定时提醒请求
- **WHEN** 用户说"明天上午 10 点提醒我开会"
- **THEN** agent 根据 prompt 指引调用 `set_delayed_task` 设定任务，并回复用户确认

#### Scenario: Agent 识别到隐含的后续跟进
- **WHEN** 用户说"这个方案我周五前给你反馈"，agent 判断需要跟进
- **THEN** agent 可主动使用 `set_delayed_task` 设定周五的跟进提醒

### Requirement: System prompt 包含到期事件处理指引

Agent 的 system prompt SHALL 包含到期事件的处理指引，明确说明：
- `【系统事件：定时任务到期】` 格式的消息是 agent 此前自主设定的定时任务
- 收到此类消息时应先回顾当前对话上下文，判断任务是否仍然适用
- 如果任务仍有效，主动执行；如果用户意图或情况已变化，灵活调整或告知用户

#### Scenario: Agent 收到到期事件后结合现状执行
- **WHEN** agent 收到以 `【系统事件：定时任务到期】` 开头的消息，且任务仍然适用
- **THEN** agent 根据 prompt 指引理解这是自己此前设定的任务，结合当前上下文主动执行（如发送提醒消息）

#### Scenario: Agent 发现任务到期时用户意图已变化
- **WHEN** agent 收到到期事件，但当前对话上下文显示用户意图已发生变化（如用户已自行完成、已取消计划等）
- **THEN** agent 不盲目执行原始任务，而是告知用户"你此前设定的 XX 任务已到期"，并说明观察到的变化，询问是否仍需执行

#### Scenario: Agent 不将到期事件当作用户对话
- **WHEN** agent 收到到期事件
- **THEN** agent 不以"收到您的消息"等对话式回复处理，而是进入任务执行模式
