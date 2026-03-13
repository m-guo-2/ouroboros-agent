## Why

Skill 加载机制存在三个问题：

1. **占位符泄漏**：`BuildSystemPrompt` 没有做 `{{skills}}` 模板替换，而是直接追加 `skillsSnippet`。如果 DB prompt 中包含 `{{skills}}`，占位符会原样出现在最终发给模型的 prompt 中，与追加的 skills 内容重复。这违反了 047 透明化决策的设计意图。
2. **缺少"必召回"机制**：当前 agent 绑定的 skill 统一走 System Loaded 路径（tools 注册 + readme 作为使用指南附加），但没有"必召回"（always-inline）选项——将 skill 的 name、description 和完整 skill.md 内容直接内联到 SystemPrompt 中。对于定义 agent 身份和核心行为的 skill，这种内联方式比工具注册更合适。
3. **设计文档与实现不一致**：047 决策文档约定 `BuildSystemPrompt` 做模板替换，实际实现是拼接追加；044 设计哲学描述的两级加载模型中没有体现"必召回"概念。

## What Changes

- **修复 `BuildSystemPrompt`**：实现真正的 `{{skills}}` 模板替换，如果 prompt 不含 `{{skills}}` 则不追加 skills 内容（符合 047 决策：完全由管理者控制）
- **引入 skill 绑定类型**：agent 配置的每个 skill 绑定支持 `mode` 属性，区分 `always`（必召回）和 `on_demand`（按需召回）：
  - `always`：skill 的 name、description 和完整 readme 内容直接嵌入 `{{skills}}` 展开结果中；tools 照常注册
  - `on_demand`：仅在 `{{skills}}` 展开结果中放 name、description 和 skill_id（保障 `load_skill` 可访问）；tools 不预注册
- **更新 `AgentConfig.Skills` 数据结构**：从 `[]string`（skill ID 列表）直接替换为 `[]SkillBinding`（结构化绑定）
- **同步设计文档**：确保 044、047 等相关设计文档与新实现一致

## Capabilities

### New Capabilities

- `skill-binding-modes`: 引入 always / on_demand 两种 skill 绑定模式，控制 skill 内容在 SystemPrompt 中的呈现方式

### Modified Capabilities

（无已有 spec 需修改）

## Impact

**后端：**
- **`agent/internal/runner/processor.go`**：`BuildSystemPrompt` 改为真正的模板替换
- **`agent/internal/storage/skills.go`**：`GetSkillsContext` 根据绑定 mode 决定 skill 内容的生成策略
- **`agent/internal/storage/types.go`**：`AgentConfig.Skills` 从 `[]string` 直接替换为 `[]SkillBinding`
- **`agent/internal/storage/agents.go`**：skill 字段的序列化/反序列化适配新结构
- **DB**：`agent_configs.skills` JSON 格式直接使用 `[{"id":"...","mode":"..."}]`
- **API**：agent CRUD 接口的 skills 字段格式变更

**前端：**
- **`admin/src/api/types.ts`**：`AgentProfile.skills` 类型从 `string[]` 改为 `SkillBinding[]`
- **`admin/src/components/features/agents/agent-detail.tsx`**：技能绑定 Tab 从纯开关切换改为开关 + mode 选择器（必召回 / 按需加载）
- **`admin/src/api/agents.ts`**：`create` 和 `update` 的 skills 参数类型适配

**设计文档**：044、047 等决策文档需同步更新
