## ADDED Requirements

### Requirement: List group configs endpoint
The system SHALL expose `GET /api/agents/{agentId}/groups` that returns all group configs for the given agent, ordered by `created_at DESC`.

#### Scenario: Agent has group configs
- **WHEN** GET `/api/agents/agent-1/groups` is called and 2 group configs exist
- **THEN** response is `200 {success: true, data: [config1, config2]}`

#### Scenario: Agent has no group configs
- **WHEN** GET `/api/agents/agent-1/groups` is called and no group configs exist
- **THEN** response is `200 {success: true, data: []}`

### Requirement: Get single group config endpoint
The system SHALL expose `GET /api/agents/{agentId}/groups/{id}` that returns one group config by its primary key `id`.

#### Scenario: Config exists
- **WHEN** GET `/api/agents/agent-1/groups/gc-abc123` is called and the config exists
- **THEN** response is `200 {success: true, data: {id, agentId, sessionKey, displayName, systemPrompt, skills, ...}}`

#### Scenario: Config not found
- **WHEN** GET `/api/agents/agent-1/groups/nonexistent` is called
- **THEN** response is `404 {success: false, error: "group config not found"}`

### Requirement: Create group config endpoint
The system SHALL expose `POST /api/agents/{agentId}/groups` that creates a new group config. The request body SHALL include `sessionKey` (required), and optionally `displayName`, `systemPrompt`, `skills`.

#### Scenario: Successful creation
- **WHEN** POST `/api/agents/agent-1/groups` with body `{sessionKey: "qiwei:room1", displayName: "客服群", systemPrompt: "你是客服助手"}` is called
- **THEN** response is `201 {success: true, data: {id: "<generated>", agentId: "agent-1", sessionKey: "qiwei:room1", ...}}`

#### Scenario: Missing sessionKey
- **WHEN** POST `/api/agents/agent-1/groups` with body `{displayName: "test"}` is called
- **THEN** response is `400 {success: false, error: "sessionKey is required"}`

#### Scenario: Duplicate session_key for agent
- **WHEN** POST `/api/agents/agent-1/groups` with a `sessionKey` that already has a config for this agent
- **THEN** response is `409 {success: false, error: "group config already exists for this session_key"}`

### Requirement: Update group config endpoint
The system SHALL expose `PUT /api/agents/{agentId}/groups/{id}` that applies partial updates to a group config. Updatable fields: `displayName`, `systemPrompt`, `skills`. Setting a field to `null` in the request body SHALL set the DB column to NULL (removing the override).

#### Scenario: Update system_prompt
- **WHEN** PUT `/api/agents/agent-1/groups/gc-abc` with body `{systemPrompt: "new prompt"}`
- **THEN** response is `200 {success: true, data: {... systemPrompt: "new prompt" ...}}`

#### Scenario: Clear skills override
- **WHEN** PUT `/api/agents/agent-1/groups/gc-abc` with body `{skills: null}`
- **THEN** the config's `skills` field is set to NULL and response includes `skills: null`

#### Scenario: Config not found
- **WHEN** PUT `/api/agents/agent-1/groups/nonexistent` with any body
- **THEN** response is `404 {success: false, error: "group config not found"}`

### Requirement: Delete group config endpoint
The system SHALL expose `DELETE /api/agents/{agentId}/groups/{id}` that removes a group config.

#### Scenario: Successful deletion
- **WHEN** DELETE `/api/agents/agent-1/groups/gc-abc` is called and the config exists
- **THEN** response is `200 {success: true, data: {deleted: true}}`

#### Scenario: Config not found
- **WHEN** DELETE `/api/agents/agent-1/groups/nonexistent` is called
- **THEN** response is `404 {success: false, error: "group config not found"}`

### Requirement: Route registration
The group config routes SHALL be registered in `api.Mount()` alongside existing agent routes.

#### Scenario: Routes are accessible
- **WHEN** the agent server starts
- **THEN** all group config endpoints (`GET/POST/PUT/DELETE /api/agents/{agentId}/groups[/{id}]`) are routable
