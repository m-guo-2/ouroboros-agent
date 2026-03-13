## ADDED Requirements

### Requirement: BuildSystemPrompt 实现模板替换

`BuildSystemPrompt` SHALL 将 `agentSystemPrompt` 中的 `{{skills}}` 占位符替换为 `skillsSnippet` 内容。如果 `agentSystemPrompt` 不包含 `{{skills}}`，SHALL 不注入任何 skills 内容。最终 prompt 中 SHALL NOT 出现 `{{skills}}` 字面文本。

#### Scenario: prompt 包含 {{skills}} 占位符

- **WHEN** agent 的 system prompt 中包含 `{{skills}}`
- **THEN** `{{skills}}` 被替换为生成的 skills snippet 内容，最终 prompt 不含 `{{skills}}` 字面文本

#### Scenario: prompt 不包含 {{skills}} 占位符

- **WHEN** agent 的 system prompt 中不包含 `{{skills}}`
- **THEN** skills snippet 不被注入，最终 prompt 与 DB 中存储的内容一致（加上 memoryInstruction 追加）

### Requirement: SkillBinding 数据结构

Agent 配置的 skills 字段 SHALL 为 `[]SkillBinding`，每个绑定项包含 `id`（skill ID）和 `mode`（`always` 或 `on_demand`）。不支持旧 `[]string` 格式。

#### Scenario: 结构化 skill 绑定

- **WHEN** agent config 的 skills JSON 为 `[{"id":"skill-a","mode":"always"},{"id":"skill-b","mode":"on_demand"}]`
- **THEN** 系统正确解析出两个绑定，skill-a 为 always 模式，skill-b 为 on_demand 模式

#### Scenario: 空数组

- **WHEN** agent config 的 skills JSON 为 `[]`
- **THEN** 无任何 skill 绑定（空 = 不绑定）

### Requirement: always 模式 skill 内联

绑定 mode 为 `always` 的 skill SHALL 将完整内容内联到 `{{skills}}` 展开结果中。内联内容 SHALL 包含：skill 名称、描述、完整 readme。同时，该 skill 的 tools SHALL 注册到 agent 的可用工具列表中。

#### Scenario: 单个 always skill

- **WHEN** agent 绑定了一个 mode=always 的 skill（name="企微核心"，id="wecom-core"，readme 内容为 "..."）
- **THEN** `{{skills}}` 展开结果中包含该 skill 的名称、描述和完整 readme 内容
- **THEN** 该 skill 的 tools 被注册为 agent 可调用的工具

#### Scenario: 多个 always skill 无序排列

- **WHEN** agent 绑定了多个 mode=always 的 skill
- **THEN** 所有 always skill 的内容在 `{{skills}}` 展开结果中依次呈现，不要求特定顺序

### Requirement: on_demand 模式 skill 索引

绑定 mode 为 `on_demand` 的 skill SHALL 仅在 `{{skills}}` 展开结果中放置索引信息（name + description + skill_id）。SHALL NOT 包含 readme 内容。SHALL NOT 预注册该 skill 的 tools。该 skill 的完整内容 SHALL 可通过 `load_skill` 工具按需获取。

#### Scenario: on_demand skill 的索引展示

- **WHEN** agent 绑定了一个 mode=on_demand 的 skill（name="群管理"，id="wecom-group-mgmt"，description="建群、改名等"）
- **THEN** `{{skills}}` 展开结果中包含该 skill 的名称、id 和描述的简短索引行
- **THEN** 该 skill 的 tools 不出现在 agent 的工具列表中
- **THEN** 通过 `load_skill(skill_id="wecom-group-mgmt")` 可获取完整 readme 和 tools 参考

### Requirement: 未绑定的 enabled skill 可按需加载

未被 agent 绑定但处于 enabled 状态的 skill SHALL 仍可通过 `load_skill` 工具按需加载。其索引 SHALL 出现在 `{{skills}}` 展开结果的"扩展技能"区域中。

#### Scenario: 未绑定 skill 的可发现性

- **WHEN** 存在一个 enabled 的 skill 未被当前 agent 绑定
- **THEN** 该 skill 的索引出现在 `{{skills}}` 展开结果的"可按需加载的扩展技能"列表中
- **THEN** 通过 `load_skill` 可获取其完整内容

### Requirement: Admin API 支持结构化绑定格式

Agent 的 CRUD API SHALL 使用 `[{"id":"...","mode":"..."}]` 格式的 skills 字段。读写统一使用此格式。

#### Scenario: 创建 agent 时指定 skill 绑定模式

- **WHEN** 通过 API 创建 agent，skills 字段为 `[{"id":"wecom-core","mode":"always"}]`
- **THEN** agent 创建成功，skills 按新格式持久化

#### Scenario: 读取 agent 返回结构化格式

- **WHEN** 通过 API 读取 agent 配置
- **THEN** skills 字段返回 `[{"id":"...","mode":"..."}]` 格式

### Requirement: Admin UI 技能绑定支持 mode 选择

Agent 详情页的技能绑定 Tab SHALL 为每个已绑定的 skill 显示 mode 选择器，支持在"必召回"（always）和"按需加载"（on_demand）之间切换。未绑定的 skill SHALL NOT 显示 mode 选择器。新绑定的 skill 默认 mode SHALL 为 on_demand。

#### Scenario: 绑定 skill 时显示 mode 选择器

- **WHEN** 用户在技能绑定 Tab 中开启一个 skill 的绑定开关
- **THEN** 该 skill 行中出现 mode 选择器（必召回 / 按需加载），默认选中"按需加载"

#### Scenario: 切换已绑定 skill 的 mode

- **WHEN** 用户对已绑定的 skill 将 mode 从"按需加载"切换为"必召回"
- **THEN** UI 即时反映选择变更，保存后 skills 字段中该项的 mode 变为 `always`

#### Scenario: 取消绑定时 mode 选择器消失

- **WHEN** 用户关闭一个已绑定 skill 的绑定开关
- **THEN** 该 skill 从绑定列表中移除，mode 选择器不再显示

#### Scenario: 加载已有 agent 配置时回显 mode

- **WHEN** 打开一个已配置了 skills 绑定（含 mode）的 agent 详情页
- **THEN** 每个已绑定 skill 的 mode 选择器正确回显为保存时的选择

### Requirement: Admin UI AgentProfile 类型适配

前端 `AgentProfile.skills` 类型 SHALL 为 `Array<{id: string, mode: "always" | "on_demand"}>`。API 交互中 skills 字段 SHALL 使用此格式。

#### Scenario: 前端类型一致性

- **WHEN** 前端代码中引用 `AgentProfile.skills`
- **THEN** 类型为 `Array<{id: string, mode: "always" | "on_demand"}>` 而非 `string[]`

#### Scenario: 保存 agent 时发送结构化格式

- **WHEN** 用户在 agent 详情页保存配置
- **THEN** HTTP 请求体中 skills 字段为 `[{"id":"...","mode":"..."}]` 格式
