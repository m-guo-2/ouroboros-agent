## ADDED Requirements

### Requirement: Session 动态 Skill 记录表

系统 SHALL 维护 `session_active_skills` 表，记录动态加载到特定 session 的 skill：
- `id`（TEXT，主键）
- `session_id`（TEXT，非空）
- `skill_id`（TEXT，非空）
- `source`（TEXT，默认 `'hook'`）：加载来源标记
- `created_at`（INTEGER）
- `UNIQUE(session_id, skill_id)`：同一 session 下同一 skill 不重复

#### Scenario: 记录创建

- **WHEN** hook 的 `activate_skill` action 为 session `sess-1` 加载 skill `icebreaker`
- **THEN** `session_active_skills` 表新增一条记录，session_id 为 `sess-1`，skill_id 为 `icebreaker`，source 为 `hook`

#### Scenario: 幂等写入

- **WHEN** hook 重复为同一 session 加载同一 skill
- **THEN** 不产生重复记录，不报错（INSERT OR IGNORE 语义）

### Requirement: Effective Skills 合并

processSession 构建 effective skills 列表时，SHALL 将 `AgentConfig.Skills`（经 persona 覆盖后）与 `session_active_skills` 中当前 session 的 skill ID 合并，去重后传入 `GetSkillsContext`。

#### Scenario: 合并常驻和动态 skill

- **WHEN** `AgentConfig.Skills` 为 `["faq", "coding"]`，`session_active_skills` 中当前 session 有 `["icebreaker"]`
- **THEN** effective skills 为 `["faq", "coding", "icebreaker"]`（顺序：常驻在前，动态在后）

#### Scenario: 去重

- **WHEN** `AgentConfig.Skills` 包含 `"faq"`，`session_active_skills` 也包含 `"faq"`
- **THEN** effective skills 中 `"faq"` 只出现一次

#### Scenario: 无动态 skill

- **WHEN** `session_active_skills` 中当前 session 无记录
- **THEN** effective skills 等于 `AgentConfig.Skills`，行为与当前逻辑完全一致

### Requirement: complete_skill Tool

系统 SHALL 提供 `complete_skill` tool，注册到 skill 内部工具集，允许 LLM 主动卸载 skill。

Tool 参数：
- `skill_id`（string，必填）：要卸载的 skill ID

执行逻辑：
1. 从 `session_active_skills` 表删除对应记录
2. 如果记录不存在（skill 不是动态加载的，或已被卸载），返回提示信息，不报错

#### Scenario: 成功卸载动态 skill

- **WHEN** LLM 调用 `complete_skill(skill_id: "icebreaker")`，当前 session 的 `session_active_skills` 中存在 `icebreaker` 记录
- **THEN** 记录被删除，下次 processSession 构建 effective skills 时 `icebreaker` 不再包含在内

#### Scenario: 卸载后 skill 不再参与 prompt

- **WHEN** `icebreaker` 被 `complete_skill` 卸载
- **THEN** 后续消息处理中，`icebreaker` 不出现在 `SkillsSnippet` 中，`load_skill` 也无法加载它（除非它同时存在于 `AgentConfig.Skills`）

#### Scenario: 卸载不存在的 skill

- **WHEN** LLM 调用 `complete_skill(skill_id: "nonexistent")`
- **THEN** 返回提示"该 skill 未在当前 session 中动态加载"，不报错

#### Scenario: 不能卸载常驻 skill

- **WHEN** LLM 调用 `complete_skill(skill_id: "faq")`，`faq` 是 `AgentConfig.Skills` 中的常驻 skill，不在 `session_active_skills` 中
- **THEN** 返回提示"该 skill 是常驻 skill，无法通过 complete_skill 卸载"

### Requirement: complete_skill Tool 注册

`complete_skill` SHALL 和 `load_skill`、`load_skill_reference`、`run_script` 一样注册到 skill 内部工具集，对所有 subagent profile 可用。

#### Scenario: Tool 在主 agent 中可用

- **WHEN** agent 有动态加载的 skill
- **THEN** LLM 的可用工具列表中包含 `complete_skill`

#### Scenario: Tool 在 subagent 中可用

- **WHEN** subagent 执行任务
- **THEN** subagent 的工具列表中包含 `complete_skill`（通过 skillTools 白名单）
