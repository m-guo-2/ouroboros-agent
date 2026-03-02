# 可观测性简化：JSONL 按需读取，去除落库与实时总线

- **日期**：2026-02-26
- **类型**：架构重构
- **状态**：已实施

## 背景

原有可观测性依赖：Agent HTTP 上报 → execution-trace SQLite 落库 → observation-bus 实时 SSE。存在 SQLite 持久化、内存 EventEmitter、双通道等多重复杂度。需求：只考虑 Agent 观测链路、不落库、基于 JSONL 按需读取、尽量简洁。

## 决策

1. **Trace 不再落库**：移除 execution-trace 的 SQLite、handleTraceEvent、POST /api/traces/events
2. **移除 ObservationBus 与 SSE**：不实时推送，Monitor 改为 1 秒轮询 GET /api/traces/:id
3. **Agent 只写 JSONL**：loop 内所有 trace 事件（thinking、tool_call、tool_result、done 等）写 slog → agent.jsonl
4. **Server 按需读取**：新增 agent-log-reader，从 agent.jsonl 按 traceId 解析并组装为 ExecutionTrace 格式

## 变更内容

- **Agent (Go)**
  - loop.go：完善 trace 事件字段（thinking、toolInput、toolResult、toolSuccess、toolDuration、totalCostUsd 等）
  - worker.go：移除 ReportTraceEventSync，改用 slog.Info 记录 trace 开始
- **Server (TS)**
  - 新增 `agent-log-reader.ts`：getTraceByTraceId、getTraceIdsBySessionId
  - 重写 `traces.ts` 路由：仅保留 GET /:id（从 JSONL 读），兼容 GET /active、/recent-summaries（返回空）
  - 移除 monitor 路由、channel-dispatcher 中 handleTraceEvent 调用
  - agent-sessions 删除会话时不再清理 execution_traces 表
- **Admin**
  - Monitor 轮询间隔改为 1 秒（原 2 秒）

## 影响

- Agent 日志路径：默认 `logs/agent.jsonl`（相对 CWD）。Server 读取时优先 `server/data/logs/agent.jsonl`，fallback `agent/logs/agent.jsonl`
- 需保证 Agent 与 Server 能访问同一 JSONL 文件（同机或共享目录）。可通过 `LOG_FILE` / `AGENT_LOG_PATH` 配置
- execution-trace.ts、observation-bus.ts、monitor.ts 仍存在但未被引用，可后续删除
- 消息表仍保存 traceId，Monitor 通过 session.messages 中的 traceId 拉取 trace 详情
