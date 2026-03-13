## ADDED Requirements

### Requirement: System prompt 包含四段式主动能力指引

Agent 的 system prompt SHALL 包含一段"主动能力"指引，分为四个子章节：

**设定任务**：引导模型在对话中主动识别需要延后执行的任务场景，包括：
- 用户明确要求定时提醒或定时执行
- 对话中隐含的后续跟进事项
- agent 判断需要定时追踪的承诺
- 没有明确时间的跟进事项（agent 自行选择合理检查时间）

**取消任务**：引导模型在用户明确取消时使用 `cancel_delayed_task`，不确定 task_id 时先用 `list_delayed_tasks` 查看。

**任务到期处理**：引导模型基于事件中的三个时间点（创建时间、计划执行时间、实际触发时间）和对话上下文综合判断：
- 任务仍然有效 → 主动执行
- 情况已变化 → 灵活调整或告知用户
- 实际触发明显晚于计划时间 → 说明延迟情况

**自我续期**：对于持续关注类事项，到期检查后如果事情尚未完结，可再次调用 `set_delayed_task` 设定下一次检查时间。

#### Scenario: Agent 识别到用户的定时提醒请求
- **WHEN** 用户说"明天上午 10 点提醒我开会"
- **THEN** agent 根据 prompt 指引调用 `set_delayed_task` 设定任务，并回复用户确认

#### Scenario: Agent 识别到隐含的后续跟进
- **WHEN** 用户说"这个方案我周五前给你反馈"，agent 判断需要跟进
- **THEN** agent 可主动使用 `set_delayed_task` 设定周五的跟进提醒

#### Scenario: Agent 识别到模糊时间的跟进事项
- **WHEN** 用户说"帮我盯着小王的方案"
- **THEN** agent 自行选择合理的检查时间，设定 `set_delayed_task`

#### Scenario: Agent 取消用户不再需要的任务
- **WHEN** 用户说"那个提醒不用了"
- **THEN** agent 使用 `list_delayed_tasks` 查看待执行任务，确认后调用 `cancel_delayed_task` 取消

#### Scenario: Agent 收到到期事件后结合时间线执行
- **WHEN** agent 收到到期事件，实际触发时间接近计划执行时间，任务仍然适用
- **THEN** agent 主动执行（如发送提醒消息）

#### Scenario: Agent 发现任务延迟触发
- **WHEN** agent 收到到期事件，实际触发时间明显晚于计划执行时间
- **THEN** agent 告知用户延迟情况，判断任务是否仍有意义后决定执行或放弃

#### Scenario: Agent 发现任务到期时用户意图已变化
- **WHEN** agent 收到到期事件，但当前对话上下文显示用户意图已变化
- **THEN** agent 不盲目执行，而是告知用户并说明观察到的变化

#### Scenario: Agent 对持续关注类事项进行自我续期
- **WHEN** agent 收到到期事件，检查后发现事情尚未完结
- **THEN** agent 向用户报告当前状态，并再次调用 `set_delayed_task` 设定下一次检查时间

#### Scenario: Agent 不将到期事件当作用户对话
- **WHEN** agent 收到到期事件
- **THEN** agent 不以"收到您的消息"等对话式回复处理，而是进入任务执行模式
