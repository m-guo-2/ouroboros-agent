## 1. 存储层

- [x] 1.1 在 `storage/types.go` 新增 `SubagentModelConfig` 结构体和 `AgentConfig.SubagentModels` 字段
- [x] 1.2 在 `storage/db.go` 的 `runSchema` 中添加 migration：`ALTER TABLE agent_configs ADD COLUMN subagent_models TEXT DEFAULT '{}'`
- [x] 1.3 在 `storage/agents.go` 的 `scanAgentConfig` 中读取 `subagent_models` 列并反序列化到 `SubagentModels`
- [x] 1.4 在 `storage/agents.go` 的 `CreateAgentConfig` 中序列化写入 `subagent_models`
- [x] 1.5 在 `storage/agents.go` 的 `UpdateAgentConfig` 中支持 `subagentModels` key，序列化为 JSON 写入

## 2. API 层

- [x] 2.1 在 `api/agents.go` 的 `handleAgents`（POST）中解析 body 的 `subagentModels` 字段并传入 `AgentConfig`
- [x] 2.2 验证 `GET /api/agents/{id}` 和 `GET /api/agents` 响应正确返回 `subagentModels` 字段

## 3. Runner 层

- [x] 3.1 在 `runner/processor.go` 中提取 `buildLLMClient(provider string, creds *storage.ProviderCredentials) engine.LLMClient` 工厂函数
- [x] 3.2 重构 `processOneMessage` 中主 agent LLM client 构建逻辑为调用 `buildLLMClient`
- [x] 3.3 新增 `resolveSubagentLLM(agentConfig *storage.AgentConfig, profile string, mainClient engine.LLMClient, mainModel string) (engine.LLMClient, string)` 函数
- [x] 3.4 修改 `run_subagent_async` handler，调用 `resolveSubagentLLM` 获取子 agent 的 client 和 model，替代直接传递主 agent 的值

## 4. Admin UI

- [x] 4.1 在 `admin/src/api/types.ts` 中新增 `SubagentModelConfig` 类型定义
- [x] 4.2 在 `agent-detail.tsx` 的「配置」tab 中添加「子 Agent 模型配置」section，包含 4 个 profile 行（provider 下拉 + model 输入）
- [x] 4.3 在保存逻辑中将子 agent 模型配置序列化为 `subagentModels` 字段，空值 profile 过滤掉
- [x] 4.4 从 API 加载 agent 数据时正确回显 `subagentModels` 到各 profile 行

## 5. 验证

- [x] 5.1 通过 admin UI 为某个 agent 配置子 agent 使用不同 provider+model，保存后刷新页面确认回显正确
- [x] 5.2 触发子 agent 任务，确认日志中子 agent 使用了配置的模型而非主 agent 模型
- [x] 5.3 测试未配置 subagentModels 的 agent，确认子 agent 回退到主 agent 模型
