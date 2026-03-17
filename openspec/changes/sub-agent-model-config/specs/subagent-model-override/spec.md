## ADDED Requirements

### Requirement: Subagent model configuration storage
系统 SHALL 在 `agent_configs` 表中存储 per-profile 的子 agent 模型配置（`subagent_models` 列，JSON 格式）。配置结构为 `{profile: {provider, model}}` 映射，支持的 profile 为：`developer`、`file_analysis`、`web_research`、`data_report`。

#### Scenario: Agent with subagent model overrides
- **WHEN** 管理员通过 API 保存了 subagent model 配置 `{"web_research": {"provider": "deepseek", "model": "deepseek-chat"}}`
- **THEN** `agent_configs` 表中该 agent 的 `subagent_models` 列 SHALL 持久化此配置
- **THEN** 通过 `GET /api/agents/{id}` 返回的 `subagentModels` 字段 SHALL 包含相同数据

#### Scenario: Agent without subagent model config
- **WHEN** agent 的 `subagent_models` 列为空或 `{}`
- **THEN** `GET /api/agents/{id}` 返回的 `subagentModels` 字段 SHALL 为空对象或省略

#### Scenario: Database migration for existing agents
- **WHEN** 系统升级后首次启动
- **THEN** `agent_configs` 表 SHALL 自动添加 `subagent_models` 列（默认值 `'{}'`）
- **THEN** 已有 agent 的行为 SHALL 不受影响

### Requirement: API supports subagent model CRUD
`PUT /api/agents/{id}` SHALL 接受 `subagentModels` 字段并持久化。`GET /api/agents/{id}` 和 `GET /api/agents` SHALL 在响应中返回 `subagentModels` 字段。

#### Scenario: Update subagent models via API
- **WHEN** 调用 `PUT /api/agents/{id}` body 包含 `{"subagentModels": {"developer": {"provider": "openai", "model": "gpt-4o-mini"}}}`
- **THEN** 系统 SHALL 保存配置
- **THEN** 后续 `GET /api/agents/{id}` 响应 SHALL 包含 `subagentModels.developer` 为 `{"provider": "openai", "model": "gpt-4o-mini"}`

#### Scenario: Clear subagent model for a profile
- **WHEN** 调用 `PUT /api/agents/{id}` body 包含 `{"subagentModels": {}}`
- **THEN** 所有 profile 的子 agent 模型配置 SHALL 被清除
- **THEN** 运行时所有子 agent SHALL 使用主 agent 模型

#### Scenario: Partial update preserves other fields
- **WHEN** 调用 `PUT /api/agents/{id}` 仅包含 `{"subagentModels": {...}}`
- **THEN** agent 的其他字段（displayName、systemPrompt、provider、model 等）SHALL 不受影响

### Requirement: Runtime subagent model resolution
Runner 启动子 agent 时 SHALL 按以下优先级解析模型：
1. 该 profile 在 `subagentModels` 中有配置且 provider + model 均非空 → 使用配置的模型
2. 否则 → 回退到主 agent 的 provider + model

#### Scenario: Subagent uses overridden model
- **WHEN** agent 配置了 `subagentModels.web_research = {provider: "deepseek", model: "deepseek-chat"}`
- **WHEN** 主 agent 触发 `run_subagent_async` 指定 profile 为 `web_research`
- **THEN** 该子 agent SHALL 使用 deepseek provider 和 deepseek-chat 模型运行

#### Scenario: Subagent falls back to main agent model
- **WHEN** agent 未配置 `subagentModels.developer`（或该字段为空）
- **WHEN** 主 agent 触发 `run_subagent_async` 指定 profile 为 `developer`
- **THEN** 该子 agent SHALL 使用主 agent 的 provider 和 model 运行

#### Scenario: Overridden provider credentials unavailable
- **WHEN** subagent 配置了 provider 为 `moonshot` 但系统未配置 moonshot 的 API key
- **WHEN** 主 agent 触发对应 profile 的子 agent
- **THEN** 系统 SHALL 回退到主 agent 的模型并记录警告日志

### Requirement: Admin UI for subagent model configuration
Admin 面板的 agent 详情页 SHALL 提供子 agent 模型配置 UI，允许管理员为每个 profile 独立设置 provider 和 model。

#### Scenario: View subagent model settings
- **WHEN** 管理员打开 agent 详情的「配置」tab
- **THEN** 页面 SHALL 显示子 agent 模型配置区域
- **THEN** 每个 profile（developer、file_analysis、web_research、data_report）SHALL 各有一行配置项
- **THEN** 未配置的 profile SHALL 显示 placeholder 文案提示将继承主 agent 模型

#### Scenario: Set model for a specific profile
- **WHEN** 管理员在 web_research 行选择 provider 为 deepseek，输入 model 为 deepseek-chat
- **WHEN** 点击保存
- **THEN** 系统 SHALL 调用 `PUT /api/agents/{id}` 包含 `subagentModels` 字段
- **THEN** 保存成功后该行 SHALL 显示配置的 provider 和 model

#### Scenario: Clear model override for a profile
- **WHEN** 管理员将某个 profile 的 provider 和 model 都清空
- **WHEN** 点击保存
- **THEN** 该 profile SHALL 不再包含在 `subagentModels` 中
- **THEN** UI SHALL 恢复显示 placeholder 提示文案

### Requirement: LLM client factory extraction
系统 SHALL 将 LLM client 构建逻辑从 `processOneMessage` 中提取为独立函数，供主 agent 和子 agent 共用，避免代码重复。

#### Scenario: Main agent uses factory function
- **WHEN** Runner 处理主 agent 消息
- **THEN** SHALL 通过提取的工厂函数构建 LLM client
- **THEN** 行为 SHALL 与重构前完全一致

#### Scenario: Subagent uses factory function with different provider
- **WHEN** Runner 为子 agent 构建 LLM client
- **WHEN** 子 agent 的 provider 与主 agent 不同（如主 agent 用 anthropic，子 agent 用 openai）
- **THEN** 工厂函数 SHALL 根据子 agent 的 provider 构建正确类型的 client
