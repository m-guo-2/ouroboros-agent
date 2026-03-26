## Context

当前子 agent 通过 `subagent.StartRequest` 启动时，从主 agent 的 `registry.GetAll()` 拿到全量工具列表，再由 `filterToolsByProfile` 按 profile 硬编码白名单过滤。Skill 系统（`GetSkillsContext`）只在主 agent 的 `processOneMessage` 中被调用，编译出 `SkillContext`（system prompt 片段 + 工具定义 + 执行器），子 agent 对此完全无感知。

`sub-agent-model-config` 已为 `agent_configs` 添加了 `subagent_models` JSON 列和 `SubagentModelConfig`，本次沿用相同模式扩展 skill 配置。

关键约束：
- `SkillBinding` 已有 `id` + `mode`（always / on_demand）结构
- `GetSkillsContext(agentID, []SkillBinding)` 接受任意 binding 列表，不耦合 agent_configs 表
- 子 agent 工具过滤逻辑在 `filterToolsByProfile` 中，需要与 skill 工具合并

## Goals / Non-Goals

**Goals:**
- 管理员可以在 admin UI 中为每个 subagent profile 绑定独立的 skill 列表
- 未配置的 profile 不加载任何额外 skill（零配置向后兼容）
- Runner 启动子 agent 时根据 profile 查找对应的 skill 绑定，编译 SkillContext，将 skill 的工具和文档注入子 agent
- 子 agent 的 system prompt 中包含绑定 skill 的文档片段

**Non-Goals:**
- 不做 per-session 或 per-task 级别的 skill 覆盖
- 不修改 skill 系统本身（skill 存储、skill 发现、load_skill 机制）
- 不允许子 agent 的 skill 工具绕过 profile 级别的工具过滤（skill 工具直接注入，不受 `filterToolsByProfile` 白名单约束）
- 不支持子 agent 使用 on_demand 模式的 skill（子 agent 无 load_skill 交互能力，绑定的 skill 统一以 always 模式加载）

## Decisions

### D1: 存储方案 — `agent_configs` 表新增 `subagent_skills` JSON 列

在 `agent_configs` 表新增 `subagent_skills TEXT DEFAULT '{}'`，以 JSON 存储 profile → skill ID 数组的映射。

```json
{
  "developer": ["skill-code-review", "skill-git-ops"],
  "web_research": ["skill-domain-knowledge"]
}
```

**为什么存 ID 数组而不存完整 SkillBinding？** 子 agent 不支持 on_demand 模式，所有绑定的 skill 统一以 always 方式加载，无需存 mode 字段。存 ID 数组更简洁，运行时再构造 `[]SkillBinding{id, mode: "always"}`。

### D2: 数据模型 — Go 结构体扩展

`AgentConfig` 新增字段：

```go
type AgentConfig struct {
    // ...existing fields...
    SubagentSkills map[string][]string `json:"subagentSkills,omitempty"`
}
```

key 为 profile 名称（developer / file_analysis / web_research / data_report），value 为 skill ID 列表。

### D3: API 扩展 — 沿用现有 partial update 模式

`PUT /api/agents/{id}` body 新增 `subagentSkills` 字段。`UpdateAgentConfig` 的 `updates` map 新增 `subagentSkills` key，序列化为 JSON 写入。`GET` 响应同步返回。

### D4: Runner skill 解析 — 新增 `resolveSubagentSkills` 函数

在 `processor.go` 中新增 `resolveSubagentSkills(agentConfig, agentID, profile)` 函数：
1. 查找 `agentConfig.SubagentSkills[profile]`
2. 如果有且非空，构造 `[]SkillBinding`（每个 ID → `{id, mode: "always"}`），调用 `GetSkillsContext(agentID, bindings)` 编译出 `SkillContext`
3. 如果为空或不存在，返回 nil（子 agent 不注入任何 skill）

### D5: 子 agent 启动流程变更

`StartRequest` 新增 `SkillContext *storage.SkillContext` 字段。

在 `manager.run` 中：
- 如果 `req.SkillContext != nil`，将 `SkillContext.SkillsSnippet` 追加到子 agent 的 system prompt
- 将 `SkillContext.Tools` 对应的 `RegisteredTool` 列表追加到子 agent 的工具列表中（不受 `filterToolsByProfile` 过滤）
- 注册 skill 工具的执行器（使用 `ToolRegistry.RegisterSkills` 已有逻辑复用）

实现方式：在 `manager.run` 中，创建子 agent 专属的 `ToolRegistry`，先注册 profile 过滤后的内置工具，再注册 skill 工具。

### D6: Admin UI — 在子 agent 配置区域添加 skill 绑定

在 `agent-detail.tsx` 的子 Agent 模型配置 section 下方，为每个 profile 添加 skill 多选。使用已有的 `useSkills()` hook 获取可选 skill 列表，以 checkbox 或 multi-select 形式让管理员为每个 profile 选择 skill。

## Risks / Trade-offs

- **[risk] Skill 工具与内置工具命名冲突** — 如果 skill 定义的工具名与 profile 白名单中的内置工具重名，会产生覆盖。→ 缓解：skill 工具已有 `[Skill: xxx]` 前缀约定，冲突概率低；如发生，后注册的 skill 工具覆盖内置工具。
- **[risk] 子 agent 无法使用 load_skill / load_skill_reference** — on_demand skill 依赖这两个工具的交互式调用，子 agent 的工具白名单中没有它们。→ 设计决策：所有子 agent skill 以 always 模式加载，不支持 on_demand。
- **[trade-off] 不支持 on_demand 模式** — 简化了实现但限制了灵活性。如果未来需要，可在子 agent 工具白名单中加入 `load_skill` 并将 mode 存入配置。
- **[trade-off] 存 ID 数组而非 SkillBinding 数组** — 简洁但无法为子 agent skill 配置不同 mode。如果未来需要 on_demand 支持，需迁移数据格式。
