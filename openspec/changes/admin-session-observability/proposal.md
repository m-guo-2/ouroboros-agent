## Why

Admin Monitor 页面目前只能看到会话的消息、压缩和执行链路，无法查看 session 的记忆内容（`session_facts`）和定时任务（`delayed_tasks`）。运维和调试时需要了解 agent 记住了什么、有哪些待执行的定时任务，目前只能直接查数据库，效率低且容易遗漏。

## What Changes

- 新增 **Session Memory API**：`GET /api/agent-sessions/{id}/facts`，返回指定 session 的所有记忆条目
- 新增 **Delayed Tasks API**：`GET /api/agent-sessions/{id}/delayed-tasks`，返回指定 session 的定时任务（支持按状态过滤）
- Monitor 页面新增 **Session Memory 面板**：在 session header 下方以 tab 或抽屉形式展示记忆列表（fact、category、时间）
- Monitor 页面新增 **Delayed Tasks 面板**：展示该 session 的定时任务列表（任务内容、计划执行时间、状态）

## Capabilities

### New Capabilities

- `session-facts-api`: 提供 session 记忆的只读 HTTP API，供 admin 前端查询
- `session-delayed-tasks-api`: 提供 session 定时任务的只读 HTTP API，支持状态过滤
- `session-observability-ui`: Monitor 页面新增记忆和定时任务的可视化面板

### Modified Capabilities

（无需修改已有 spec 级别行为）

## Impact

- **后端**：`agent/internal/api/` 新增两个 handler，复用已有 `storage` 层的查询函数
- **前端**：`admin/src/components/features/monitor/` 新增面板组件和对应 hooks
- **API 类型**：`admin/src/api/` 新增类型定义和 fetch 函数
- **路由**：`agent/internal/api/router.go` 注册新路由
- **无 Breaking Change**：纯增量添加，不影响现有功能
