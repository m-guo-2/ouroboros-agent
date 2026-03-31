## 1. 数据层：类型定义与存储

- [x] 1.1 在 `storage/types.go` 中新增 `Hook` 和 `HookAction` 类型定义（event string, actions []HookAction; type string, skillId string）
- [x] 1.2 在 `AgentConfig` 中新增 `Hooks []Hook` 字段
- [x] 1.3 DB migration：`agent_configs` 表新增 `hooks TEXT DEFAULT '[]'` 列
- [x] 1.4 `scanAgentConfig` 中解析 hooks JSON 列到 `AgentConfig.Hooks`
- [x] 1.5 `CreateAgentConfig` 和 `UpdateAgentConfig` 中支持 hooks 字段的序列化和写入
- [x] 1.6 DB migration：创建 `session_active_skills` 表（id, session_id, skill_id, source, created_at, UNIQUE(session_id, skill_id)）
- [x] 1.7 新增存储函数：`ActivateSessionSkill(sessionID, skillID, source string) error`（INSERT OR IGNORE）
- [x] 1.8 新增存储函数：`GetActiveSessionSkills(sessionID string) ([]string, error)`——返回 skill ID 列表
- [x] 1.9 新增存储函数：`DeactivateSessionSkill(sessionID, skillID string) (bool, error)`——删除记录，返回是否实际删除

## 2. API 层：Hook 配置管理

- [x] 2.1 `api/agents.go` 的 POST handler 中解析 `hooks` 字段并传入 `CreateAgentConfig`
- [x] 2.2 `api/agents.go` 的 PUT handler 中支持 `hooks` 字段的更新（传入 `UpdateAgentConfig`）
- [x] 2.3 确认 GET handler 返回的 `AgentConfig` 已自动包含 `hooks`（JSON 序列化）

## 3. Hook 分发函数

- [x] 3.1 新增 `DispatchHooks(hooks []Hook, event string, sessionID string) error` 函数：遍历 hooks 匹配 event，按 actions 顺序执行
- [x] 3.2 实现 `executeHookAction(action HookAction, sessionID string) error` 函数，处理 `activate_skill` 类型：调用 `ActivateSessionSkill`
- [x] 3.3 未识别的 action type 记录警告日志并跳过
- [x] 3.4 分发和执行时记录 business 日志：事件名、匹配 hook 数量、执行的 action、skill_id、结果

## 4. Effective Skills 合并

- [x] 4.1 在 `processSession` 构建 `skillsCtx` 前，调用 `GetActiveSessionSkills(sessionID)` 获取动态 skill ID 列表
- [x] 4.2 将动态 skill IDs 与 `agentConfig.Skills`（经 persona 覆盖后）合并去重
- [x] 4.3 用合并后的 skill ID 列表调用 `GetSkillsContext`（替换当前直接使用 `agentConfig.Skills` 的逻辑）

## 5. complete_skill Tool

- [x] 5.1 在 `engine/registry.go` 的 `skillToolSchemas` 中新增 `complete_skill` 的 schema 定义（参数：skill_id string 必填）
- [x] 5.2 在 `engine/registry.go` 的 `skillToolDescriptions` 中新增 `complete_skill` 的描述
- [x] 5.3 将 `complete_skill` 加入 `subagent/manager.go` 的 `skillTools` 白名单
- [x] 5.4 在 `runner/processor.go` 中实现 `complete_skill` tool handler：校验 skill_id、区分常驻 vs 动态、调用 `DeactivateSessionSkill`、返回结果信息
- [x] 5.5 在 `RegisterSkillInternalTools` 中注册 `complete_skill` handler

## 6. 验证

- [x] 6.1 端到端验证：通过 API 配置 agent hooks → 调用 `DispatchHooks` 触发 activate_skill → skill 出现在 effective skills → LLM 调用 complete_skill → skill 被卸载 → 后续不再包含该 skill
