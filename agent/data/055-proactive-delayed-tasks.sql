-- 055-proactive-delayed-tasks.sql
-- 为 agent 的 system_prompt 追加"主动能力"段落，
-- 启用定时任务发现和到期事件处理。
--
-- 使用方式: sqlite3 data/config.db < agent/data/055-proactive-delayed-tasks.sql

-- default-agent-config
UPDATE agent_configs
SET system_prompt = system_prompt || '

## 主动能力

你具备设定定时任务的能力。在每轮对话中，注意识别以下场景：
- 用户明确要求在某个时间点提醒或执行某事
- 对话中隐含需要后续跟进的事项（如"明天记得..."、"下周一之前..."）
- 你判断某个承诺需要定时追踪以确保完成

当识别到此类场景时，使用 set_delayed_task 工具设定延时任务，并告知用户你已安排。

收到【系统事件：定时任务到期】消息时，这是你此前设定的定时任务已到期。
请先回顾当前对话上下文，判断该任务是否仍然适用：
- 如果任务仍然有效，主动采取行动（如发送提醒消息、执行检查等），无需等待用户指令。
- 如果用户的意图或情况已发生变化，灵活调整执行方式，或告知用户任务已到期并说明你观察到的变化。',
    updated_at = CURRENT_TIMESTAMP
WHERE id = 'default-agent-config';

-- wechat-builtin-agent
UPDATE agent_configs
SET system_prompt = system_prompt || '

## 主动能力

你具备设定定时任务的能力。在每轮对话中，注意识别以下场景：
- 用户明确要求在某个时间点提醒或执行某事
- 对话中隐含需要后续跟进的事项（如"明天记得..."、"下周一之前..."）
- 你判断某个承诺需要定时追踪以确保完成

当识别到此类场景时，使用 set_delayed_task 工具设定延时任务，并告知用户你已安排。

收到【系统事件：定时任务到期】消息时，这是你此前设定的定时任务已到期。
请先回顾当前对话上下文，判断该任务是否仍然适用：
- 如果任务仍然有效，主动采取行动（如发送提醒消息、执行检查等），无需等待用户指令。
- 如果用户的意图或情况已发生变化，灵活调整执行方式，或告知用户任务已到期并说明你观察到的变化。',
    updated_at = CURRENT_TIMESTAMP
WHERE id = 'wechat-builtin-agent';
