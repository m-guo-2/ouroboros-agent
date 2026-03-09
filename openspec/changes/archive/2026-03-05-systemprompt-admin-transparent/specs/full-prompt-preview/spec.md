## ADDED Requirements

### Requirement: Admin 后台 system prompt 即最终 prompt
系统 SHALL 将 `agent_configs.system_prompt` 经模板展开后作为完整的 SystemPrompt 发给模型，不追加任何代码层硬编码内容。

#### Scenario: 无模板变量的 prompt
- **WHEN** agent 的 system_prompt 为纯文本（不含 `{{skills}}`）
- **THEN** 模型收到的 SystemPrompt 与数据库中的 system_prompt 完全一致，无任何追加

#### Scenario: 含 `{{skills}}` 的 prompt
- **WHEN** agent 的 system_prompt 包含 `{{skills}}`
- **THEN** `{{skills}}` 被替换为当前绑定技能的索引内容（技能摘要 + action README + 可加载列表），其余部分不变

#### Scenario: Skills 绑定变更后 prompt 自动更新
- **WHEN** agent 绑定的 skills 发生变更（增/删/改）
- **THEN** 下次请求时 `{{skills}}` 展开的内容自动反映最新绑定，无需手动编辑 prompt

### Requirement: 消除 buildSystemPrompt 中的硬编码 builtin
`buildSystemPrompt` SHALL 不再包含硬编码的消息格式协议段，该内容 SHALL 迁移至数据库中各 agent 的 system_prompt。

#### Scenario: 代码中无 builtin 追加
- **WHEN** `buildSystemPrompt` 被调用
- **THEN** 函数仅执行 `{{skills}}` 模板展开，不追加任何硬编码文本

### Requirement: SkillContext 不再生成 SystemPromptAddition
`GetSkillsContext` SHALL 将生成的技能索引内容存入 `SkillsSnippet` 字段（语义：可被模板引用的文本片段），而非 `SystemPromptAddition`（语义：暗中追加的内容）。

#### Scenario: SkillContext 结构变更
- **WHEN** `GetSkillsContext` 返回 SkillContext
- **THEN** `SkillContext` 包含 `SkillsSnippet` 字段，不包含 `SystemPromptAddition` 字段

### Requirement: 完整 prompt 预览 API
系统 SHALL 提供 `GET /api/agents/:id/full-prompt` 端点，返回 `{{skills}}` 展开后的最终完整 SystemPrompt。

#### Scenario: 正常获取
- **WHEN** 请求 `GET /api/agents/:id/full-prompt`，且 agent 存在
- **THEN** 返回 `{ "success": true, "data": { "fullPrompt": "<展开后的完整文本>" } }`

#### Scenario: Agent 不存在
- **WHEN** 请求 `GET /api/agents/:id/full-prompt`，且 agent 不存在
- **THEN** 返回 404

### Requirement: Admin UI 预览展开后的完整 prompt
Agent 详情页 SHALL 在 system prompt 编辑区下方提供只读预览区，展示 `{{skills}}` 展开后的最终内容。

#### Scenario: 页面加载时自动预览
- **WHEN** 用户进入 Agent 详情页
- **THEN** 预览区自动请求 `/api/agents/:id/full-prompt` 并展示结果

#### Scenario: 保存后刷新预览
- **WHEN** 用户保存 system prompt 编辑
- **THEN** 预览区自动刷新，展示最新的展开结果

#### Scenario: 手动刷新预览
- **WHEN** 用户点击"刷新预览"按钮
- **THEN** 预览区重新请求并展示最新结果

### Requirement: 数据迁移
现有 agent 的 system_prompt SHALL 通过迁移脚本更新，将原 builtin 消息格式协议内容和 `{{skills}}` 变量合入。

#### Scenario: default-agent-config 迁移
- **WHEN** 运行迁移脚本
- **THEN** `default-agent-config` 的 system_prompt 包含原 builtin 消息格式协议内容，且包含 `{{skills}}`
