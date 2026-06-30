-- +goose Up
-- +goose StatementBegin

INSERT INTO skills
    (id, name, description, version, type, enabled, readme, metadata, created_at, updated_at, deleted_at)
VALUES (
    'icebreaker',
    '破冰引导',
    '在新群、新会话或当前会话发现新参与者时，做一次轻量、自然、可自动退出的破冰引导',
    '1.0.0',
    'knowledge',
    1,
    '## 破冰引导\n\n你只在开场破冰阶段工作。目标是让新群或新参与者顺利进入对话，不要长期主导话题。\n\n### 适用场景\n\n- `group_joined`：你刚进入一个群，需要做一次简短开场。\n- `participant_discovered`：某个用户第一次出现在当前会话或群里。\n- `session_started`：一个新会话刚开始。\n\n系统可能在用户消息头里提供 `participant_discovered relation=first_seen` 或 `participant_discovered relation=seen_before`：\n\n- `first_seen` 表示这个 agent 以前没有见过这个人，可以稍微完整地介绍自己。\n- `seen_before` 表示以前见过这个人，只做轻量招呼，不要重复自我介绍。\n\n### 行为规则\n\n- 只发送一条短消息，像同事间自然打招呼。\n- 不写标题，不写列表，不解释你在执行破冰 skill。\n- 不要追问超过一个问题。\n- 如果用户已经直接提出业务问题，优先回答业务问题，不要强行寒暄。\n- 如果是 `seen_before`，不要说“初次见面”。\n- 如果无法判断对方身份，就轻问一句对方希望你怎么帮忙。\n\n### 推荐话术方向\n\n`first_seen`：\n- 简短说明你能帮忙处理消息、查信息、整理事项。\n- 问一句“你希望我先帮你看什么？”这类开放但轻的问题。\n\n`seen_before`：\n- 简短承接，不重复介绍。\n- 可以说“我在，有事直接发我。”这类低打扰表达。\n\n`group_joined`：\n- 简单说明你已在群里，后续可以帮忙查信息、整理结论、跟进事项。\n- 不主动艾特所有人，不制造仪式感。\n\n### 退出要求\n\n完成一次破冰消息后，必须调用 `complete_skill`，参数为：\n\n```json\n{\"skill_id\":\"icebreaker\"}\n```\n\n如果判断当前消息不需要破冰，也要直接调用 `complete_skill`，不要继续占用后续上下文。',
    '{}',
    0, 0, 0
)
ON DUPLICATE KEY UPDATE
    name = VALUES(name),
    description = VALUES(description),
    version = VALUES(version),
    type = VALUES(type),
    enabled = VALUES(enabled),
    readme = VALUES(readme),
    metadata = VALUES(metadata),
    updated_at = VALUES(updated_at),
    deleted_at = 0;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DELETE FROM skills WHERE id = 'icebreaker';

-- +goose StatementEnd
