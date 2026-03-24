## Why

Subagent 执行过程对运维和调试完全不可见。当前 Monitor 页面的 Trace 面板只展示主 agent 的执行链路，subagent 的 LLM 调用、工具调用、re-entry、取消等关键步骤无法在 UI 中查看。Subagent job 数据仅存在于内存和文件系统，没有查询 API，也没有与主 agent trace 的关联——调试 subagent 问题需要手动翻日志文件和磁盘 JSON，效率极低。

## What Changes

- 新增 **Subagent Jobs API**：`GET /api/agent-sessions/{id}/subagent-jobs`，返回指定 session 的所有 subagent job 列表（状态、profile、耗时、impact 摘要）
- 新增 **Subagent Job Detail API**：`GET /api/subagent-jobs/{jobId}`，返回单个 job 的完整信息（含 impacts、事件时间线）
- **Trace 关联**：在主 agent trace 的 `tool_call` 步骤中（`run_subagent_async`）嵌入 `subTraceId`，使 UI 可直接跳转到 subagent 的执行 trace
- **Subagent Trace 支持**：traces API 的 `buildTrace` 补充对 `subagent_reentry` 事件的处理，使 subagent trace 完整可查
- **Monitor UI 增强**：在 Decision Inspector 中，当检测到 `run_subagent_async` 工具调用时，展示 subagent 执行面板（状态、进度、可展开查看 subagent 自身的 trace 步骤）

## Capabilities

### New Capabilities

- `subagent-jobs-api`: 提供 session 维度的 subagent job 查询 API 和单 job 详情 API，供 admin 前端展示 subagent 执行历史
- `subagent-trace-linking`: 在主 agent trace 中嵌入 subagent trace 引用，在 subagent trace 中补全缺失的事件类型，实现 parent/child trace 双向关联
- `subagent-execution-ui`: Monitor 页面 Decision Inspector 中新增 subagent 执行过程的可视化展示，支持查看 subagent trace 步骤和 job 状态

### Modified Capabilities

（无需修改已有 spec 级别行为）

## Impact

- **后端**：`agent/internal/api/` 新增 subagent jobs handler；`traces.go` 的 `buildTrace` 补充事件类型；`subagent/manager.go` 需暴露按 session 查询 jobs 的方法
- **前端**：`admin/src/components/features/monitor/` 新增 subagent 执行面板组件；`admin/src/api/` 新增类型定义和 API 客户端
- **路由**：`agent/internal/api/router.go` 注册新路由
- **数据层**：当前 subagent job 仅在内存+文件系统，本次不迁移到 SQLite，而是从 Manager 内存和磁盘 events.jsonl 读取（后续可考虑持久化）
- **无 Breaking Change**：纯增量添加，不影响现有功能
