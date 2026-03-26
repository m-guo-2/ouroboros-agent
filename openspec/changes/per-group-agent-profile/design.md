## Context

当前 agent 配置粒度为 per-agent：`agent_configs` 表存储 `system_prompt`、`provider`/`model`、`skills`、`subagent_models`、`subagent_skills`，同一 agent 下所有群聊和私聊共享同一套配置。请求处理流程为：

```
dispatcher → GetAgentConfig(agentID)
           → GetSkillsContext(agentConfig.Skills)
           → BuildSystemPrompt(agentConfig.SystemPrompt, skillsSnippet)
           → buildLLMClient(agentConfig.Provider, creds)
           → RunAgentLoop
```

群由 `ChannelConversationID` 标识，在 dispatcher 中通过 `resolveSessionKey` 生成 `session_key`（格式：`channel:conversationID`）。session_key 已用于 session 隔离，但不参与配置选择。

子 agent 启动时通过 `resolveSubagentLLM` 和 `resolveSubagentSkillsSnippet` 读取 `agentConfig.SubagentModels` 和 `agentConfig.SubagentSkills` 做 profile 级覆盖。

管理员的真实心智模型不是"给某个 session_key 覆盖字段"，而是"我要一个客服助手人设用在客服群，一个技术专家人设用在技术群"。因此引入 **Persona（人设）** 作为核心抽象：一个 Persona 定义一次，分配给 N 个群，改 Persona 即改所有关联群。

产品约束：**群只有在收到第一条消息后才会出现在系统中**（session 在那时创建）。策略：新群先用 agent 默认配置，出现后管理员在 Admin UI 中看到并快速分配 Persona。

## Goals / Non-Goals

**Goals:**
- 引入 Persona 概念：命名的行为配置包（system_prompt + model + skills + subagent_models + subagent_skills）
- 支持将群分配到 Persona，一个 Persona 可被多个群共享
- 未分配 Persona 的群使用 agent 默认配置
- Persona 中未设置的字段回退到 agent 默认值（字段级粒度）
- Admin UI 自动发现新群（从 session 中提取），管理员一键下拉分配 Persona
- Persona 支持复制，快速创建相似人设

**Non-Goals:**
- 通过渠道 API 主动拉取群列表（群发现依赖 session 自动出现）
- Merge 语义（Persona 中已设置的字段为 replace，不做追加/删减）
- Per-session 或 per-user 级别的 Persona
- Persona 版本历史或审计日志
- 跨 agent 共享 Persona 模板

## Decisions

### D1: 数据模型 — 两张表分离定义和分配

**`agent_personas` 表（Persona 定义）**

```sql
CREATE TABLE IF NOT EXISTS agent_personas (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    display_name TEXT NOT NULL,
    system_prompt TEXT,
    provider TEXT,
    model TEXT,
    skills TEXT,
    subagent_models TEXT,
    subagent_skills TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_agent_personas_agent ON agent_personas(agent_id);
```

**`group_persona_assignments` 表（群→Persona 分配）**

```sql
CREATE TABLE IF NOT EXISTS group_persona_assignments (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    session_key TEXT NOT NULL,
    group_name TEXT NOT NULL DEFAULT '',
    persona_id TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_group_assignments_agent_key
    ON group_persona_assignments(agent_id, session_key);
```

两张表分离的好处：
- **Persona 复用**：10 个客服群共享 1 个"客服助手"Persona，改一次全生效
- **关注点分离**：Persona 管"是什么行为"，assignment 管"用在哪个群"
- **灵活性**：`persona_id = NULL` 表示管理员确认该群使用默认配置（从"未分配"队列消失）

字段说明：
- Persona 的覆盖字段均可为 NULL（不覆盖，回退到 agent 默认值）
- `skills`：JSON 数组字符串，NULL = 不覆盖
- `subagent_models`：JSON 对象字符串，NULL = 不覆盖
- `subagent_skills`：JSON 对象字符串，NULL = 不覆盖
- `provider` + `model`：必须同时非 NULL 才生效

### D2: Go 结构体

```go
type Persona struct {
    ID             string                         `json:"id"`
    AgentID        string                         `json:"agentId"`
    DisplayName    string                         `json:"displayName"`
    SystemPrompt   *string                        `json:"systemPrompt"`
    Provider       *string                        `json:"provider"`
    Model          *string                        `json:"model"`
    Skills         *[]string                      `json:"skills"`
    SubagentModels map[string]SubagentModelConfig  `json:"subagentModels,omitempty"`
    SubagentSkills map[string][]string            `json:"subagentSkills,omitempty"`
    CreatedAt      string                         `json:"createdAt"`
    UpdatedAt      string                         `json:"updatedAt"`
}

type GroupAssignment struct {
    ID         string  `json:"id"`
    AgentID    string  `json:"agentId"`
    SessionKey string  `json:"sessionKey"`
    GroupName  string  `json:"groupName"`
    PersonaID  *string `json:"personaId"`
    CreatedAt  string  `json:"createdAt"`
    UpdatedAt  string  `json:"updatedAt"`
}
```

`PersonaID` 使用指针：nil/NULL = 未分配（不应出现在 assignment 表中），`""` = 使用默认配置（管理员点了"忽略"）。

### D3: 运行时覆盖 — processSession 中两步查询

在 `processSession` 中，`GetAgentConfig(agentID)` 之后、`GetSkillsContext` 之前注入：

```go
agentConfig, _ := storage.GetAgentConfig(agentID)

// 两步查询：session_key → persona_id → persona config
assignment, err := storage.GetGroupAssignment(agentID, sessionData.SessionKey)
if err != nil {
    logger.Warn(ctx, "群分配查询失败，使用默认配置", "error", err)
}
if assignment != nil && assignment.PersonaID != nil && *assignment.PersonaID != "" {
    persona, err := storage.GetPersona(*assignment.PersonaID)
    if err != nil {
        logger.Warn(ctx, "Persona 查询失败，使用默认配置", "error", err)
    }
    if persona != nil {
        applyPersonaOverride(agentConfig, persona)
    }
}

skillsCtx, _ := storage.GetSkillsContext(agentConfig.Skills)
```

`applyPersonaOverride` 逐字段替换：
- `SystemPrompt`：非 nil 则替换
- `Provider` + `Model`：同时非 nil 才替换
- `Skills`：非 nil 则替换
- `SubagentModels`：非 nil 则替换
- `SubagentSkills`：非 nil 则替换

覆盖发生在 agentConfig 内存副本上，后续流程（BuildSystemPrompt、resolveSubagentLLM 等）自动生效。任一步查询失败均静默降级到默认配置。

**性能考虑**：两次查询（assignment + persona）均为主键/唯一索引查询，SQLite WAL 模式下各 <1ms，合计远小于 LLM 调用延迟。

### D4: 未分配群发现

新增 `ListUnconfiguredGroups(agentID)`：

```sql
SELECT DISTINCT s.session_key, s.channel_name, s.source_channel, MAX(s.updated_at) as last_active
FROM agent_sessions s
WHERE s.agent_id = ?
  AND s.channel_conversation_id != ''
  AND s.session_key NOT IN (
      SELECT ga.session_key FROM group_persona_assignments ga WHERE ga.agent_id = ?
  )
GROUP BY s.session_key
ORDER BY last_active DESC
```

过滤私聊（`channel_conversation_id != ''`），返回已出现但未分配的群。

### D5: Admin API

**Persona 管理**

| Method | Path | 功能 |
|--------|------|------|
| GET | `/api/agents/{agentId}/personas` | 列出所有 Persona |
| GET | `/api/agents/{agentId}/personas/{id}` | 获取单个 Persona |
| POST | `/api/agents/{agentId}/personas` | 创建 Persona |
| POST | `/api/agents/{agentId}/personas/{id}/clone` | 复制 Persona |
| PUT | `/api/agents/{agentId}/personas/{id}` | 更新 Persona |
| DELETE | `/api/agents/{agentId}/personas/{id}` | 删除 Persona（需检查无群引用）|

**群分配管理**

| Method | Path | 功能 |
|--------|------|------|
| GET | `/api/agents/{agentId}/groups` | 列出所有群分配 |
| GET | `/api/agents/{agentId}/groups/discover` | 发现未分配的群 |
| POST | `/api/agents/{agentId}/groups` | 创建群分配（含手动添加）|
| PUT | `/api/agents/{agentId}/groups/{id}` | 更新群分配（切换 Persona）|
| DELETE | `/api/agents/{agentId}/groups/{id}` | 删除群分配 |

### D6: Admin UI

Agent 详情页新增两个标签页：**Personas** 和 **群分配**。

**Personas 标签页**

```
┌─────────────────────────────────────────────┐
│  Personas                    [+ 新建 Persona] │
│                                              │
│  ┌────────────────────────────────────────┐  │
│  │ 🟢 客服助手                             │  │
│  │ 模型: gpt-4o-mini  Skills: 产品知识(+1) │  │
│  │ 已分配: 3 个群                          │  │
│  │                      [编辑] [复制]      │  │
│  └────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────┐  │
│  │ 🔵 技术专家                             │  │
│  │ 模型: claude-4  Skills: 代码审查, git   │  │
│  │ 已分配: 1 个群                          │  │
│  │                      [编辑] [复制]      │  │
│  └────────────────────────────────────────┘  │
└─────────────────────────────────────────────┘
```

Persona 卡片展示：名称、模型、skills 概览、关联群数量。
编辑表单：每个覆盖字段用 toggle 控制（关闭 = 不覆盖，用 agent 默认值；开启 = 编辑覆盖值）。toggle 关闭时灰色显示默认值作参考。

**群分配标签页**

```
┌─────────────────────────────────────────────────┐
│  ⚡ 新发现的群（2）                               │
│  ┌───────────────────────────────────────────┐  │
│  │  qiwei:room-new1    3分钟前               │  │
│  │  Persona: [选择 Persona ▾] [确认] [忽略]  │  │
│  ├───────────────────────────────────────────┤  │
│  │  feishu:chat-abc    10分钟前              │  │
│  │  Persona: [选择 Persona ▾] [确认] [忽略]  │  │
│  └───────────────────────────────────────────┘  │
│                                                 │
│  ─── 已分配的群 ───                    [+ 添加群] │
│  群名称       渠道标识             Persona        │
│  客服一群     qiwei:room-cs1     [客服助手 ▾]     │
│  客服二群     qiwei:room-cs2     [客服助手 ▾]     │
│  技术群       qiwei:room-dev     [技术专家 ▾]     │
│  运营群       qiwei:room-ops     [默认配置 ▾]     │
└─────────────────────────────────────────────────┘
```

核心交互：下拉选 Persona，一步完成分配。"忽略"创建一条 persona_id 为空字符串的记录（确认用默认），该群从"新发现"消失。"+ 添加群"支持手动输入 session_key 用于预配置。

## Risks / Trade-offs

- **[两步查询]** → assignment + persona 各一次主键查询，合计 <2ms，可忽略。如果将来需要极致优化可合并为一次 JOIN 查询。

- **[Persona 删除保护]** → 删除已被群引用的 Persona 需先解除所有群分配，API 层校验。

- **[Replace 语义]** → Persona 中已设置的字段完全替换 agent 默认值。管理员需在 Persona 编辑表单中看到完整配置。toggle 关闭 = 回退默认。

- **[新群首批消息用默认配置]** → 接受延迟。新群先用默认配置回复，出现在"未分配"队列后管理员快速分配。对于需要预配置的群，支持手动添加。

- **[与 group-prompt-override 的关系]** → 本变更完全 supersede 该变更，数据模型更完整，建议优先实施本变更。
