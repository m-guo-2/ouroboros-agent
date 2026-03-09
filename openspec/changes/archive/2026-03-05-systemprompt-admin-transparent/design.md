## Context

当前 `buildSystemPrompt(agentSystemPrompt, skillsAddition)` 做了三件事：
1. 取数据库段 `agentSystemPrompt`
2. 追加硬编码的 builtin（消息格式协议，~200 tokens）
3. 追加 `skillsAddition`（技能索引 + README + 可加载列表）

管理者在 Admin 写的只是第一段，后两段是代码暗中加的。用户要求：写什么就是什么，完全透明。

## Goals / Non-Goals

**Goals:**
- Admin 后台的 system prompt 字段 = 最终发给模型的完整内容（经模板展开后）
- 消除 `processor.go` 中的硬编码 builtin
- 消除 `skills.go` 的 `SystemPromptAddition` 暗中追加
- 保留技能动态绑定能力——通过 `{{skills}}` 模板变量实现

**Non-Goals:**
- 不改 Skills 的工具注册逻辑（tools 仍由 SkillContext 提供）
- 不改 Admin UI 的编辑器形态（仍是 Textarea）

## Decisions

### 1. buildSystemPrompt 简化为模板展开

改造前：

```go
func buildSystemPrompt(agentSystemPrompt, skillsAddition string) string {
    parts = [agentSystemPrompt, builtin, skillsAddition]
    return join(parts)
}
```

改造后：

```go
func BuildSystemPrompt(agentSystemPrompt, skillsSnippet string) string {
    result := agentSystemPrompt
    if strings.Contains(result, "{{skills}}") {
        result = strings.ReplaceAll(result, "{{skills}}", skillsSnippet)
    }
    return result
}
```

- 没有 builtin 追加——消息格式协议已合并进数据库 prompt
- `{{skills}}` 是唯一的模板变量，展开为技能索引内容
- 如果 prompt 中没写 `{{skills}}`，技能索引就不出现（显式控制）

**为什么用模板变量而不是完全静态**：Skills 绑定会变（增删 skill、启停 skill），如果把技能索引硬写进 prompt，每次改绑定都要手动更新 prompt 文本。`{{skills}}` 让管理者决定技能索引放在 prompt 的哪个位置，同时保持内容自动生成。

**替代方案考虑**：
- 完全静态（没有模板变量）——技能内容硬写进 prompt，改绑定就要改 prompt，维护成本高
- 多个模板变量（`{{builtin}}`、`{{skills}}`、`{{deferred_skills}}`）——过度设计，一个 `{{skills}}` 够用

### 2. skills.go 改为生成 snippet 而非 SystemPromptAddition

`GetSkillsContext` 当前生成 `SystemPromptAddition` 字符串并存入 `SkillContext`。改为：

- 字段重命名：`SystemPromptAddition` → `SkillsSnippet`
- 内容不变：仍然生成技能索引 + action README + 可加载列表
- 语义变了：不再是"暗中追加到 prompt"，而是"一段可被模板引用的文本"

调用链：`processor.go` 取到 `skillsCtx.SkillsSnippet`，传给 `BuildSystemPrompt` 做模板展开。

### 3. 数据迁移：把 builtin 合进现有 agent prompt

提供 SQL 迁移脚本，对现有 agent（目前只有 `default-agent-config`）的 `system_prompt` 做更新：

1. 把原 builtin 消息格式协议内容追加进去
2. 在末尾加上 `{{skills}}`
3. 如果是人设版 prompt，在适当位置插入消息格式协议和 `{{skills}}`

迁移后的 prompt 结构示例：

```
[角色定位 + 核心原则 + 行为准则]

## 消息格式协议
[原 builtin 内容]

{{skills}}
```

### 4. Admin UI 提示

在 system prompt 编辑区的 label 或 placeholder 中说明：
- 此内容即最终发给模型的完整 SystemPrompt
- 支持 `{{skills}}` 变量，运行时自动展开为当前绑定技能的索引

## Risks / Trade-offs

- **[迁移风险]** → 现有 prompt 需要手动或脚本合入 builtin 内容。如果有多个 agent，每个都要处理。当前只有一个 `default-agent-config`，风险可控。
- **[忘记写 `{{skills}}`]** → 如果管理者的 prompt 中没有 `{{skills}}`，模型不会看到技能索引。这是有意为之——完全由管理者控制。工具定义仍然在 tools 参数中，模型仍然能用工具，只是没有人类可读的使用指南。
- **[builtin 升级]** → 以前改 builtin 只需改代码，现在需要更新数据库里每个 agent 的 prompt。但这正是"透明化"的代价——改动必须显式。
