## Context

当前系统中，子 agent（subagent）通过 `subagent.StartRequest` 启动时，直接继承主 agent 的 `Model` 和 `LLMClient`（`processor.go:568-573`）。四种 subagent profile（developer、file_analysis、web_research、data_report）没有独立的模型配置入口。

`agent_configs` 表已有 `provider` + `model` 字段驱动主 agent 的模型选择。`UpdateAgentConfig` 使用 `map[string]interface{}` 做 partial update，扩展新字段较自然。Admin UI 在 `agent-detail.tsx` 中以 tabs 形式组织配置。

## Goals / Non-Goals

**Goals:**
- 管理员可以在 admin UI 中为每个 subagent profile 单独配置 provider + model
- 未配置的 profile 自动回退到主 agent 的模型（零配置向后兼容）
- Runner 启动子 agent 时根据 profile 查找对应模型配置，构建独立的 LLM client

**Non-Goals:**
- 不做 per-session 或 per-task 级别的模型覆盖
- 不支持 subagent profile 使用独立的 API key（复用主 agent provider 级别的 credentials）
- 不改动 `models` 表或统一 `model_id` 路径（那是另一个独立问题）
- 不修改 subagent profile 的 system prompt 或 tool 过滤逻辑

## Decisions

### D1: 存储方案 — `agent_configs` 表新增 `subagent_models` JSON 列

在 `agent_configs` 表新增一列 `subagent_models TEXT DEFAULT '{}'`，以 JSON 存储 profile → {provider, model} 的映射。

```json
{
  "developer": {"provider": "openai", "model": "gpt-4o"},
  "web_research": {"provider": "deepseek", "model": "deepseek-chat"}
}
```

**为什么不新建独立表？** 子 agent 模型配置和 agent 配置强关联、数据量很小（每个 agent 最多 4 条记录），JSON 列方案简单、无需额外 JOIN，与 `skills`/`channels` 列的存储模式一致。

### D2: 数据模型 — Go 结构体扩展

`AgentConfig` 新增字段：

```go
type SubagentModelConfig struct {
    Provider string `json:"provider"`
    Model    string `json:"model"`
}

type AgentConfig struct {
    // ...existing fields...
    SubagentModels map[string]SubagentModelConfig `json:"subagentModels,omitempty"`
}
```

### D3: API 扩展 — 沿用现有 partial update 模式

`PUT /api/agents/{id}` body 新增 `subagentModels` 字段。`UpdateAgentConfig` 的 `updates` map 新增 `subagentModels` key，序列化为 JSON 写入。`GET` 响应同步返回。

### D4: Runner 模型解析 — 辅助函数 `resolveSubagentLLM`

在 `processor.go` 中新增 `resolveSubagentLLM(agentConfig, profile)` 函数：
1. 查找 `agentConfig.SubagentModels[profile]`
2. 如果有且 provider + model 非空，用对应 credentials 构建新 LLM client
3. 否则返回主 agent 的 llmClient 和 modelName

子 agent 启动处（`run_subagent_async` handler）调用此函数替代直接传递主 agent 的 client。

### D5: LLM client 构建复用 — 提取工厂函数

当前 `processor.go` 中 LLM client 构建逻辑（根据 provider 判断用 Anthropic 还是 OpenAI compatible）内联在 `processOneMessage` 中。提取为独立函数 `buildLLMClient(provider string, credentials *ProviderCredentials) engine.LLMClient`，主 agent 和子 agent 共用。

### D6: Admin UI — 在「配置」tab 中添加子 agent 模型配置 section

在 `agent-detail.tsx` 的「配置」tab 中，主 agent 模型选择下方添加「子 Agent 模型配置」折叠区域。每个 profile 一行，包含 provider 下拉 + model 输入框。留空表示继承主 agent 配置，UI 以 placeholder 文案提示。

## Risks / Trade-offs

- **[risk] Provider credentials 不匹配** — 如果为 subagent 配置了某个 provider 但系统未设置该 provider 的 API key，运行时会失败。→ 缓解：前端保存时校验 provider 是否有 credentials；Runner 解析失败时回退到主 agent 模型并打日志。
- **[risk] JSON 列查询不便** — SQLite JSON 列无法直接索引查询。→ 可接受：只需按 agent ID 读取后在应用层解析，不存在按 subagent model 反查的场景。
- **[trade-off] 不支持独立 API key** — 简化了设计但限制了灵活性。如果未来需要，可在 `SubagentModelConfig` 中扩展 `apiKey`/`baseUrl` 字段。
