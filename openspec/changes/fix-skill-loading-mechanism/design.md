## Context

当前 skill 加载流程：

1. `processor.go` 的 `BuildSystemPrompt(agentSystemPrompt, skillsSnippet)` 将 DB prompt 与 skills 片段**直接拼接**（`prompt + "\n\n" + skillsSnippet + memoryInstruction`）
2. DB prompt（如 `moli-system-prompt.md`）末尾包含 `{{skills}}` 占位符，但运行时未被替换，原样出现在最终 prompt 中
3. `GetSkillsContext` 根据 `agentConfig.Skills`（`[]string`）将 skill 分为 active（绑定的）和 deferred（未绑定的），但 active skill 统一走 "tools 注册 + readme 作为使用指南" 路径，没有 "直接内联到 prompt" 的选项

相关设计文档：
- 044：定义了 System Loaded / On-Demand 两级加载，但没有 "必召回内联" 概念
- 047：约定 `BuildSystemPrompt` 做 `{{skills}}` 模板替换，实际实现不一致
- 062：统一了 agent skill 绑定到 ID 维度，空数组 = 不绑定

**项目状态**：未正式上线，不需要向后兼容，直接使用最优方案。

## Goals / Non-Goals

**Goals:**

1. `BuildSystemPrompt` 实现真正的 `{{skills}}` 模板替换，消除占位符泄漏
2. 引入 always / on_demand 两种 skill 绑定模式
3. always 模式的 skill 完整内容（name + description + readme）内联到 `{{skills}}` 展开结果
4. on_demand 模式的 skill 仅放索引（name + description + skill_id），确保 `load_skill` 可访问
5. `AgentConfig.Skills` 直接使用 `[]SkillBinding` 结构，不做旧格式兼容
6. 同步更新相关设计文档

**Non-Goals:**

- 不改变 skill 的 GitHub 存储格式和同步机制
- 不改变 `load_skill` / `load_skill_reference` 工具的行为
- 不改变 Admin UI 的技能管理列表/详情页（仅在 agent 详情页的绑定区域增加 mode 切换）
- 不引入第三种 mode（如 "conditional"）
- 不处理 `memoryInstruction` 硬编码追加的问题（独立 change）

## Decisions

### 1. `BuildSystemPrompt` 改为模板替换

**选择**：如果 `agentSystemPrompt` 包含 `{{skills}}`，则替换为 `skillsSnippet`；否则不注入 skills 内容。

**理由**：符合 047 决策的原始意图——Admin 写什么就是什么。如果管理者选择不在 prompt 中放 `{{skills}}`，模型不应看到技能信息。

**替代方案**：
- 保持拼接但先去掉 `{{skills}}` 字面量 → 仍然违反"Admin 写什么就是什么"原则
- 永远追加 → 管理者无法控制 skills 出现的位置

**注意**：`memoryInstruction` 的硬编码追加问题暂不在此 change 处理，保留现有行为（追加在末尾）。

### 2. SkillBinding 数据结构

**选择**：

```go
type SkillBinding struct {
    ID   string `json:"id"`
    Mode string `json:"mode"` // "always" | "on_demand"
}
```

`AgentConfig.Skills` 从 `[]string` 直接替换为 `[]SkillBinding`。DB 中 `agent_configs.skills` JSON 格式统一为 `[{"id":"...","mode":"..."}]`，不保留旧 `["id"]` 格式。

**理由**：项目未上线，不存在旧数据需要兼容。直接用结构化格式，代码最干净，语义最清晰。同一个 skill 在不同 agent 上可以有不同的加载策略。

**替代方案**：
- 在 skill manifest 上加 `alwaysLoaded` 属性 → 不灵活，同一 skill 对不同 agent 可能需要不同策略
- 用两个独立字段 `alwaysSkills` + `onDemandSkills` → 冗余，增加 API 复杂度

### 3. always 模式下 skills snippet 内容

**选择**：always 模式的 skill 在 `{{skills}}` 展开中包含：
- 标题行：`### Skill: {name}`
- 描述行：`{description}`
- 完整 readme 内容（即 skill.md / README.md 的全文）
- 如果有多个 always skill，依次排列，不分前后

同时，always 模式的 skill 的 tools 照常注册到 tool 列表。

**理由**：必召回 skill 的核心价值是让模型在 SystemPrompt 中直接获得完整上下文，不需要通过 `load_skill` 工具间接获取。这对定义 agent 身份和核心工作流的 skill 至关重要。

### 4. on_demand 模式下 skills snippet 内容

**选择**：on_demand 模式的 skill 在 `{{skills}}` 展开中包含：
- 索引行：`- **{name}**（id: \`{skill_id}\`）: {description}`
- 不包含 readme 内容
- 不预注册 tools

**理由**：通过 `load_skill` 按需获取完整内容，减少 SystemPrompt 膨胀。on_demand 对应的是 044 设计哲学中的 On-Demand Loaded 概念。

### 5. Admin UI 技能绑定交互

**当前状态**：`agent-detail.tsx` 的"技能"Tab 中，每个 skill 显示为一行，右侧是一个 `Switch` 开关（开/关）。`selectedSkills` 是 `string[]`，保存时发送 `skills: selectedSkills`。

**选择**：改为两级交互——

1. 保留 Switch 开关控制"是否绑定"
2. 绑定后，在该行内显示 mode 选择器：`必召回` / `按需`（对应 `always` / `on_demand`），默认 `按需`
3. 未绑定的 skill 不显示 mode 选择器

**数据模型变更**：
- `selectedSkills` 从 `string[]` 改为 `SkillBinding[]`（`{id: string, mode: "always" | "on_demand"}`）
- `AgentProfile.skills` 类型从 `string[]` 改为 `SkillBinding[]`
- `toggleSkill(skillId)` 改为：开启时默认添加 `{id, mode: "on_demand"}`；关闭时移除
- 新增 `setSkillMode(skillId, mode)` 切换已绑定 skill 的 mode
- 保存时发送 `skills: selectedSkills`（新格式）

**UI 布局**：

```
┌─────────────────────────────────────────────────────┐
│  企微核心                                    [Switch] │
│  核心消息收发和联系人搜索                              │
│                          ┌─────────┬──────────┐     │
│                          │ ● 必召回 │  按需加载 │     │  ← 绑定后显示
│                          └─────────┴──────────┘     │
├─────────────────────────────────────────────────────┤
│  群管理                                      [Switch] │
│  建群、改名、加/踢成员等                               │
│                          ┌─────────┬──────────┐     │
│                          │  必召回  │ ● 按需加载│     │
│                          └─────────┴──────────┘     │
├─────────────────────────────────────────────────────┤
│  朋友圈                                      [    ] │  ← 未绑定，不显示 mode
│  浏览、发布、点赞朋友圈                                │
└─────────────────────────────────────────────────────┘
```

**替代方案**：
- 用下拉菜单选择 mode → 多一次点击，不直观
- 三态开关（关 / 按需 / 必召回）→ Switch 组件不支持三态，需自定义组件，复杂度过高

### 6. Admin UI 完整 Prompt 预览

**当前状态**：`agent-detail.tsx` 的"配置"Tab 下方已有"完整 Prompt 预览"区域，通过 `GET /api/agents/:id/full-prompt` 获取展开后的 prompt。

**选择**：保持现有预览机制不变。backend 修复 `BuildSystemPrompt` 后，预览自然反映正确的模板替换结果。

需要确保：保存 skill 绑定变更后，自动刷新预览（当前已有 `fetchFullPrompt()` 在 save 后调用）。

## Risks / Trade-offs

- **[always skill 过多导致 prompt 膨胀]** → 在 Admin UI 中给出 token 估算提示，让管理者有感知。文档中建议 always skill 控制在 1-3 个。
- **[模板替换后 prompt 不含 `{{skills}}` 的情况]** → 如果管理者移除了 `{{skills}}`，skill 信息不会出现在 prompt 中。这是 by design（047 决策），但需在 Admin UI 中给出提醒。
