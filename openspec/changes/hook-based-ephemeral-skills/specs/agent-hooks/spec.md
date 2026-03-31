## ADDED Requirements

### Requirement: Hook 数据结构

AgentConfig SHALL 包含 `Hooks` 字段，类型为 `[]Hook`。每个 Hook 包含：
- `event`（string，必填）：触发事件名称，开放字符串，不限定枚举
- `actions`（[]HookAction，必填，至少一个）：事件触发后执行的动作列表

每个 HookAction 包含：
- `type`（string，必填）：动作类型
- `skillId`（string，按 type 决定是否必填）：目标 skill ID

#### Scenario: Hook 结构序列化

- **WHEN** agent 配置包含 hooks 字段，值为 `[{"event":"session_started","actions":[{"type":"activate_skill","skillId":"icebreaker"}]}]`
- **THEN** 解析后 `AgentConfig.Hooks` 包含 1 个 Hook，event 为 `session_started`，actions 包含 1 个 type 为 `activate_skill`、skillId 为 `icebreaker` 的 HookAction

#### Scenario: hooks 字段为空或缺失

- **WHEN** agent 配置中 hooks 字段为 `[]` 或不存在
- **THEN** `AgentConfig.Hooks` 为空切片，不影响 agent 正常运行

#### Scenario: 任意事件名称均可配置

- **WHEN** agent 配置中 hooks 包含 `event: "my_custom_event"`
- **THEN** 解析成功，`AgentConfig.Hooks` 包含该 hook，不因事件名未在枚举中而拒绝

### Requirement: Hook 配置存储

`agent_configs` 表 SHALL 包含 `hooks` 列（TEXT 类型，默认值 `'[]'`），存储 JSON 格式的 hook 规则数组。

#### Scenario: 新建 agent 不带 hooks

- **WHEN** 创建 agent 时未指定 hooks
- **THEN** 数据库中 hooks 列值为 `[]`，`AgentConfig.Hooks` 为空切片

#### Scenario: 更新 agent 的 hooks 配置

- **WHEN** 通过 `PUT /api/agents/{id}` 提交 `{"hooks": [{"event":"session_started","actions":[{"type":"activate_skill","skillId":"icebreaker"}]}]}`
- **THEN** 数据库中 hooks 列更新为对应 JSON，后续 `GetAgentConfig` 返回的 `Hooks` 包含该规则

### Requirement: Hook Action 类型

第一期 SHALL 支持以下 action type：
- `activate_skill`：将指定 skill 加入当前 session 的 effective skills 列表

遇到未识别的 action type 时，SHALL 记录警告日志并跳过该 action，不影响同一 hook 内其他 action 执行。

#### Scenario: activate_skill 执行成功

- **WHEN** hook 触发，action 为 `{"type":"activate_skill","skillId":"icebreaker"}`，skill `icebreaker` 存在于本地 store
- **THEN** skill `icebreaker` 被加入当前 session 的 effective skills 列表，参与后续 prompt 构建和 tool 注册

#### Scenario: activate_skill 重复激活幂等

- **WHEN** hook 触发 `activate_skill`，但该 skill 已在当前 session 的 effective skills 中
- **THEN** 不产生重复记录，保持幂等

#### Scenario: activate_skill 目标 skill 不存在

- **WHEN** hook 触发 `activate_skill`，但 skillId 对应的 skill 不在本地 store 中
- **THEN** 记录警告日志（diagnostic），不阻塞其他 action 和 hook 执行

#### Scenario: 未识别的 action type

- **WHEN** hook 触发，action type 为 `"unknown_action"`
- **THEN** 记录警告日志，跳过该 action，同一 hook 内其他 action 正常执行

### Requirement: DispatchHooks 分发函数

系统 SHALL 提供 `DispatchHooks(hooks []Hook, event string, sessionID string)` 函数：
1. 遍历 hooks，筛选 `hook.Event == event` 的条目
2. 对匹配的 hook，按 actions 顺序执行每个 action
3. 无匹配时静默返回，不报错

该函数不绑定特定调用位置，由调用方决定在何处、以何事件名触发。

#### Scenario: 匹配事件并执行 actions

- **WHEN** hooks 中包含 `event: "foo"` 的 hook，调用 `DispatchHooks(hooks, "foo", sessionID)`
- **THEN** 该 hook 的 actions 被依次执行

#### Scenario: 无匹配事件

- **WHEN** hooks 中无 `event: "bar"` 的 hook，调用 `DispatchHooks(hooks, "bar", sessionID)`
- **THEN** 无任何 action 执行，不报错

#### Scenario: 同一事件多个 hook

- **WHEN** hooks 中有两条 `event: "foo"` 的 hook
- **THEN** 两条 hook 的 actions 均被执行，按 hooks 数组顺序

### Requirement: Hooks 不受 Persona 覆盖

Persona 的 `applyPersonaOverride` SHALL 不修改 `AgentConfig.Hooks`。Hooks 只在 agent 层定义。

#### Scenario: Persona 覆盖不影响 hooks

- **WHEN** agent 配置了 hooks，且 session 关联的 persona 覆盖了 skills 和 systemPrompt
- **THEN** hooks 保持 agent 原始配置不变

### Requirement: Hook 配置 API

`GET /api/agents/{id}` 返回的 AgentConfig SHALL 包含 `hooks` 字段。
`PUT /api/agents/{id}` SHALL 支持通过 `hooks` 字段更新 hook 配置。
`POST /api/agents` SHALL 支持在创建时指定 `hooks` 字段。

#### Scenario: API 读取 hooks

- **WHEN** 调用 `GET /api/agents/{id}`，该 agent 配置了 hooks
- **THEN** 响应 JSON 中包含 `hooks` 数组，内容与配置一致

#### Scenario: API 更新 hooks

- **WHEN** 调用 `PUT /api/agents/{id}` 提交新的 hooks 配置
- **THEN** hooks 配置被更新，后续 `GET` 返回新配置
