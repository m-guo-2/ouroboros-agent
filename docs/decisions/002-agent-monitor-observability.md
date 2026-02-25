# Agent Monitor 可观测性界面

- **日期**：2026-02-08
- **类型**：架构决策 | 代码变更
- **状态**：已实施

## 背景

Admin 需要一个可观测性界面来查看 Agent 回复消息时的思考过程、工具调用详情和执行统计。界面以群聊/私聊的形式组织，支持跨渠道（飞书/企微/WebUI）查看所有 Agent 的活动。

现有系统已有结构化日志（三级 JSONL + trace/span），但只写不读；消息记录中不包含 traceId，无法关联到执行日志；前端的 Debug 面板仅支持当前流式会话，不支持历史回看和跨渠道监控。

## 决策

采用**日志关联 + 实时事件总线**的双通道架构：
- 历史可观测性：通过 traceId 将消息记录与 JSONL 日志关联，新增日志读取 API
- 实时可观测性：新增 ObservationBus（EventEmitter）+ SSE 端点，channel-router 处理消息时同步推送事件

前端新增 Monitor 页面作为统一会话追踪入口。

## 变更内容

**后端新增文件：**
- `server/src/services/observation-bus.ts` — 基于 EventEmitter 的观测事件总线
- `server/src/services/logger/reader.ts` — JSONL 日志文件读取器（按 traceId/spanId/时间查询）
- `server/src/routes/logs.ts` — 日志查询 API（`/api/logs/trace/:traceId`、`/api/logs/recent`）
- `server/src/routes/monitor.ts` — Monitor SSE 端点（`/api/monitor/stream`，后端保留供其他消费者使用）

**后端修改文件：**
- `server/src/services/channel-router.ts` — 增强执行日志（tool_input/tool_output/observation 事件推送）、消息保存时附带 traceId
- `server/src/services/database.ts` — `AgentMessageRecord` 增加 `traceId` 字段、`agentSessionDb` 增加 `getFiltered()` 方法
- `server/src/routes/agent-sessions.ts` — 会话列表支持 `agentId/channel/userId` 过滤，返回 agentDisplayName
- `server/src/index.ts` — 注册 `/api/logs` 和 `/api/monitor` 路由

**前端（已在后续重构中重写）：**
- Monitor 页面位于 `admin/src/components/features/monitor/monitor-page.tsx`
- 使用 TanStack Query 轮询替代 SSE（processing 状态 2-3 秒轮询，空闲 10 秒）
- 统一会话视图：左侧会话列表 + 右侧消息交互详情（用户消息 → 执行 trace → 助手回复）

## 影响

- `AgentMessageRecord` 新增 `traceId` 字段，已有数据的旧消息不会有此字段，会优雅降级
- ObservationBus 是内存事件总线，不影响已有 channel-router 的处理流程（emit 是非阻塞的）
- 日志读取器目前是全文件扫描，对于大量日志可能有性能瓶颈，后续可考虑索引优化
