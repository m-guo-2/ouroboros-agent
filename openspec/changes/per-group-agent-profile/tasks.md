## 1. 存储层 — Persona

- [x] 1.1 在 `storage/db.go` 的 `runSchema` 中添加 `agent_personas` 建表 DDL 和索引
- [x] 1.2 在 `storage/types.go` 中定义 `Persona` 结构体（指针字段区分 NULL）
- [x] 1.3 新建 `storage/personas.go`，实现 CRUD：`ListPersonas`、`GetPersona`、`CreatePersona`、`UpdatePersona`、`ClonePersona`、`DeletePersona`
- [x] 1.4 `ListPersonas` 和 `GetPersona` 需返回关联群数量（`groupCount`，通过 JOIN 或子查询）
- [x] 1.5 `DeletePersona` 需检查无群分配引用，有引用时返回错误

## 2. 存储层 — 群分配

- [x] 2.1 在 `storage/db.go` 中添加 `group_persona_assignments` 建表 DDL 和唯一索引
- [x] 2.2 在 `storage/types.go` 中定义 `GroupAssignment` 和 `UnconfiguredGroup` 结构体
- [x] 2.3 新建 `storage/group_assignments.go`，实现：`GetGroupAssignment(agentID, sessionKey)`、`ListGroupAssignments(agentID)`、`CreateGroupAssignment`、`UpdateGroupAssignment`、`DeleteGroupAssignment`
- [x] 2.4 实现 `ListUnconfiguredGroups(agentID)`：从 agent_sessions 中提取未分配的群聊

## 3. 运行时覆盖

- [x] 3.1 在 `runner/processor.go` 中新增 `applyPersonaOverride(agent *AgentConfig, persona *Persona)` 函数
- [x] 3.2 在 `processSession` 中注入两步查询（assignment → persona）并调用覆盖，失败静默降级

## 4. Admin API — Persona

- [x] 4.1 新建 `api/personas.go`，注册 `/api/agents/{agentId}/personas` 路由
- [x] 4.2 实现 GET（list + single）、POST（create）、PUT（update）、DELETE
- [x] 4.3 实现 `POST /api/agents/{agentId}/personas/{id}/clone`

## 5. Admin API — 群分配

- [x] 5.1 新建 `api/group_assignments.go`，注册 `/api/agents/{agentId}/groups` 路由
- [x] 5.2 实现 GET（list）、`GET discover`（未分配群发现）
- [x] 5.3 实现 POST（创建分配，含"忽略"即 persona_id 为空）、PUT（切换 Persona）、DELETE

## 6. Admin UI — Persona 管理

- [x] 6.1 在 `admin/src/api/types.ts` 中新增 `Persona`、`GroupAssignment`、`UnconfiguredGroup` 类型
- [x] 6.2 新增 Persona 和群分配的 API 调用函数
- [x] 6.3 在 agent 详情页新增 "Personas" 和 "群分配" 标签页入口
- [x] 6.4 实现 Persona 列表卡片组件（名称、模型、skills 概览、关联群数）
- [x] 6.5 实现 Persona 创建/编辑表单（toggle 开关 + 各字段编辑器 + 默认值参考）
- [x] 6.6 实现 Persona 复制功能（弹窗输入新名称）

## 7. Admin UI — 群分配管理

- [x] 7.1 实现未分配群队列组件（discover 数据 + Persona 下拉选择 + 确认/忽略）
- [x] 7.2 实现已分配群列表（显示 group_name + session_key + Persona 下拉可切换）
- [x] 7.3 实现手动添加群功能（输入 session_key + 群名称 + 选择 Persona）
- [x] 7.4 群分配标签页角标显示未分配群数量

## 8. 验证

- [ ] 8.1 创建 Persona "客服助手"（覆盖 system_prompt + skills），确认保存和回显正确
- [ ] 8.2 复制 Persona，确认新 Persona 字段与原始一致
- [ ] 8.3 将群分配到 Persona，在群聊中发消息确认使用 Persona 配置
- [ ] 8.4 切换群的 Persona，确认下次消息使用新 Persona
- [ ] 8.5 测试未分配群使用 agent 默认配置
- [ ] 8.6 新群出现后确认 discover API 和 UI 队列正确展示
