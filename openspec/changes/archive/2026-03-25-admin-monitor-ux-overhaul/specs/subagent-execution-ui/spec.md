## MODIFIED Requirements

### Requirement: Subagent Jobs Tab
Monitor 页面 SHALL 新增 "Subagent" tab（与 Memory、Delayed Tasks 并列），展示当前 session 的所有 subagent job 列表。

#### Scenario: session 有 subagent job
- **WHEN** 用户查看某 session 的 Monitor 页面，该 session 有 2 个 subagent job
- **THEN** Subagent tab 显示 2 行列表，每行包含：job name、profile 标签、状态标签（颜色区分 running/completed/failed/canceled）、创建时间、impact 数量
- **THEN** 点击某行展开该 job 的内嵌详情面板

#### Scenario: session 无 subagent job
- **WHEN** session 没有 subagent job
- **THEN** Subagent tab 显示空状态提示

#### Scenario: job 状态为 running
- **WHEN** 列表中存在 status 为 running 的 job
- **THEN** 该行显示动态运行指示（脉冲点）

#### Scenario: 展开 job 详情
- **WHEN** 用户点击某个 job 卡片
- **THEN** 卡片展开显示完整 task 描述、result（如果有）、error（如果有）、impacts 列表
- **THEN** 数据通过 `GET /api/subagent-jobs/{id}` 获取

#### Scenario: job 有 trace 可查看
- **WHEN** 展开的 job 有 `subTraceId`
- **THEN** 详情面板中显示 "查看执行过程" 按钮
- **THEN** 点击按钮后切换到 "对话" tab，Inspector 中打开对应的 subagent trace

### Requirement: Decision Inspector 中的 Subagent 步骤卡片
在 Decision Inspector 的 step 列表中，当 `tool_call` 的 `toolName` 为 `run_subagent_async` 时，系统 SHALL 渲染为 subagent 步骤卡片而非普通 tool_call 展示。

#### Scenario: 展示 subagent 步骤卡片
- **WHEN** Decision Inspector 渲染到一个 `tool_call` 步骤，`toolName` 为 `run_subagent_async`
- **THEN** 渲染为特殊卡片，展示：subagent name（从 toolInput.name 读取）、profile 标签、task 摘要（截断显示）
- **THEN** 卡片上有"查看执行过程"按钮

#### Scenario: 对应的 tool_result 包含 subTraceId
- **WHEN** 该 tool_call 的配对 tool_result 步骤的 `subTraceId` 非空
- **THEN** "查看执行过程"按钮可点击，点击后加载 subagent trace

## ADDED Requirements

### Requirement: Subagent Job 完整 Task 展示
SubagentJobsPanel 中的 job 卡片 SHALL 支持查看完整的 task 描述，不被截断。

#### Scenario: task 描述超过一行
- **WHEN** job 的 task 描述超过单行显示长度
- **THEN** 默认显示截断为 2 行，末尾显示 "展开" 按钮
- **THEN** 点击 "展开" 后显示完整 task 文本

### Requirement: Subagent Result/Error 展示
SubagentJobsPanel 的 job 详情 SHALL 展示 subagent 的执行结果和错误信息。

#### Scenario: job 成功完成
- **WHEN** job 的 status 为 "completed" 且有 result
- **THEN** 展开的详情面板中显示 "执行结果" 区域，渲染 result 文本（支持 Markdown）

#### Scenario: job 失败
- **WHEN** job 的 status 为 "failed" 且有 error
- **THEN** 展开的详情面板中显示红色 "错误信息" 区域，渲染 error 文本

#### Scenario: job 正在运行
- **WHEN** job 的 status 为 "running"
- **THEN** 展开的详情面板中显示 "执行中..." 状态指示

### Requirement: Subagent Impacts 展示
SubagentJobsPanel 的 job 详情 SHALL 展示 subagent 的影响记录列表。

#### Scenario: job 有 impacts
- **WHEN** job 详情中 impacts 数组非空
- **THEN** 详情面板中显示 "影响记录" 列表，每条 impact 包含：时间、工具名称、摘要

#### Scenario: job 无 impacts
- **WHEN** job 详情中 impacts 为空
- **THEN** 不显示 "影响记录" 区域

### Requirement: 两层 Subagent Trace 钻取
Decision Inspector SHALL 支持至少 2 层 subagent trace 钻取导航。

#### Scenario: 主 trace → subagent trace
- **WHEN** 用户在主 trace 中点击 subagent 步骤的 "查看执行过程"
- **THEN** Inspector 显示 subagent 的 trace 内容
- **THEN** 面包屑显示 "主 Trace → Subagent: xxx"

#### Scenario: subagent trace → 嵌套 subagent trace
- **WHEN** subagent trace 中也包含 `run_subagent_async` 步骤，用户点击 "查看执行过程"
- **THEN** Inspector 显示嵌套 subagent 的 trace 内容
- **THEN** 面包屑显示 "主 Trace → Subagent: xxx → Subagent: yyy"

#### Scenario: 面包屑导航返回
- **WHEN** 用户点击面包屑中的 "主 Trace" 或 "Subagent: xxx"
- **THEN** Inspector 返回对应层级的 trace 视图
