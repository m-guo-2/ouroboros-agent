# Agent 三级日志体系 + 完整 LLM I/O 捕获

- **日期**：2026-02-27
- **类型**：架构重构
- **状态**：已实施

## 背景

原 Agent 使用 Go `log/slog` 输出单一 JSONL 文件，存在以下问题：

1. **日志层级缺失**：所有事件混在一个文件中，边界事件（HTTP 生命周期）、业务事件（LLM/工具调用）和详细数据（完整 LLM 请求/响应）无法区分
2. **LLM I/O 丢失**：只记录 token 用量和耗时摘要，不保存实际发给 LLM 的请求和返回的响应，排查问题时缺乏关键上下文
3. **无日志轮转**：单文件持续增长，无清理策略
4. **与 Server 端日志体系不统一**：Server 已有三级日志（boundary/business/detail），Agent 端未对齐

## 决策

### 1. Agent 三级日志体系

对齐 Server 端的三级分层：

| 级别 | 用途 | 文件 |
|------|------|------|
| `boundary` | 服务生命周期、HTTP 请求/响应 | `{logDir}/boundary/{date}.jsonl` |
| `business` | Trace 事件（llm_call、tool_call、thinking、done 等） | `{logDir}/business/{date}.jsonl` |
| `detail` | 调试诊断数据（大文本、变量快照） | `{logDir}/detail/{date}.jsonl` |

### 2. 完整 LLM I/O 单独存储

LLM 的完整请求和响应 JSON 体量较大，不适合内联到 JSONL 行内：

- 存储路径：`{logDir}/detail/llm-io/{traceID}_iter{iteration}.json`
- 内容格式：`{ traceId, iteration, time, request, response }`
- 关联方式：business 级 `llm_call` 事件包含 `llmIORef` 字段，值为文件名（不含扩展名）
- LLM 客户端在 `Chat()` 方法中捕获原始 JSON 字节（`RawRequest`/`RawResponse`），引擎循环调用 `logger.WriteLLMIO()` 写入

### 3. 日期分文件 + 自动清理

- 每级日志按日期分文件：`2026-02-27.jsonl`
- 后台 goroutine 每小时检查，按保留天数清理旧文件
- 默认保留：boundary 30 天、business 14 天、detail 7 天、llm-io 7 天

### 4. Context 传播

使用 `context.Context` 自动注入 `traceID` 和 `sessionID`：
- `logger.WithTrace(ctx, traceID, sessionID)` 写入 context
- 所有日志函数从 context 中提取并写入每条记录

### 5. 控制台输出

开发环境下同步输出到 stderr，带 ANSI 颜色区分级别，方便实时观察。

## 变更内容

### Agent (Go)

- **新增 `agent/internal/logger/logger.go`**：完整的三级日志实现
  - `Init(logDir)` 初始化目录结构 + 启动清理 goroutine
  - `Boundary/Business/Detail/Error/Warn` 公开函数
  - `WriteLLMIO` 写入独立 LLM I/O 文件并返回 ref
  - `ReadLLMIO` 读取 LLM I/O 文件
  - `Flush` 刷新缓冲
- **`agent/internal/engine/llm.go`**：`LLMResponse` 新增 `RawRequest`/`RawResponse` 字段，Anthropic 和 OpenAI 客户端捕获原始 JSON
- **`agent/internal/engine/loop.go`**：LLM 调用后写入 LLM I/O，`llm_call` 事件携带 `llmIORef`
- **`agent/cmd/agent/main.go`、`handlers.go`、`worker.go`、`processor.go`**：全部替换 `log/slog` 为 `logger.*`

### Server (TS)

- **重写 `agent-log-reader.ts`**：适配新的日期分文件 + business/*.jsonl 结构
  - `readBusinessJsonl()` 读取 business 级日志
  - `readLLMIO(ref)` / `listLLMIORefs(traceId)` 读取 LLM I/O 文件
- **重写 `traces.ts`**：新增 `GET /api/traces/:id/llm-io` 和 `GET /api/traces/:id/llm-io/:ref` 端点

### Admin (React)

- **`types.ts`**：`ExecutionStep` 新增 `llmIORef?: string`
- **`traces.ts`**：API 客户端新增 `getLLMIO` 和 `listLLMIORefs`
- **`monitor-page.tsx`**：
  - 新增 `LLMIOViewer` 组件（模态框展示完整请求/响应 JSON）
  - `IterationGroup` 添加「I/O」按钮，有 `llmIORef` 时可点击查看

## 影响

- Agent 日志目录从单文件 `LOG_FILE` 改为目录 `LOG_DIR`（默认 `logs/`）
- Server 的 `agent-log-reader` 需能访问 Agent 的 `logs/` 目录（同机部署，路径通过 `resolveAgentLogDir` 解析）
- LLM I/O 文件可能较大（数十 KB 到数 MB），7 天自动清理防止磁盘膨胀
- Monitor 页面每个 Iteration 可直接查看完整 LLM 交互，大幅提升排查效率
