## ADDED Requirements

### Requirement: List personas
系统 SHALL 提供 `GET /api/agents/{agentId}/personas`，返回所有 Persona（含关联群数量）。

#### Scenario: List with entries
- **WHEN** agent 有 2 个 Persona
- **THEN** 返回 `{success: true, data: [Persona, ...]}` 每个含 `groupCount`

#### Scenario: List empty
- **WHEN** agent 无 Persona
- **THEN** 返回 `{success: true, data: []}`

### Requirement: Get persona
系统 SHALL 提供 `GET /api/agents/{agentId}/personas/{id}`。

#### Scenario: Get existing
- **WHEN** id 有效
- **THEN** 返回完整 Persona 数据（含 groupCount）

#### Scenario: Get non-existing
- **WHEN** id 无效
- **THEN** 返回 404

### Requirement: Create persona
系统 SHALL 提供 `POST /api/agents/{agentId}/personas`。displayName 为必填。

#### Scenario: Create with overrides
- **WHEN** body 含 displayName 和 systemPrompt
- **THEN** 返回 201

#### Scenario: Missing displayName
- **WHEN** body 缺少 displayName
- **THEN** 返回 400

### Requirement: Clone persona
系统 SHALL 提供 `POST /api/agents/{agentId}/personas/{id}/clone`。

#### Scenario: Clone existing
- **WHEN** body 含新 displayName
- **THEN** 返回 201，新 Persona 复制原始所有覆盖字段

#### Scenario: Clone non-existing
- **WHEN** 原始 id 无效
- **THEN** 返回 404

### Requirement: Update persona
系统 SHALL 提供 `PUT /api/agents/{agentId}/personas/{id}`，支持部分更新。

#### Scenario: Update field
- **WHEN** body 含 `{"systemPrompt": "新提示词"}`
- **THEN** 仅更新 system_prompt

#### Scenario: Clear override
- **WHEN** body 含 `{"systemPrompt": null}`
- **THEN** system_prompt 设为 NULL（回退默认）

### Requirement: Delete persona
系统 SHALL 提供 `DELETE /api/agents/{agentId}/personas/{id}`。有群引用时拒绝删除。

#### Scenario: Delete unreferenced
- **WHEN** Persona 无群分配引用
- **THEN** 返回 200

#### Scenario: Delete referenced
- **WHEN** Persona 有群分配引用
- **THEN** 返回 409，说明需先解除群分配

### Requirement: List group assignments
系统 SHALL 提供 `GET /api/agents/{agentId}/groups`，返回所有群分配。

#### Scenario: List assignments
- **WHEN** agent 有 4 条群分配
- **THEN** 返回 `{success: true, data: [GroupAssignment, ...]}`

### Requirement: Discover unconfigured groups
系统 SHALL 提供 `GET /api/agents/{agentId}/groups/discover`。

#### Scenario: Discover new groups
- **WHEN** 有 2 个未分配群
- **THEN** 返回列表含 session_key、渠道类型、最近活跃时间

### Requirement: Create group assignment
系统 SHALL 提供 `POST /api/agents/{agentId}/groups`。sessionKey 必填。

#### Scenario: Assign persona
- **WHEN** body 含 sessionKey、groupName、personaId
- **THEN** 返回 201

#### Scenario: Assign default (ignore)
- **WHEN** body 含 sessionKey、groupName，personaId 为空字符串
- **THEN** 返回 201，表示确认使用默认

#### Scenario: Duplicate sessionKey
- **WHEN** sessionKey 已存在
- **THEN** 返回 409

### Requirement: Update group assignment
系统 SHALL 提供 `PUT /api/agents/{agentId}/groups/{id}`。

#### Scenario: Switch persona
- **WHEN** body 含新 personaId
- **THEN** 更新分配

### Requirement: Delete group assignment
系统 SHALL 提供 `DELETE /api/agents/{agentId}/groups/{id}`。

#### Scenario: Delete assignment
- **WHEN** 删除有效 id
- **THEN** 返回 200，该群回到"未分配"状态
