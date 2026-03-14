## Why

当前 system prompt 和 skills 配置粒度为 per-agent：同一个 agent 下所有群聊和私聊共享同一套 system prompt 与 skills。不同群的使用场景差异大（客服群需要产品知识、技术群需要代码能力、内部群需要运维工具），无法按群定制 agent 行为，只能通过创建多个 agent 来变通，管理成本高。

## What Changes

- 新增 `group_configs` 存储层，支持按 `(agent_id, session_key)` 粒度覆盖 `system_prompt` 和 `skills`
- 修改请求处理流程：在加载 agent 配置后，查询群级覆盖配置并 replace 对应字段
- 新增 Admin API 端点，用于增删改查群级配置
- Admin 前端新增群配置管理界面（可后续跟进）

## Capabilities

### New Capabilities
- `group-config-store`: 群级配置的存储与查询，包括 DB schema、CRUD 操作、与 agent 配置的 replace 合并逻辑
- `group-config-api`: 群级配置的 Admin HTTP API，支持按 agent + session_key 管理覆盖配置

### Modified Capabilities

## Impact

- **Storage**: `agent/internal/storage/` — 新增 `group_configs` 表和相关查询函数
- **Runner**: `agent/internal/runner/processor.go` — 在 `processOneEvent` 中加载群配置并覆盖 agent 默认值
- **Admin API**: `agent/internal/admin/` — 新增群配置管理端点
- **DB Migration**: `agent/internal/storage/db.go` — 新增建表 DDL
