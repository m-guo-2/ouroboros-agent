# 修复 channel-dispatcher 可观测性断层

- **日期**：2026-02-09
- **类型**：代码变更
- **状态**：已实施

## 背景

003 重构将消息处理从 `channel-router.ts`（全链路处理）拆分为 `channel-dispatcher.ts`（薄派发）+ `agent`（SDK 执行）。重构后，飞书收到消息能正常回复，但管理后台（MonitorView）看不到消息记录和决策过程。

根因：dispatcher 瘦身过度，丢掉了 5 项关键的可观测性基础设施：
1. 用户消息存储时 `sessionId: ""`（空），按 session 查不到
2. 不创建 `agent_sessions` 记录，session 列表为空
3. 不调用 `startMessageTrace()`，没有 `traceId`，无法查看执行详情
4. 不发射 `observationBus` 事件，实时监控空白
5. Agent App 通过 Data API 存消息只写 `messages` 表，不写 session 的 `messages` JSON 字段

## 决策

在不违背"dispatcher 不含业务逻辑"原则的前提下，将会话管理和消息追踪作为**基础设施**加回 dispatcher。Agent App 的执行逻辑不变。

## 变更内容

### `server/src/services/channel-dispatcher.ts`
- 新增 session 创建/获取逻辑（`agentSessionDb.getActiveSession / create`）
- 新增消息追踪（`startMessageTrace()` 生成 `traceId`）
- 用户消息存储带 `sessionId` + `traceId`
- 用户消息同步写入 session JSON（`agentSessionDb.addMessage`）
- 发射 `observationBus` 的 `execution_start` 事件
- 派发请求附带 `sessionId` + `traceId` 给 Agent App

### `agent/src/services/sdk-runner.ts`
- `ProcessRequest` 新增 `sessionId?` 和 `traceId?` 字段
- 优先使用 dispatcher 传入的 `sessionId`，避免重复创建 session
- 保存 assistant 消息时携带 `traceId` + `initiator`

### `agent/src/services/server-client.ts`
- `sendToChannel` 方法新增 `traceId?` 参数

### `server/src/routes/data.ts`
- `POST /api/data/messages`：新增 `agentSessionDb.addMessage()` 同步写入 session JSON
- `POST /api/data/channels/send`：接收 `traceId`，发送成功后同步写入 session JSON

## 影响

- MonitorView 可以正确显示飞书渠道的消息记录和 session 列表
- 助手消息关联 `traceId`，可查看执行详情（若后续 Agent App 补充结构化日志）
- 实时监控可以看到 `execution_start` 事件
- session JSON 和 messages 表保持双写一致
- Agent App 处理过程中的 reasoning/tool_call 等细节事件暂不通过 observation bus 推送（需要后续增加 Agent App → Server 的事件上报通道）
