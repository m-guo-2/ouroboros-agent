## 1. Manager 层扩展

- [x] 1.1 在 `subagent.Manager` 上新增 `ListBySession(sessionID string) []*Job` 方法，遍历内存 jobs 按 sessionID 过滤并按 CreatedAt 降序返回副本
- [x] 1.2 在 `subagent.Manager` 上新增 `ReadEvents(jobID string) ([]map[string]interface{}, error)` 方法，从 `events.jsonl` 逐行解析 JSON，跳过损坏行

## 2. 后端 API

- [x] 2.1 新增 `GET /api/agent-sessions/{id}/subagent-jobs` handler，调用 `Manager.ListBySession` 返回 job 摘要列表（截断 task 到 200 字符，计算 impactCount）
- [x] 2.2 新增 `GET /api/subagent-jobs/{jobId}` handler，调用 `Manager.Get` + `Manager.ReadEvents` 返回 job 完整详情
- [x] 2.3 在 `api/router.go` 注册上述两个路由

## 3. Trace 关联与事件补全

- [x] 3.1 在 `executionStep` 结构体新增 `SubTraceID string` 字段（`json:"subTraceId,omitempty"`）
- [x] 3.2 在 `buildTrace` 的 `tool_result` 分支中，当 `tool` 为 `run_subagent_async` 时，从 `toolResult` 提取 `subTraceId` 写入 step
- [x] 3.3 在 `buildTrace` 的 switch 中新增 `subagent_reentry` case，生成对应 step（type=`subagent_reentry`，content 包含 jobId 和 reentry 轮次）

## 4. 前端 API 层

- [x] 4.1 在 `admin/src/api/types.ts` 新增 `SubagentJobSummary` 和 `SubagentJobDetail` 类型定义；在 `ExecutionStep` 中新增可选 `subTraceId` 字段
- [x] 4.2 新增 `admin/src/api/subagent-jobs.ts`，实现 `getSessionSubagentJobs(sessionId)` 和 `getSubagentJobDetail(jobId)` 函数

## 5. 前端 Hooks

- [x] 5.1 新增 `useSessionSubagentJobs(sessionId)` React Query hook，支持 processing 状态时自动轮询

## 6. Subagent Jobs Tab

- [x] 6.1 新增 `SubagentJobsPanel` 组件，展示 session 的 subagent job 列表（name、profile、status 标签、时间、impact 数）
- [x] 6.2 在 Monitor 页面的 tab 栏中添加 "Subagent" tab，接入 `SubagentJobsPanel`

## 7. Decision Inspector 增强

- [x] 7.1 新增 `SubagentStepCard` 组件，替代普通 tool_call 展示 `run_subagent_async` 步骤（展示 name、profile、task 摘要、状态、"查看执行过程"按钮）
- [x] 7.2 在 `FlatEventRow` 或 `RoundDetail` 中，当 `toolName` 为 `run_subagent_async` 时渲染 `SubagentStepCard`
- [x] 7.3 实现 subagent trace 内联查看：点击"查看执行过程"后通过 `subTraceId` 加载 trace，在 Decision Inspector 内用面包屑导航 + 复用 TraceStatsBar/RoundDetail 渲染
- [x] 7.4 处理 `subagent_reentry` step 类型的渲染（信息条，显示 re-entry 轮次）
