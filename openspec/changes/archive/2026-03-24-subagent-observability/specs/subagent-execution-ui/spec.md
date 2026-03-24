## ADDED Requirements

### Requirement: Subagent Jobs Tab
Monitor 页面 SHALL 新增 "Subagent" tab（与 Memory、Delayed Tasks 并列），展示当前 session 的所有 subagent job 列表。

#### Scenario: session 有 subagent job
- **WHEN** 用户查看某 session 的 Monitor 页面，该 session 有 2 个 subagent job
- **THEN** Subagent tab 显示 2 行列表，每行包含：job name、profile 标签、状态标签（颜色区分 running/completed/failed/canceled）、创建时间、impact 数量
- **THEN** 点击某行可导航到该 job 的 trace 视图

#### Scenario: session 无 subagent job
- **WHEN** session 没有 subagent job
- **THEN** Subagent tab 显示空状态提示

#### Scenario: job 状态为 running
- **WHEN** 列表中存在 status 为 running 的 job
- **THEN** 该行显示动态运行指示（如脉冲点或 spinner）

### Requirement: Decision Inspector 中的 Subagent 步骤卡片
在 Decision Inspector 的 step 列表中，当 `tool_call` 的 `toolName` 为 `run_subagent_async` 时，系统 SHALL 渲染为 subagent 步骤卡片而非普通 tool_call 展示。

#### Scenario: 展示 subagent 步骤卡片
- **WHEN** Decision Inspector 渲染到一个 `tool_call` 步骤，`toolName` 为 `run_subagent_async`
- **THEN** 渲染为特殊卡片，展示：subagent name（从 toolInput.name 读取）、profile 标签、task 摘要（截断显示）
- **THEN** 卡片上有"查看执行过程"按钮

#### Scenario: 对应的 tool_result 包含 subTraceId
- **WHEN** 该 tool_call 的配对 tool_result 步骤的 `subTraceId` 非空
- **THEN** "查看执行过程"按钮可点击，点击后加载 subagent trace

#### Scenario: tool_result 尚无 subTraceId（subagent 仍在运行）
- **WHEN** tool_call 后尚无配对的 tool_result
- **THEN** 卡片显示"运行中..."状态，"查看执行过程"按钮禁用

### Requirement: Subagent Trace 内联查看
点击"查看执行过程"后，Decision Inspector SHALL 在当前面板内加载并展示 subagent 的 trace 步骤，复用已有的 RoundDetail 和 FlatEventRow 组件。

#### Scenario: 成功加载 subagent trace
- **WHEN** 用户点击"查看执行过程"，系统通过 `GET /api/traces/{subTraceId}` 加载 subagent trace
- **THEN** 面板顶部显示面包屑导航："主 Trace > Subagent: {name}"
- **THEN** 面板内容替换为 subagent trace 的步骤列表（使用 TraceStatsBar + RoundDetail 渲染）
- **THEN** 用户可通过面包屑返回主 trace

#### Scenario: subagent trace 加载失败
- **WHEN** `GET /api/traces/{subTraceId}` 返回 404 或错误
- **THEN** 面板显示错误提示"无法加载 subagent 执行记录"，不影响主 trace 展示

#### Scenario: subagent trace 包含 reentry 步骤
- **WHEN** subagent trace 中包含 `subagent_reentry` 类型的步骤
- **THEN** 渲染为信息条，显示"Re-entry 第 N 轮"

### Requirement: 前端 API 客户端扩展
`admin/src/api/` SHALL 新增 subagent jobs 相关的 API 客户端函数和类型定义。

#### Scenario: 类型定义
- **WHEN** 前端代码导入 subagent API 类型
- **THEN** 可使用 `SubagentJobSummary`（列表项类型，含 id、name、profile、status、subTraceId、createdAt、updatedAt、impactCount、task）和 `SubagentJobDetail`（详情类型，含完整 impacts 和 events）

#### Scenario: API 客户端函数
- **WHEN** 前端调用 subagent API
- **THEN** 可使用 `getSessionSubagentJobs(sessionId)` 和 `getSubagentJobDetail(jobId)` 两个函数

### Requirement: Subagent 数据 Hook
`admin/src/` SHALL 提供 React Query hook 用于获取 subagent job 数据。

#### Scenario: 使用 session subagent jobs hook
- **WHEN** 组件调用 `useSessionSubagentJobs(sessionId)` hook
- **THEN** 返回 React Query 结果，包含 `data`（SubagentJobSummary 数组）、`isLoading`、`error`
- **THEN** 自动轮询刷新（当 session 处于 processing 状态时）
