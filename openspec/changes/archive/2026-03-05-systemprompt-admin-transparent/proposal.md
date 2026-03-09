## Why

Moli 的 SystemPrompt 由三段在代码里暗中拼接：数据库段（Admin 可编辑）+ `processor.go` 硬编码的 builtin 段 + `skills.go` 动态生成的 Skills 段。管理者在 Admin UI 编辑 system prompt 时，不知道最终发给模型的内容还包含了两段看不见的追加。这导致调试困难、预期与实际不一致。

目标：**Admin 后台写什么就是什么**。消除所有隐藏拼接，system prompt 完全配置化。

## What Changes

- **BREAKING**：`buildSystemPrompt` 不再追加 builtin 段和 skills 段，直接返回数据库中的 `system_prompt`
- builtin 消息格式协议内容合并进数据库中的 agent system prompt
- Skills 附加段（技能索引 + README + 可加载技能列表）合并进数据库中的 agent system prompt
- `SkillContext.SystemPromptAddition` 字段废弃，skills.go 不再生成 prompt 追加内容
- Admin UI 编辑的 system prompt 支持 `{{skills}}` 模板变量，运行时自动展开为当前绑定技能的索引内容，保持技能动态绑定能力

## Capabilities

### New Capabilities
- `prompt-full-control`: SystemPrompt 完全由 Admin 后台管理，消除代码层硬编码拼接，支持 `{{skills}}` 模板变量自动展开

### Modified Capabilities

## Impact

- `agent/internal/runner/processor.go`：`buildSystemPrompt` 简化为模板展开
- `agent/internal/storage/skills.go`：`GetSkillsContext` 不再生成 `SystemPromptAddition`，改为生成可被模板引用的数据
- `agent/internal/storage/types.go`：`SkillContext.SystemPromptAddition` 字段移除或改用途
- DB `agent_configs`：所有 agent 的 `system_prompt` 需迁移，把 builtin 段和 skills 段内容补入
- Admin UI：无需改动（编辑区已存在），但需知道 `{{skills}}` 变量可用
