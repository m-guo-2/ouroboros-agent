## ADDED Requirements

### Requirement: Group config table exists
The system SHALL maintain a `group_configs` SQLite table with columns: `id` (TEXT PK), `agent_id` (TEXT NOT NULL), `session_key` (TEXT NOT NULL), `display_name` (TEXT DEFAULT ''), `system_prompt` (TEXT NULL), `skills` (TEXT NULL), `created_at`, `updated_at`. A unique index SHALL exist on `(agent_id, session_key)`.

#### Scenario: Table created on startup
- **WHEN** the storage layer initializes
- **THEN** the `group_configs` table and its unique index exist in the database

#### Scenario: Duplicate agent+session_key rejected
- **WHEN** a group config is inserted with an `(agent_id, session_key)` pair that already exists
- **THEN** the insert fails with a unique constraint error

### Requirement: Query group config by agent and session key
The system SHALL provide `GetGroupConfig(agentID, sessionKey)` that returns a `*GroupConfig` or nil if no matching row exists. `SystemPrompt` and `Skills` fields SHALL use pointer types to distinguish SQL NULL (not overridden) from empty values (overridden to empty).

#### Scenario: Group config exists
- **WHEN** `GetGroupConfig("agent-1", "qiwei:room123")` is called and a matching row exists with `system_prompt = "custom prompt"` and `skills = '["skill-a"]'`
- **THEN** the returned `GroupConfig.SystemPrompt` points to `"custom prompt"` and `GroupConfig.Skills` points to `["skill-a"]`

#### Scenario: Group config does not exist
- **WHEN** `GetGroupConfig("agent-1", "qiwei:room999")` is called and no matching row exists
- **THEN** the function returns `nil, nil`

#### Scenario: Partial override — only system_prompt set
- **WHEN** a group config row has `system_prompt = "custom"` and `skills = NULL`
- **THEN** `GetGroupConfig` returns `SystemPrompt` pointing to `"custom"` and `Skills` as nil

### Requirement: CRUD operations for group config
The system SHALL provide `CreateGroupConfig`, `UpdateGroupConfig`, `DeleteGroupConfig`, and `ListGroupConfigs` functions following the same patterns as `agent_configs` CRUD in `storage/agents.go`.

#### Scenario: Create group config
- **WHEN** `CreateGroupConfig` is called with `agentID = "agent-1"`, `sessionKey = "qiwei:room123"`, `systemPrompt = "hello"`
- **THEN** a new row is inserted and the created config is returned with a generated `id`

#### Scenario: Update group config system_prompt
- **WHEN** `UpdateGroupConfig(id, {"systemPrompt": "new prompt"})` is called
- **THEN** the row's `system_prompt` is updated and `updated_at` is refreshed

#### Scenario: Update group config skills to null (clear override)
- **WHEN** `UpdateGroupConfig(id, {"skills": null})` is called
- **THEN** the row's `skills` column is set to NULL (removing the override)

#### Scenario: Delete group config
- **WHEN** `DeleteGroupConfig(id)` is called for an existing config
- **THEN** the row is deleted and `true` is returned

#### Scenario: List group configs for agent
- **WHEN** `ListGroupConfigs("agent-1")` is called and 3 group configs exist for that agent
- **THEN** all 3 configs are returned ordered by `created_at DESC`

### Requirement: Runtime override in processor
The `processOneEvent` function SHALL query `GetGroupConfig(agentID, sessionKey)` after loading agent config and before calling `GetSkillsContext`. If a group config exists, its non-nil fields SHALL replace the corresponding fields on `agentConfig` (replace semantics).

#### Scenario: Group has system_prompt override
- **WHEN** agent config has `system_prompt = "default"` and group config has `system_prompt = "custom"`
- **THEN** the LLM receives `"custom"` as the base system prompt

#### Scenario: Group has skills override
- **WHEN** agent config has `skills = ["skill-a", "skill-b"]` and group config has `skills = ["skill-c"]`
- **THEN** `GetSkillsContext` is called with `["skill-c"]` only

#### Scenario: Group has no override (nil fields)
- **WHEN** group config exists but both `system_prompt` and `skills` are NULL
- **THEN** agent default `system_prompt` and `skills` are used unchanged

#### Scenario: No group config exists
- **WHEN** no group config row matches the current `(agentID, sessionKey)`
- **THEN** agent default config is used unchanged, no error

#### Scenario: GetGroupConfig query fails
- **WHEN** `GetGroupConfig` returns an error (e.g., database issue)
- **THEN** processing continues with agent default config, the error is logged as a warning
