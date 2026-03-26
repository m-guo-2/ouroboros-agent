## 1. 存储层

- [ ] 1.1 在 `storage/types.go` 的 `AgentConfig` 中新增 `SubagentSkills map[string][]string` 字段
- [ ] 1.2 在 `storage/db.go` 的 `runSchema` 中添加 migration：`ALTER TABLE agent_configs ADD COLUMN subagent_skills TEXT DEFAULT '{}'`
- [ ] 1.3 在 `storage/agents.go` 的 `scanAgentConfig` 中读取 `subagent_skills` 列并反序列化到 `SubagentSkills`
- [ ] 1.4 在 `storage/agents.go` 的 `CreateAgentConfig` 中序列化写入 `subagent_skills`
- [ ] 1.5 在 `storage/agents.go` 的 `UpdateAgentConfig` 中支持 `subagentSkills` key，序列化为 JSON 写入

## 2. API 层

- [ ] 2.1 在 `api/agents.go` 的 `handleAgents`（POST/PUT）中解析 body 的 `subagentSkills` 字段并传入 `AgentConfig`
- [ ] 2.2 验证 `GET /api/agents/{id}` 和 `GET /api/agents` 响应正确返回 `subagentSkills` 字段

## 3. Runner 层

- [ ] 3.1 在 `runner/processor.go` 中新增 `resolveSubagentSkills(agentConfig *storage.AgentConfig, agentID string, profile string) *storage.SkillContext` 函数
- [ ] 3.2 修改 `run_subagent_async` handler，调用 `resolveSubagentSkills` 获取 SkillContext，传入 `StartRequest`
- [ ] 3.3 在 `subagent.StartRequest` 中新增 `SkillContext *storage.SkillContext` 字段

## 4. Subagent Manager

- [ ] 4.1 在 `manager.run` 中处理 `req.SkillContext`：将 `SkillsSnippet` 追加到子 agent 的 system prompt
- [ ] 4.2 在 `manager.run` 中将 SkillContext 的工具注册到子 agent 的工具列表（独立于 `filterToolsByProfile` 白名单）

## 5. Admin UI

- [ ] 5.1 在 `admin/src/api/types.ts` 中的 `AgentProfile` 新增 `subagentSkills?: Record<string, string[]>` 类型
- [ ] 5.2 在 `agent-detail.tsx` 的子 Agent 配置区域，为每个 profile 添加 skill 多选控件
- [ ] 5.3 在保存逻辑中将子 agent skill 配置序列化为 `subagentSkills` 字段，空数组 profile 过滤掉
- [ ] 5.4 从 API 加载 agent 数据时正确回显 `subagentSkills` 到各 profile 的 skill 选择控件

## 6. 验证

- [ ] 6.1 通过 admin UI 为某个 agent 的 developer profile 绑定 skill，保存后刷新页面确认回显正确
- [ ] 6.2 触发 developer 子 agent 任务，确认子 agent 的 system prompt 包含绑定 skill 的文档
- [ ] 6.3 确认绑定 skill 的工具出现在子 agent 的工具列表中且可调用
- [ ] 6.4 测试未配置 subagentSkills 的 agent，确认子 agent 行为与之前一致
