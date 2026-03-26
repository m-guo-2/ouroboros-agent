## ADDED Requirements

### Requirement: Persona persistence
系统 SHALL 提供 `agent_personas` 表存储 Persona 定义。每条记录包含 `id`、`agent_id`、`display_name`，以及可选覆盖字段：`system_prompt`、`provider`、`model`、`skills`（JSON 数组）、`subagent_models`（JSON 对象）、`subagent_skills`（JSON 对象）。覆盖字段 NULL 表示不覆盖，回退到 agent 默认值。

#### Scenario: Create persona with partial overrides
- **WHEN** 调用 `CreatePersona` 传入 agent_id、display_name="客服助手"、system_prompt="你是客服"，其余覆盖字段为 nil
- **THEN** 系统创建记录，system_prompt 存储传入值，其余覆盖列为 NULL

#### Scenario: Create persona with full overrides
- **WHEN** 调用 `CreatePersona` 传入所有覆盖字段
- **THEN** 系统创建记录，所有覆盖列均存储传入值

#### Scenario: Update persona
- **WHEN** 调用 `UpdatePersona` 更新 skills 字段
- **THEN** 仅 skills 列更新，其他字段不变，updated_at 更新

#### Scenario: Clone persona
- **WHEN** 调用 `ClonePersona` 传入已有 persona ID 和新名称
- **THEN** 系统创建一条新记录，复制原 persona 的所有覆盖字段，使用新名称和新 ID

#### Scenario: Delete persona without group references
- **WHEN** 调用 `DeletePersona`，该 persona 无群分配引用
- **THEN** 系统删除记录

#### Scenario: Delete persona with group references
- **WHEN** 调用 `DeletePersona`，该 persona 有群分配引用
- **THEN** 系统返回错误，拒绝删除

#### Scenario: List personas for agent
- **WHEN** 调用 `ListPersonas(agentID)`
- **THEN** 返回该 agent 的所有 Persona，按 created_at 升序

### Requirement: Group assignment persistence
系统 SHALL 提供 `group_persona_assignments` 表，以 `(agent_id, session_key)` 为唯一键存储群到 Persona 的映射。`persona_id` 为空字符串表示管理员确认使用默认配置。

#### Scenario: Assign persona to group
- **WHEN** 调用 `CreateGroupAssignment` 传入 agent_id、session_key、group_name、persona_id="ps-001"
- **THEN** 系统创建分配记录

#### Scenario: Assign default to group (ignore)
- **WHEN** 调用 `CreateGroupAssignment` 传入 persona_id=""
- **THEN** 系统创建记录，persona_id 为空字符串（表示确认使用默认）

#### Scenario: Change group persona
- **WHEN** 调用 `UpdateGroupAssignment` 将 persona_id 从 "ps-001" 改为 "ps-002"
- **THEN** 该群下次请求使用新 Persona 的配置

#### Scenario: Unique constraint
- **WHEN** 已存在相同 agent_id + session_key 的分配
- **THEN** 创建返回错误

#### Scenario: Delete group assignment
- **WHEN** 调用 `DeleteGroupAssignment`
- **THEN** 该群回到"未分配"状态，从 discover 中重新出现

### Requirement: Unconfigured group discovery
系统 SHALL 提供 `ListUnconfiguredGroups(agentID)` 函数，从 `agent_sessions` 中提取已出现但未在 `group_persona_assignments` 中的群聊 session_key，过滤私聊。

#### Scenario: Discover new groups
- **WHEN** agent 有 5 个群聊 session，其中 3 个已有分配记录
- **THEN** 返回 2 条未分配群记录，含 session_key、渠道类型、最近活跃时间

#### Scenario: No unconfigured groups
- **WHEN** 所有群聊 session 均有分配记录
- **THEN** 返回空切片

### Requirement: Runtime persona application
系统 SHALL 在 `processSession` 中，`GetAgentConfig` 之后、`GetSkillsContext` 之前，执行两步查询（session_key → assignment → persona）并将 Persona 非 NULL 字段 replace 到 agentConfig 内存副本。

#### Scenario: Group assigned to persona with full overrides
- **WHEN** 群分配到 Persona，Persona 所有覆盖字段非 NULL
- **THEN** agentConfig 全部对应字段被 Persona 值替换

#### Scenario: Group assigned to persona with partial overrides
- **WHEN** Persona 仅 system_prompt 非 NULL
- **THEN** 仅 SystemPrompt 被替换，其余保持 agent 默认

#### Scenario: Model override requires both provider and model
- **WHEN** Persona provider 非 NULL 但 model 为 NULL
- **THEN** provider 和 model 均不覆盖

#### Scenario: Group assigned to default (persona_id empty)
- **WHEN** 群的 assignment 存在但 persona_id 为空字符串
- **THEN** agentConfig 不修改，使用默认配置

#### Scenario: Group not assigned
- **WHEN** 群无 assignment 记录
- **THEN** agentConfig 不修改，使用默认配置

#### Scenario: Query failure
- **WHEN** assignment 或 persona 查询失败
- **THEN** 静默降级到默认配置，记录 warn 日志

#### Scenario: Persona count on listing
- **WHEN** 调用 `ListPersonas` 或 `GetPersona`
- **THEN** 返回的 Persona 数据 SHALL 包含该 Persona 被分配的群数量（`groupCount`）
