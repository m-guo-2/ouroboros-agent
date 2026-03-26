## ADDED Requirements

### Requirement: Persona list view
Admin UI SHALL 在 agent 详情页的 Personas 标签页中展示所有 Persona 卡片。每张卡片显示名称、模型概览、skills 概览、关联群数量。提供编辑、复制操作。

#### Scenario: View persona cards
- **WHEN** 管理员进入 Personas 标签页，有 2 个 Persona
- **THEN** 展示 2 张卡片，显示名称、模型、skills 标签、"已分配 N 个群"

#### Scenario: Empty state
- **WHEN** 无 Persona
- **THEN** 展示空状态提示和"新建 Persona"按钮

### Requirement: Persona create/edit form
Admin UI SHALL 提供 Persona 创建和编辑共用表单。display_name 必填。每个覆盖字段用 toggle 控制：关闭 = 不覆盖（NULL），开启 = 编辑。toggle 关闭时灰色显示 agent 默认值。

#### Scenario: Create persona
- **WHEN** 管理员输入名称，开启 system_prompt toggle 并输入内容，保存
- **THEN** 调用 POST API 创建 Persona

#### Scenario: Edit persona
- **WHEN** 管理员编辑已有 Persona，关闭 system_prompt toggle
- **THEN** 调用 PUT API，systemPrompt 传 null

#### Scenario: Toggle off shows default
- **WHEN** toggle 关闭
- **THEN** 该字段区域以灰色文字显示 agent 默认值

### Requirement: Persona clone
Admin UI SHALL 支持复制 Persona。点击"复制"后弹出名称输入，确认后创建新 Persona。

#### Scenario: Clone persona
- **WHEN** 管理员点击"复制"，输入新名称
- **THEN** 调用 clone API，新 Persona 出现在列表中

### Requirement: Unconfigured group queue
Admin UI SHALL 在群分配标签页顶部展示"新发现的群"队列。每行显示 session_key、渠道类型、最近活跃时间。提供 Persona 下拉选择 + "确认"和"忽略"操作。

#### Scenario: New groups appear
- **WHEN** 有 2 个未分配群
- **THEN** 页面顶部展示 2 条记录，群分配标签显示数字角标

#### Scenario: Assign from queue
- **WHEN** 管理员在下拉框选择 Persona 并点击"确认"
- **THEN** 调用 POST API 创建分配，该群从队列移到"已分配"列表

#### Scenario: Ignore from queue
- **WHEN** 管理员点击"忽略"
- **THEN** 创建 persona_id 为空的分配记录，该群从队列消失，在已分配列表中显示"默认配置"

### Requirement: Group assignment list
Admin UI SHALL 展示已分配群的列表。每行显示 group_name、session_key、当前 Persona（下拉可切换）。支持手动添加群和删除分配。

#### Scenario: View assignments
- **WHEN** 有 4 条群分配
- **THEN** 展示 4 行，Persona 列为下拉选择器（含"默认配置"选项）

#### Scenario: Switch persona inline
- **WHEN** 管理员在列表中将某群的下拉框从"客服助手"切换为"技术专家"
- **THEN** 调用 PUT API 更新分配

#### Scenario: Manual add group
- **WHEN** 管理员点击"添加群"，输入 session_key 和群名称，选择 Persona
- **THEN** 调用 POST API 创建分配

#### Scenario: Delete assignment
- **WHEN** 管理员删除某条分配
- **THEN** 该群回到"未分配"队列
