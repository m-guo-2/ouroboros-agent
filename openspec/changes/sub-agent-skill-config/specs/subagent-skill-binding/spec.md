## ADDED Requirements

### Requirement: Subagent skill 绑定存储
系统 SHALL 在 `agent_configs` 表中存储每个 subagent profile 的 skill 绑定配置。配置格式为 profile → skill ID 列表的 JSON 映射。未配置的 profile 不加载任何 skill。

#### Scenario: 保存子 agent skill 配置
- **WHEN** 管理员通过 API 为 agent 设置 `subagentSkills` 字段（如 `{"developer": ["skill-A", "skill-B"]}`）
- **THEN** 系统将配置持久化到 `agent_configs.subagent_skills` 列

#### Scenario: 读取子 agent skill 配置
- **WHEN** 系统通过 API 返回 agent 配置
- **THEN** 响应中 SHALL 包含 `subagentSkills` 字段，反映当前持久化的映射

#### Scenario: 未配置时的默认行为
- **WHEN** agent 的 `subagentSkills` 为空或未设置
- **THEN** 所有 profile 的子 agent 不加载任何额外 skill（向后兼容）

### Requirement: Subagent 启动时加载 profile 绑定的 skill
Runner 启动子 agent 时 SHALL 根据 profile 查找对应的 skill 绑定，编译 SkillContext，并将 skill 文档和工具注入子 agent 运行环境。

#### Scenario: profile 有 skill 绑定
- **WHEN** 启动 profile 为 "developer" 的子 agent，且 `subagentSkills["developer"]` 包含 `["skill-A"]`
- **THEN** 系统 SHALL 将 skill-A 的文档追加到子 agent 的 system prompt
- **THEN** 系统 SHALL 将 skill-A 定义的工具注册到子 agent 的工具列表中

#### Scenario: profile 无 skill 绑定
- **WHEN** 启动 profile 为 "file_analysis" 的子 agent，且 `subagentSkills` 中无 "file_analysis" 条目
- **THEN** 子 agent 仅使用 profile 默认的内置工具，不注入任何 skill

#### Scenario: 绑定的 skill 不存在或已禁用
- **WHEN** `subagentSkills["developer"]` 中引用的 skill ID 在系统中不存在或已禁用
- **THEN** 系统 SHALL 静默跳过该 skill，不影响其他 skill 的加载和子 agent 的启动

### Requirement: Subagent skill 统一以 always 模式加载
所有绑定到子 agent 的 skill SHALL 以 always 模式加载（完整文档 + 全部工具），不支持 on_demand 模式。

#### Scenario: skill 加载模式
- **WHEN** skill 被绑定到某个 subagent profile
- **THEN** 该 skill 的完整 readme 和全部工具 SHALL 注入子 agent，等同于主 agent 中 always 模式的行为

### Requirement: Skill 工具不受 profile 内置工具白名单过滤
通过 subagent skill 绑定注入的工具 SHALL 独立于 `filterToolsByProfile` 的内置工具白名单，直接可用。

#### Scenario: skill 工具可用性
- **WHEN** developer profile 的子 agent 绑定了定义 `code_review` 工具的 skill
- **THEN** 子 agent 的工具列表中 SHALL 包含 `code_review`，即使该工具不在 developer 的内置白名单中

### Requirement: Admin UI 子 agent skill 配置
管理员 SHALL 可以在 Admin UI 中为每个 subagent profile 选择要绑定的 skill。

#### Scenario: 配置界面展示
- **WHEN** 管理员打开 agent 详情页的配置 tab
- **THEN** 界面 SHALL 展示每个 subagent profile 的 skill 选择区域

#### Scenario: 选择并保存 skill 绑定
- **WHEN** 管理员为 developer profile 选择了 skill-A 和 skill-B，然后点击保存
- **THEN** 系统 SHALL 将 `subagentSkills: {"developer": ["skill-A-id", "skill-B-id"]}` 提交到 API

#### Scenario: 加载已有配置回显
- **WHEN** agent 已有 `subagentSkills` 配置
- **THEN** 打开配置页面时 SHALL 正确回显每个 profile 已绑定的 skill
