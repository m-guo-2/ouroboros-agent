## 1. 后端：简化 buildSystemPrompt

- [x] 1.1 将 `processor.go` 中 `buildSystemPrompt` 改为仅做 `{{skills}}` 模板展开，删除硬编码 builtin 段，删除 skillsAddition 追加逻辑
- [x] 1.2 导出为 `BuildSystemPrompt(agentSystemPrompt, skillsSnippet string) string`
- [x] 1.3 更新 processor.go 内部调用点，传入 `skillsCtx.SkillsSnippet`

## 2. 后端：SkillContext 字段重命名

- [x] 2.1 `storage/types.go`：`SkillContext.SystemPromptAddition` → `SkillsSnippet`
- [x] 2.2 `storage/skills.go`：所有 `ctx.SystemPromptAddition` 引用改为 `ctx.SkillsSnippet`
- [x] 2.3 全局搜索确认无其他文件引用 `SystemPromptAddition`

## 3. 后端：新增 full-prompt API

- [x] 3.1 在 `agents.go` 的 `handleAgentsWithID` 中增加 `GET /api/agents/:id/full-prompt` 处理：读取 agent 配置 → 调用 `GetSkillsContext` → 调用 `BuildSystemPrompt` → 返回 `{ fullPrompt }`

## 4. 数据迁移

- [x] 4.1 编写 SQL 迁移脚本 `agent/data/047-prompt-transparent.sql`：更新 `default-agent-config` 的 system_prompt，合入 builtin 消息格式协议内容和 `{{skills}}`
- [x] 4.2 同步更新 `043-wecom-skills.sql` 中的 system_prompt，保持初始化脚本一致

## 5. 前端：API 层

- [x] 5.1 在 `admin/src/api/agents.ts` 中新增 `getFullPrompt(id: string)` 方法

## 6. 前端：Agent 详情页预览区

- [x] 6.1 在 `agent-detail.tsx` 的系统提示词编辑区下方增加只读预览区，展示展开后的完整 prompt
- [x] 6.2 预览区：页面加载时自动请求、保存后自动刷新、提供"刷新预览"按钮
- [x] 6.3 编辑区 label 增加说明：支持 `{{skills}}` 变量
