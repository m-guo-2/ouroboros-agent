# Agent 结构化日志：双输出 + Context 自动注入 Trace

- **日期**：2026-02-25
- **类型**：架构决策 / 代码变更
- **状态**：已实施

## 背景

Agent (Go) 之前使用 `log.Printf` 和 `fmt.Printf` 输出日志，存在三个问题：

1. 仅有控制台输出，无文件持久化，不利于后续可观测性（日志检索、告警）
2. 非结构化文本，难以被日志系统解析
3. traceId/sessionId 靠手工拼字符串 `[react-engine] sid=xxx tid=xxx`，不一致且容易遗漏

## 决策

1. 创建 `agent/internal/logger` 包，基于 Go 1.21 标准库 `log/slog`
2. 双输出：控制台 TextHandler + JSONL 文件 JSONHandler（通过 `multiHandler` 扇出）
3. traceId/sessionId 通过 `context.Context` 传递，`contextHandler` 在每条日志记录中自动注入
4. 所有调用点统一使用 `slog.InfoContext(ctx, ...)` / `slog.ErrorContext(ctx, ...)` 等标准 API

## 变更内容

- **新增** `agent/internal/logger/logger.go`
  - `Init(logFile)` — 初始化双输出，设为 `slog.SetDefault`
  - `WithTrace(ctx, traceID, sessionID) context.Context` — 往 ctx 注入 trace
  - `contextHandler` — 拦截每条日志，从 ctx 提取 traceId/sessionId 自动加到 record
  - `multiHandler` — 扇出到 console + JSONL file
- **修改** `agent/cmd/agent/main.go` — 初始化 logger，所有 `log.Printf` 替换为 `slog.Info/Error/Warn`
- **修改** `agent/internal/runner/processor.go` — `createTraceReporter` 接受 ctx，内部用 `slog.*Context(ctx, ...)`
- **修改** `agent/internal/runner/worker.go` — `drainWorker` / `EnqueueProcessRequest` 中 ctx 注入 trace
- **修改** `agent/internal/handlers/handlers.go` — 替换注释掉的 `fmt.Printf` 为 `slog.Error`

## 使用模式

```go
// 注入 trace 到 ctx（一次注入，全链路自动携带）
ctx = logger.WithTrace(ctx, traceID, sessionID)

// 任何位置只要传了这个 ctx，日志自动带 traceId + sessionId
slog.InfoContext(ctx, "tool_call", "tool", toolName)
```

JSONL 输出示例：
```json
{"time":"2026-02-25T10:00:00Z","level":"INFO","msg":"tool_call","tool":"send_channel_message","traceId":"trace-123","sessionId":"sess-456"}
```

## 影响

- 零外部依赖，纯标准库
- JSONL 文件路径通过 `LOG_FILE` 环境变量配置，默认 `logs/agent.jsonl`
- 后续接 ELK / Loki 等日志系统时直接对接 JSONL 文件即可
- 新增代码只需确保 ctx 传递链路完整，traceId 自动出现在所有日志中
