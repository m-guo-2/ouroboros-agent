## Context

当前 agent 的 skill 绑定模型：

```
Agent (agent_configs.skills)  →  所有 session 共享
  └─ Persona (persona.skills override)  →  按群覆盖
```

Skill 列表在配置时确定，运行时不可变。`processSession` 每次处理时从 `AgentConfig.Skills`（经 persona 覆盖后）构建 `SkillContext`，没有动态增删的通道。

关键现状：
- `AgentConfig.Skills` 是 `[]string`，存储 skill ID 列表
- `SkillContext` 每次 `processSession` 重新计算，不持久在 session 上
- Skill 解析走 `GetSkillsContext(skillIDs)` → 构建 Level 1 snippet + LoadableSkillIDs
- Persona 对 skills 是**整体替换**语义（`*[]string`，非 nil 时替换）
- `SessionData` 没有 skill 相关字段

需要引入两个新概念：Hook（编排规则）和 `complete_skill`（卸载 tool），让 skill 列表在运行时可动态变化。

## Goals / Non-Goals

**Goals:**
- Agent 配置中可以声明 hook 规则（event → actions）
- Hook 可以将 skill 动态加入 agent 的 effective skills 列表
- LLM 可以通过 `complete_skill` tool 主动卸载不再需要的 skill
- Hook 加载的 skill 和常驻 skill 在运行时无差异，统一参与 prompt 构建和 tool 注册
- Hook 配置跟 agent config 一起管理（CRUD、API），不引入新的管理概念
- 提供通用的 `DispatchHooks` 函数，供任意调用点以任意事件名触发

**Non-Goals:**
- 不硬编码具体事件集合——事件名是开放字符串，具体事件的接入属于后续业务层工作
- 不做 `when` 条件过滤（group_tag 等），后续扩展
- 不做 TTL / 轮次兜底的自动过期机制
- Persona 不覆盖 hooks——hooks 只在 agent 层定义
- 不做通用事件总线——hook 分发是简单的字符串匹配，不是 pub/sub
- Hook 不执行副作用（不发消息、不调 tool）——第一期 action 只有 skill 加载/卸载
- 不做具体业务事件的接入（session_started、member_joined 等由后续业务迭代对接）

## Decisions

### 1. Hook 的控制模型：Hook 编排式

**决定**：Hook 是独立的编排点，由 hook 决定在事件发生时做什么。Skill 不声明自己的 lifecycle。

**备选**：Skill 自声明 hooks（在 SKILL.md 中声明 `activate_on: session_started`）。

**理由**：
- Skill 是 agent 拥有的能力，编排权应在 agent 侧
- Hook 配置集中管理，一个地方看到所有编排规则，可观测性好
- Skill 自声明会导致知识分散，冲突处理困难（两个 skill 都声明同一事件）

### 2. 加载的 skill 不做特殊区分

**决定**：Hook 触发 `activate_skill` 后，skill 直接加入 effective skills 列表，和 `AgentConfig.Skills` 中的常驻 skill 无差异。

**备选**：引入 `ephemeral_skill_states` 表，在 processSession 时区分常驻和临时 skill。

**理由**：
- 统一模型更简单——skill 就是 skill，不需要在 prompt 构建、tool 注册等环节做分支处理
- 需要一个轻量记录来追踪"这个 session 动态加载了哪些 skill"，但这个记录只服务于合并逻辑，不影响 skill 的运行时行为

### 3. 卸载由 LLM 主动驱动

**决定**：提供 `complete_skill` tool，LLM 根据 skill prompt 中的退出条件自行判断并调用卸载。

**备选**：
- 系统 TTL 自动过期
- 轮次计数自动过期
- Hook 规则驱动卸载（`deactivate_skill` action）

**理由**：
- "什么时候该走"需要理解对话上下文（如破冰是否完成），这是 LLM 擅长的判断
- 系统规则无法表达"用户已完成自我介绍"这类语义条件
- 卸载智能在 skill 的 prompt 内容里，不在系统代码里
- 如果后续观察到 LLM 遗忘调用的问题，再加 TTL 兜底

### 4. Hook 配置存储：agent_configs JSON 列

**决定**：`agent_configs` 表新增 `hooks TEXT DEFAULT '[]'` 列，和 skills、channels 同一模式。

**备选**：独立 `agent_hooks` 表，每个 hook 一行。

**理由**：
- 和现有配置模式完全一致（skills、channels、subagent_models 都是 JSON 列）
- Hook 数量不会很多（一个 agent 通常几条规则），不需要独立表的查询灵活性
- CRUD 完全复用 `UpdateAgentConfig` 的现有模式

### 5. 动态 skill 加载记录：session 维度的轻量表

**决定**：新增 `session_active_skills` 表，记录 hook 为某个 session 加载了哪些 skill。`processSession` 时查询此表，合并到 effective skills 列表。

**表结构**：

```sql
CREATE TABLE IF NOT EXISTS session_active_skills (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    skill_id   TEXT NOT NULL,
    source     TEXT NOT NULL DEFAULT 'hook',  -- 来源标记
    created_at INTEGER NOT NULL DEFAULT 0,
    UNIQUE(session_id, skill_id)
);
```

`complete_skill` 时删除对应记录。记录存在 = skill 生效，记录不存在 = skill 已卸载。

**理由**：
- 需要知道"这个 session 被 hook 加载了哪些 skill"，才能在 processSession 时合并
- 不用在 `SessionData` 上加字段——`SessionData` 是通用会话数据，不应承载 skill 逻辑
- `complete_skill` 的语义就是删除这条记录，简单直接

### 6. 事件名称是开放字符串

**决定**：Hook 的 `event` 字段是任意字符串，系统不硬编码事件集合。`DispatchHooks` 函数接受事件名参数，和 hook 配置中的 `event` 做精确匹配。

**备选**：定义一个固定的事件枚举（`session_started`、`turn_completed` 等），不在枚举内的事件拒绝配置。

**理由**：
- 第一期只做基础设施，具体事件由后续业务迭代决定在哪些代码路径上触发
- 开放字符串允许渠道事件（`member_joined`、`contact_added`）和处理流程事件（`session_started`、`turn_completed`）共用同一套机制
- 不需要每次新增事件都改 hook 基础设施代码

### 7. DispatchHooks 是独立函数，不绑定 processSession

**决定**：提供 `DispatchHooks(hooks []Hook, event string, sessionID string)` 函数，由调用方在合适的位置调用。不在 processSession 中硬编码触发点。

**备选**：在 processSession 的固定位置埋入 hook 分发调用。

**理由**：
- 事件可能来自 processSession 内部（消息处理流程），也可能来自 dispatcher 层（渠道事件如 group_event）
- 函数式设计让调用方自己决定"在哪里、以什么事件名触发"，hook 基础设施不做假设
- 后续接入具体事件时，只需在对应代码路径加一行 `DispatchHooks(...)` 调用

## Risks / Trade-offs

**[LLM 遗忘调用 complete_skill]** → 临时 skill 永久挂载，持续消耗 prompt token。
缓解：skill 的 SKILL.md 中需要明确写退出条件和调用指令。后续可加 TTL 兜底。

**[Persona 替换 skills 后与 hook 的交互]** → Persona 可能替换掉 `AgentConfig.Skills`，但 hook 中引用的 skill_id 仍会被加载（因为 hook 加载的 skill 走 `session_active_skills`，独立于 `AgentConfig.Skills`）。
接受：这是预期行为——hook 加载的 skill 不受 persona 影响。

**[skill 不在本地 store 中]** → Hook 尝试 activate 一个未同步到本地的 skill。
缓解：`GetSkillsContext` 已有对 missing skill 的 diagnostics 处理，hook 加载的 skill 走同一路径，自然降级。
