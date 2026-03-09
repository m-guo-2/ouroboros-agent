# SystemPrompt 透明化：Admin 写什么就是什么

- **日期**：2026-03-05
- **类型**：架构决策 / 代码变更
- **状态**：已实施

## 背景

Moli 的 SystemPrompt 由三段在代码里暗中拼接：数据库段（Admin 可编辑）+ `processor.go` 硬编码的 builtin 消息格式协议段 + `skills.go` 动态生成的 Skills 段。管理者在 Admin UI 编辑 system prompt 时，不知道最终发给模型的内容还包含了两段看不见的追加，导致调试困难、预期与实际不一致。

## 决策

Admin 后台的 system prompt 字段经 `{{skills}}` 模板展开后即为最终发给模型的完整内容，消除所有代码层暗中拼接。

## 变更内容

- **`agent/internal/runner/processor.go`**：`buildSystemPrompt` 简化为导出函数 `BuildSystemPrompt(agentSystemPrompt, skillsSnippet string) string`，仅做 `{{skills}}` 模板替换，删除硬编码 builtin 段和 skillsAddition 追加逻辑
- **`agent/internal/storage/types.go`**：`SkillContext.SystemPromptAddition` 重命名为 `SkillsSnippet`（语义从"暗中追加"变为"可被模板引用的文本片段"）
- **`agent/internal/storage/skills.go`**：所有 `ctx.SystemPromptAddition` 引用改为 `ctx.SkillsSnippet`
- **`agent/internal/api/agents.go`**：新增 `GET /api/agents/:id/full-prompt` 端点，返回展开后的完整 prompt 供前端预览
- **`agent/data/047-prompt-transparent.sql`**：迁移脚本，将原 builtin 消息格式协议内容和 `{{skills}}` 合入 `default-agent-config` 的 system_prompt
- **`agent/data/043-wecom-skills.sql`**：同步更新初始化脚本，保持一致
- **`admin/src/api/agents.ts`**：新增 `getFullPrompt(id)` API 方法
- **`admin/src/components/features/agents/agent-detail.tsx`**：系统提示词编辑区增加 `{{skills}}` 变量说明，下方新增只读预览区展示展开后的完整 prompt

## 考虑过的替代方案

- **完全静态（无模板变量）**：技能索引硬写进 prompt，每次改绑定都要手动更新 prompt，维护成本高——否决
- **多个模板变量（`{{builtin}}`、`{{skills}}`、`{{deferred_skills}}`）**：过度设计，一个 `{{skills}}` 够用——否决

## 影响

- 运行迁移脚本前，现有 agent 的 prompt 不含消息格式协议和 `{{skills}}`，需执行 `047-prompt-transparent.sql`
- 如果 prompt 中不写 `{{skills}}`，模型不会看到技能索引（有意为之——完全由管理者控制）
- 以前改 builtin 只需改代码，现在需要更新数据库中的 prompt，这是"透明化"的代价
