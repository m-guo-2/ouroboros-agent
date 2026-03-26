## Why

当前 agent 配置粒度为 per-agent：同一个 agent 下所有群聊和私聊共享同一套 system prompt、model、skills 和 subagent 配置。实际场景中，一个企业往往只有一个 bot 账号（企微/飞书限制），但不同群的使用场景差异很大——客服群需要产品知识 + 温和语气、技术群需要代码能力 + 严谨风格、运营群需要数据分析 + 报表工具。目前只能创建多个 agent 并在 channel 侧手动路由，管理成本高且无法在单账号下实现。

核心问题：**一个账号 + 一个 agent 无法在不同群展现不同能力**。

## What Changes

- 引入 **Persona（人设）** 概念：一个 Persona 是一组命名的行为配置（system_prompt + model + skills + subagent_models + subagent_skills），定义 agent 在特定场景下的能力和风格
- 新增 `agent_personas` 存储层，支持为每个 agent 创建多个 Persona
- 新增 `group_persona_assignments` 存储层，支持将群分配到指定 Persona
- 修改 processor 请求处理流程：加载 agent 配置后，通过群分配查找 Persona 并 replace 对应字段
- Admin UI 新增 Persona 管理界面和群分配界面，自动从 session 中发现新出现的群

## Capabilities

### New Capabilities
- `persona-store`: Persona 和群分配的存储——`agent_personas` 表（Persona 定义）、`group_persona_assignments` 表（群→Persona 映射）、CRUD 操作、运行时覆盖合并逻辑
- `persona-api`: Admin HTTP API——Persona CRUD、群分配 CRUD、未分配群发现
- `persona-ui`: Admin 前端——Persona 管理（创建/编辑/复制/删除）、群分配（列表 + 下拉选择 + 未分配群队列）

### Modified Capabilities

## Impact

- **Storage**: `agent/internal/storage/` — 新增 `agent_personas` 和 `group_persona_assignments` 表、对应结构体和 CRUD 函数；新增从 `agent_sessions` 提取未分配群的查询
- **Runner**: `agent/internal/runner/processor.go` — 在 `processSession` 中查找群分配的 Persona 并覆盖 agent 默认值
- **Admin API**: `agent/internal/api/` — 新增 Persona 管理和群分配端点
- **Admin UI**: `admin/src/` — 新增 Persona 管理页面和群分配页面
- **DB Migration**: `agent/internal/storage/db.go` — 新增两张表的建表 DDL 和索引
