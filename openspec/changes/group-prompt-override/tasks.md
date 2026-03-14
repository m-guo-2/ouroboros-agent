## 1. Storage Layer

- [ ] 1.1 在 `storage/db.go` 的 migrations 中添加 `group_configs` 建表 DDL 和唯一索引
- [ ] 1.2 在 `storage/types.go` 中添加 `GroupConfig` 结构体（SystemPrompt/Skills 用指针类型）
- [ ] 1.3 新建 `storage/group_configs.go`，实现 `GetGroupConfig(agentID, sessionKey)`
- [ ] 1.4 在 `storage/group_configs.go` 中实现 `ListGroupConfigs`、`GetGroupConfigByID`、`CreateGroupConfig`、`UpdateGroupConfig`、`DeleteGroupConfig`

## 2. Runtime Override

- [ ] 2.1 在 `runner/processor.go` 的 `processOneEvent` 中，`GetAgentConfig` 之后、`GetSkillsContext` 之前，调用 `GetGroupConfig` 并执行 replace 覆盖
- [ ] 2.2 覆盖逻辑：仅替换非 nil 字段，查询失败时 log warning 并继续使用 agent 默认配置

## 3. Admin API

- [ ] 3.1 新建 `api/group_configs.go`，实现 `handleAgentGroups`（GET list + POST create）和 `handleAgentGroupsWithID`（GET/PUT/DELETE by id）
- [ ] 3.2 在 `api/router.go` 的 `Mount` 函数中注册 `/api/agents/{agentId}/groups` 和 `/api/agents/{agentId}/groups/{id}` 路由
