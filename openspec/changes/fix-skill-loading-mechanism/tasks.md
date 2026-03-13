## 1. 数据结构

- [x] 1.1 在 `agent/internal/storage/types.go` 中新增 `SkillBinding` 结构体（`ID string` + `Mode string`），将 `AgentConfig.Skills` 类型从 `[]string` 替换为 `[]SkillBinding`
- [x] 1.2 更新 `agent/internal/storage/agents.go` 的 `scanAgentConfig`，直接按 `[]SkillBinding` 反序列化 skills JSON
- [x] 1.3 更新 `CreateAgentConfig` 和 `UpdateAgentConfig`，按 `[]SkillBinding` 序列化 skills 字段

## 2. BuildSystemPrompt 模板替换

- [x] 2.1 修改 `agent/internal/runner/processor.go` 的 `BuildSystemPrompt`：如果 `agentSystemPrompt` 包含 `{{skills}}`，用 `strings.Replace` 替换为 `skillsSnippet`；否则不注入 skills 内容
- [x] 2.2 验证 `moli-system-prompt.md` 中的 `{{skills}}` 占位符在最终 prompt 中被正确替换，不再有字面 `{{skills}}` 残留

## 3. GetSkillsContext 支持绑定模式

- [x] 3.1 修改 `agent/internal/storage/skills.go` 的 `GetSkillsContext` 签名，将 `agentSkills []string` 改为 `agentSkills []SkillBinding`
- [x] 3.2 实现 always 模式：将 skill 的 name、description、完整 readme 内联到 `SkillsSnippet`，同时注册 tools
- [x] 3.3 实现 on_demand 模式：仅将 name、description、skill_id 索引放入 `SkillsSnippet`，不注册 tools
- [x] 3.4 保持未绑定的 enabled skill 出现在"可按需加载的扩展技能"区域
- [x] 3.5 更新 `GetSkillsContext` 的所有调用方（`processor.go`、`api/agents.go`、`api/skills.go`）传递 `[]SkillBinding`

## 4. Admin API 适配

- [x] 4.1 更新 `agent/internal/api/agents.go` 相关端点，确保 skills 字段使用 `[]SkillBinding` 格式
- [x] 4.2 确保 `GET /api/agents/:id/full-prompt` 端点正确反映模板替换后的完整 prompt

## 5. Admin UI 适配

- [x] 5.1 在 `admin/src/api/types.ts` 中新增 `SkillBinding` 接口（`{id: string, mode: "always" | "on_demand"}`），将 `AgentProfile.skills` 类型从 `string[]` 改为 `SkillBinding[]`
- [x] 5.2 在 `admin/src/api/agents.ts` 的 `create` 方法中，将 `skills` 参数类型从 `string[]` 改为 `SkillBinding[]`
- [x] 5.3 在 `admin/src/components/features/agents/agent-detail.tsx` 中：将 `selectedSkills` 状态从 `string[]` 改为 `SkillBinding[]`；修改 `toggleSkill` 逻辑（绑定时默认 `mode: "on_demand"`）；新增 `setSkillMode(skillId, mode)` 函数
- [x] 5.4 在技能绑定 Tab 的每个已绑定 skill 行中，Switch 下方增加 mode 选择器（Segmented Control），选项为"必召回" / "按需加载"，未绑定时隐藏
- [x] 5.5 确保打开 agent 详情页时，从 `agent.skills`（`SkillBinding[]`）正确初始化 `selectedSkills` 和每个 skill 的 mode 回显
- [x] 5.6 保存后自动刷新"完整 Prompt 预览"（确认在 skill 绑定变更后也能触发 `fetchFullPrompt()`）

## 6. 设计文档同步

- [x] 6.1 更新 `docs/decisions/044-skill-design-philosophy.md`：在原则二中补充 always 模式的说明
- [x] 6.2 更新 `docs/decisions/047-systemprompt-admin-transparent.md`：确认 `BuildSystemPrompt` 行为描述与实现一致（模板替换而非拼接追加）
- [x] 6.3 检查 `docs/decisions/043-wecom-skill-progressive-loading.md` 和 `docs/decisions/062-agent-skill-binding-wysiwyg.md`，确保与新机制不矛盾

## 7. 数据清理

- [x] 7.1 更新 DB 初始化脚本（`agent/data/*.sql`）中的 agent skills 字段为新格式
