## Why

当前子 agent（subagent）启动时，直接继承主 agent 的全量工具列表（`registry.GetAll()`），然后由 `filterToolsByProfile` 按 profile 硬编码的白名单过滤。Skill 带来的工具和上下文信息只在主 agent 中生效，子 agent 无法使用任何 skill。

这意味着：如果某个 skill 提供了对子 agent 有用的工具（如 developer subagent 需要的代码审查 skill、web_research subagent 需要的领域知识 skill），管理员无法将 skill 独立分配给特定 profile 的 subagent。限制了 subagent 的能力扩展性。

## What Changes

- 新增 `subagentSkills` 配置字段，允许管理员为每个 subagent profile 绑定独立的 skill 列表
- Runner 启动子 agent 时，根据 profile 查找对应的 skill 绑定，编译出专属的 SkillContext
- 子 agent 的 system prompt 中注入绑定 skill 的文档，工具列表中注入 skill 提供的工具
- Admin UI 中新增子 agent skill 配置界面

## Capabilities

### New Capabilities
- `subagent-skill-binding`: 为每个 subagent profile 独立配置 skill 绑定（存储、API、Runner 解析、UI）

### Modified Capabilities

## Impact

- **存储层**: `agent_configs` 表新增 `subagent_skills` JSON 列
- **数据模型**: `AgentConfig` 新增 `SubagentSkills` 字段
- **API**: `PUT /api/agents/{id}` 和 `GET` 响应扩展 `subagentSkills` 字段
- **Runner**: `processor.go` 中子 agent 启动逻辑需要编译 profile 专属的 SkillContext 并注入到 subagent
- **Subagent Manager**: `StartRequest` 需要扩展以接收 skill 上下文信息
- **Admin UI**: `agent-detail.tsx` 新增子 agent skill 配置 section
