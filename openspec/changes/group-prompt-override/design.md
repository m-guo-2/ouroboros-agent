## Context

当前 agent 配置粒度为 per-agent：`agent_configs` 表存储 `system_prompt` 和 `skills`（JSON 数组），同一 agent 下所有群聊和私聊共享同一套配置。请求处理流程为：

```
dispatcher → GetAgentConfig(agentID) → GetSkillsContext(agentID, skills) → BuildSystemPrompt → RunAgentLoop
```

群由 `ChannelConversationID` 标识（企微是 `FromRoomID`，飞书是 `chat_id`），在 dispatcher 中通过 `resolveSessionKey` 生成 `session_key`（格式：`channel:conversationID`）。session_key 已用于 session 隔离，但不参与配置选择。

Admin API 已有完整的 agent CRUD 端点（`/api/agents`），使用标准 JSON 响应格式（`{success, data}`）。

## Goals / Non-Goals

**Goals:**
- 支持按群（session_key）覆盖 agent 的 system_prompt 和 skills
- 未配置覆盖的群使用 agent 默认配置（两级回退）
- 覆盖语义为 replace：群配置存在时完全替换对应字段，不做 merge
- 提供 Admin API 管理群级配置
- 修改量最小化：只在 processor 加载配置处插入一次覆盖查询

**Non-Goals:**
- Admin 前端 UI（后续跟进）
- Merge 语义（在默认 skills 基础上追加/删减）
- 跨 agent 共享群配置模板
- 群配置的版本历史或审计日志

## Decisions

### 1. 存储：新增 `group_configs` 表

```sql
CREATE TABLE IF NOT EXISTS group_configs (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    session_key TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    system_prompt TEXT,
    skills TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_group_configs_agent_key ON group_configs(agent_id, session_key);
```

字段说明：
- `session_key`：群的唯一标识，格式 `channel:conversationID`（如 `qiwei:wr123abc`）。使用 session_key 而非裸 `channel_conversation_id`，因为 session_key 天然区分渠道且覆盖私聊场景（`channel:userID`）。
- `display_name`：群的显示名称，用于 admin 管理界面展示（如"客服群"、"技术讨论群"），不影响运行时逻辑。
- `system_prompt`：覆盖 `agent_configs.system_prompt`。NULL 表示不覆盖，使用 agent 默认值。
- `skills`：覆盖 `agent_configs.skills`。NULL 表示不覆盖，使用 agent 默认值。JSON 数组格式，与 `agent_configs.skills` 一致。

独立建表而非在 `agent_sessions` 上加字段，原因：
- session 是运行时状态，群配置是管理态配置，生命周期不同
- 一个群可能有多个 session（不同 agent 或 session 重建），配置应该跨 session 生效
- 关注点分离：session 管运行状态，group_config 管行为定制

**考虑过的替代方案**：在 `agent_configs` 上加一个 `overrides` JSON 字段存所有群覆盖。但这会导致单条记录膨胀严重（可能几十个群的配置），且并发写冲突风险高。独立表 + 唯一索引是更干净的方案。

### 2. 运行时覆盖逻辑

在 `processOneEvent` 中，`GetAgentConfig` 之后、`GetSkillsContext` 之前，插入一次覆盖查询：

```
agentConfig = GetAgentConfig(agentID)
groupConfig = GetGroupConfig(agentID, sessionKey)  // 新增
if groupConfig != nil {
    if groupConfig.SystemPrompt != nil { agentConfig.SystemPrompt = *groupConfig.SystemPrompt }
    if groupConfig.Skills != nil       { agentConfig.Skills = *groupConfig.Skills }
}
skillsCtx = GetSkillsContext(agentID, agentConfig.Skills)
systemPrompt = BuildSystemPrompt(agentConfig.SystemPrompt, skillsCtx.SkillsSnippet)
```

关键设计点：
- 覆盖发生在 agentConfig 上，后续流程（BuildSystemPrompt、GetSkillsContext）无需感知群配置的存在
- 使用指针语义区分"未设置"（nil）和"设置为空"（空字符串/空数组），NULL 表示不覆盖
- 查询失败时静默降级到 agent 默认配置，不阻塞消息处理

**考虑过的替代方案**：在 dispatcher 层做覆盖。但 dispatcher 的职责是路由和去重，不应承担配置组装的逻辑。processor 已经是加载 agentConfig 的地方，在同一处做覆盖最内聚。

### 3. Admin API

新增端点，挂载在 `/api/agents/{agentId}/groups/` 下：

| Method | Path | 功能 |
|--------|------|------|
| GET | `/api/agents/{agentId}/groups` | 列出 agent 的所有群配置 |
| GET | `/api/agents/{agentId}/groups/{id}` | 获取单条群配置 |
| POST | `/api/agents/{agentId}/groups` | 创建群配置 |
| PUT | `/api/agents/{agentId}/groups/{id}` | 更新群配置 |
| DELETE | `/api/agents/{agentId}/groups/{id}` | 删除群配置 |

路由注册在现有 `api.Mount()` 中追加。请求/响应格式复用现有的 `ok()`、`created()`、`apiErr()` helper。

API 使用 `id`（群配置的主键）作为路径参数，而非 `session_key`，避免 URL 编码问题（session_key 包含 `:`）。

### 4. storage 层函数

新增文件 `agent/internal/storage/group_configs.go`：

```go
type GroupConfig struct {
    ID           string    `json:"id"`
    AgentID      string    `json:"agentId"`
    SessionKey   string    `json:"sessionKey"`
    DisplayName  string    `json:"displayName"`
    SystemPrompt *string   `json:"systemPrompt"`   // nil = 不覆盖
    Skills       *[]string `json:"skills"`          // nil = 不覆盖
    CreatedAt    string    `json:"createdAt"`
    UpdatedAt    string    `json:"updatedAt"`
}

func GetGroupConfig(agentID, sessionKey string) (*GroupConfig, error)
func ListGroupConfigs(agentID string) ([]GroupConfig, error)
func GetGroupConfigByID(id string) (*GroupConfig, error)
func CreateGroupConfig(cfg GroupConfig) (*GroupConfig, error)
func UpdateGroupConfig(id string, updates map[string]interface{}) (*GroupConfig, error)
func DeleteGroupConfig(id string) (bool, error)
```

`SystemPrompt` 和 `Skills` 使用指针类型，SQL 层 NULL 映射为 Go nil，精确区分"不覆盖"与"覆盖为空"。

## Risks / Trade-offs

- **[每次请求多一次 DB 查询]** → `GetGroupConfig` 是按唯一索引的精确查询，SQLite WAL 模式下读性能极高（<1ms）。相比 LLM 调用的数秒延迟可忽略。如果将来群配置量极大，可加内存缓存。

- **[Replace 语义可能导致基础能力丢失]** → 管理员设置群 skills 时需要完整列出所有需要的 skill ID，包括想保留的默认 skills。这是有意的设计选择：显式优于隐式，避免 merge 语义的组合爆炸和理解成本。Admin UI 可以在创建时预填 agent 默认 skills 来降低操作负担。

- **[session_key 格式依赖]** → 群配置以 `session_key`（`channel:conversationID`）为 key，如果 `resolveSessionKey` 的格式变化，群配置会失效。但 session_key 是已经稳定使用的概念，且有唯一索引保护。
