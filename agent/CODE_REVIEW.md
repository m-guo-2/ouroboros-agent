# Agent 代码审阅报告

审阅范围：`agent/` 目录下全部 61 个 Go 源文件，14 个包。
审阅目标：找出可能导致程序崩溃、挂起或无法继续执行的 bug，以及设计不合理之处。

---

## 第一部分：会导致程序崩溃或无法正确执行的 Bug

### BUG-01 [严重] `GracefulShutdown` 在未持锁时遍历 `sessionWorkers`

**文件**: `internal/runner/worker.go:264-270`

```go
workerMutex.Unlock()          // ← 264: 已释放锁

for sessionID, worker := range sessionWorkers {   // ← 264: 无锁遍历
    if worker.Processing {
        _ = storage.UpdateSession(sessionID, ...)
    }
}

workerMutex.Lock()             // ← 272: 再次获取锁
```

第一个锁区块（246-262）设置 `shuttingDown = true` 并取消上下文，然后释放锁。紧接着的 `for range sessionWorkers` 没有持锁，而此时 `drainWorker` goroutine 仍可能在修改 `sessionWorkers`（从队列弹出、修改 `Processing` 字段等）。

**后果**：Go map 并发读写会导致 **fatal: concurrent map iteration and map write**，进程直接 crash。

**修复方向**：将遍历 `sessionWorkers` 的逻辑移到持锁区块内，或在释放锁前复制一份 snapshot。

---

### BUG-02 [严重] `WebuiAdapter` 向已关闭的 channel 发送消息导致 panic

**文件**: `internal/channels/registry.go:183-228`

`Send()` 持 RLock 遍历 `subscribers` 并向 channel 发送。`Unsubscribe()` 持写锁、从列表中移除 channel 并 `close(ch)`。

时序：
1. `Send()` 持 RLock，完成发送，释放 RLock
2. `Unsubscribe()` 获取写锁，close(ch)，释放写锁
3. 下一次 `Send()` 持 RLock，向已 close 的 ch 发送 → **panic: send on closed channel**

即使 `Send` 中使用了 `select { case ch <- msg: default: }`，Go 中向已关闭的 channel 发送仍然会 panic，`select` 不能阻止这一点。

**修复方向**：`Unsubscribe` 不 close channel，只从列表中移除；或使用标记位/包装类型来安全地跳过已关闭的 channel。

---

### BUG-03 [严重] `drainWorker` 在处理失败后无条件覆写 `executionStatus`

**文件**: `internal/runner/worker.go:128-143`

```go
err := processOneEvent(ctx, worker, req)
if err != nil {
    _ = storage.UpdateSession(worker.SessionID, map[string]interface{}{
        "executionStatus": "interrupted",      // ← 设置为 interrupted
    })
}

// ... (135-138: 清除 CancelFunc)

_ = storage.UpdateSession(worker.SessionID, map[string]interface{}{
    "executionStatus": "completed",            // ← 无条件覆写为 completed
})
```

当 `processOneEvent` 返回错误时，先设 "interrupted"，随后又无条件设为 "completed"，丢失了错误状态。

**后果**：所有失败的请求在 DB 中都显示为 "completed"，错误被静默吞没，监控和调试完全失效。

---

### BUG-04 [严重] `SaveCompaction` 缺少 `created_at` 字段

**文件**: `internal/storage/compactions.go:19-27`

INSERT 语句只包含 8 个字段，没有 `created_at`：
```go
`INSERT INTO context_compactions
 (id, session_id, summary, archived_before_time, archived_message_count,
  token_count_before, token_count_after, compact_model)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
```

schema 中 `created_at INTEGER NOT NULL DEFAULT 0`，所以所有 compaction 记录的 `created_at` 都是 0。

**后果**：`GetLatestCompaction` 使用 `ORDER BY created_at DESC LIMIT 1` 查找最新记录。当所有记录 `created_at = 0` 时，返回结果不确定（取决于 SQLite 内部行序），可能返回错误的 compaction 记录。这会导致 `recall_context` 工具查询到错误的归档时间窗口，上下文压缩系统整体不可靠。

---

### BUG-05 [严重] `SendToChannel` 写入 messages 表缺少 `created_at`

**文件**: `internal/channels/registry.go:89-93`

```go
_, _ = storage.DB.Exec(
    `INSERT INTO messages (id, session_id, role, content, message_type, channel, trace_id, initiator, status)
     VALUES (?, ?, 'assistant', ?, ?, ?, ?, 'agent', 'sending')`,
    msgID, msg.SessionID, msg.Content, msg.MessageType, msg.Channel, msg.TraceID,
)
```

缺少 `created_at` 列。schema 默认值为 0。

**后果**：所有通过 `SendToChannel` 写入的 assistant 消息的 `created_at = 0`。由于消息查询 `ORDER BY created_at ASC`，这些消息会排在所有用户消息之前，打乱时间序。在 `reconstructHistoryFromMessages` 中，这会导致消息顺序错乱，LLM 收到的对话历史不正确。

---

### BUG-06 [中等] `HandleIncoming` 在 goroutine 中使用已完成请求的 Context

**文件**: `internal/dispatcher/dispatcher.go:250-261`

```go
writeJSON(w, http.StatusAccepted, ...)   // ← 响应已发送

go func() {
    result := Dispatch(r.Context(), msg)  // ← 使用 r.Context()
    ...
}()
```

HTTP handler 在发送 202 响应后启动 goroutine。当 handler 函数返回后，`r.Context()` 会被取消。goroutine 中的 `Dispatch` 拿到的是一个已取消或即将取消的 context。

**后果**：`Dispatch` 内部传递此 context 给 `logger.WithTrace` 和其他操作。虽然 `EnqueueProcessRequest` 中的 `drainWorker` 使用自己的 `context.Background()`，但 `Dispatch` 函数内的 dedup/user resolution/session lookup 阶段使用的 context 已经无效。如果这些操作检查 `ctx.Err()`，会提前退出。

**修复方向**：在启动 goroutine 前复制必要的 context 值，或使用 `context.Background()` 创建新 context。

---

### BUG-07 [中等] `RunAgentLoop` LLM 调用失败时静默返回成功

**文件**: `internal/engine/loop.go:99-107`

```go
response, err := config.LLMClient.Chat(ctx, ChatParams{...})
if err != nil {
    logger.Error(ctx, "LLM 调用失败", ...)
    break   // ← break 出循环
}
```

LLM 调用失败后 break 出 for 循环，然后落到函数末尾：
```go
return &AgentLoopResult{FinalText: finalText, ...}, nil  // ← 永远返回 nil error
```

同样，`ctx.Err()` 检查（line 87）也是 break 而非 return error。

**后果**：`processOneEvent` 调用方检查 `if err != nil` 来判断是否出错，但 `RunAgentLoop` 永远返回 nil error。LLM 故障、context 取消都不会被上层感知，导致用户收不到任何响应，也没有合理的错误处理路径。

---

### BUG-08 [中等] `recallSummary` 除零风险

**文件**: `internal/engine/ostools/recall.go:155`

```go
"compressionRate": fmt.Sprintf("%.0f%%",
    float64(c.TokenCountAfter)/float64(c.TokenCountBefore)*100),
```

如果 `TokenCountBefore == 0`（比如因 BUG-04 导致 compaction 记录不完整），会产生 `+Inf` 或 `NaN`。虽然 Go 不会 panic（float64 除零不 panic），但返回给 LLM 的数据会包含 `+Inf%` 或 `NaN%` 这样的无意义字符串。

---

### BUG-09 [中等] `SaveMessage` 返回值缺少 `CreatedAt`

**文件**: `internal/storage/messages.go:229-262`

`SaveMessage` 将 `now` 写入数据库的 `created_at` 列，但返回的 `MessageData` struct 中 `CreatedAt` 字段未赋值，为零值 0。

**后果**：任何依赖返回值的 `CreatedAt` 的调用方会得到错误的时间戳。目前调用方（`dispatcher.go:165`、`processor.go:944`）都忽略了返回值，所以暂时没有实际影响，但这是一个陷阱。

---

### BUG-10 [中等] `HTTPAdapter.client` 懒初始化存在数据竞争

**文件**: `internal/channels/registry.go:142-147`

```go
func (a *HTTPAdapter) httpClient() *http.Client {
    if a.client == nil {
        a.client = sharedlogger.NewClient(...)
    }
    return a.client
}
```

多个 goroutine 可能同时调用 `Send()`，进而并发执行 `httpClient()`。对 `a.client` 的 nil 检查和赋值没有同步保护。

**后果**：数据竞争。在实践中可能只是多创建几个 HTTP client 对象（因为 Go 的赋值是原子的对于指针），但严格来说是 UB，`go race detector` 会报告。

---

### BUG-11 [低等] `EnqueueProcessRequest` 的 `shuttingDown` 检查有 TOCTOU 窗口

**文件**: `internal/runner/worker.go:148-153, 214-240`

```go
workerMutex.Lock()
if shuttingDown {
    workerMutex.Unlock()
    return fmt.Errorf("agent is shutting down")
}
workerMutex.Unlock()     // ← 释放锁

// ... 中间进行 session 查询 ...

workerMutex.Lock()       // ← 重新获取锁，此时 shuttingDown 可能已变 true
worker.Queue = append(worker.Queue, queuedReq)
go drainWorker(worker)
workerMutex.Unlock()
```

在两次持锁之间的窗口期，`GracefulShutdown` 可能已执行，将 `shuttingDown` 设为 true。第二次持锁时没有再次检查 `shuttingDown`。

**后果**：关闭期间仍可能有新请求被入队并启动 `drainWorker`。

---

## 第二部分：设计不合理之处

### DESIGN-01 Session Key 逻辑重复定义

`runner.resolveSessionKey()` 和 `dispatcher.resolveSessionKey()` 实现了相同的逻辑但是独立的两个函数。如果其中一个修改而忘记同步另一个，会导致 session 碎片化——同一个对话被拆分到不同 session。

**建议**：抽取到公共位置（如 `types` 包或 `storage` 包），只保留一份实现。

---

### DESIGN-02 全局可变状态过多

项目中有大量包级全局变量承载运行时状态：
- `storage.DB` (全局 `*sql.DB`)
- `runner.sessionWorkers` (全局 map)
- `runner.shuttingDown` (全局 bool)
- `github.DefaultStore` (全局 store)
- `channels.adapters` (全局 map)
- `subagent.defaultManager` (全局 manager)
- `config.current` (全局 config)

**后果**：
1. 单元测试难以隔离——每个测试都在操作全局状态
2. 包之间的依赖是隐式的——通过全局变量耦合而非显式参数传递
3. 初始化顺序敏感——必须按特定顺序初始化全局变量

**建议**：引入 `App` 或 `Server` struct，将这些依赖作为字段注入，main 中组装。

---

### DESIGN-03 Subagent Jobs 内存永不释放

**文件**: `internal/subagent/manager.go`

`Manager.jobs` map 只有 `Start` 写入，没有任何删除逻辑。completed/failed/canceled 的 job 会永远留在内存中。

**后果**：长时间运行的 agent 会持续积累 job 数据（包括 Impacts 列表、完整的 result 文本），造成内存泄漏。

**建议**：添加 TTL 过期机制或 LRU 淘汰策略。

---

### DESIGN-04 `RunAgentLoop` 的错误契约不明确

`RunAgentLoop` 的签名返回 `(*AgentLoopResult, error)`，但在实际实现中，除了少数 JSON marshal 错误外，**永远返回 nil error**。LLM 调用失败、context 取消等重大错误都通过 `break` 退出循环并返回 nil error。

调用方 `processOneEvent` 以 `if err != nil` 判断是否需要错误处理，但实际上永远走不到。

**建议**：LLM 失败、ctx 取消时应返回有意义的 error，或者在 `AgentLoopResult` 中增加错误状态字段。

---

### DESIGN-05 `SendToChannel` 直接操作 `storage.DB` 绕过 storage 层

**文件**: `internal/channels/registry.go:89-100`

`SendToChannel` 直接 `storage.DB.Exec()` 写入 messages 表，绕过了 `storage.SaveMessage()` 函数。这导致：
1. 缺少 `created_at` 字段（BUG-05）
2. 缺少 `sender_name`、`sender_id` 等字段
3. 如果 `SaveMessage` 的逻辑变更（如增加校验），这里不会同步

**建议**：统一通过 `storage.SaveMessage()` 写入消息。

---

### DESIGN-06 SQLite `SetMaxOpenConns(1)` 的扩展性限制

**文件**: `internal/storage/db.go:27`

虽然 SQLite 单写者模型下这是正确设置，但 `SetMaxOpenConns(1)` 意味着所有 DB 操作（包括读）都在同一个连接上串行执行。当并发请求增多时，DB 会成为瓶颈。

**建议**：如果需要更高并发，可考虑读写分离（读连接池 + 单写连接），或迁移到 PostgreSQL。

---

### DESIGN-07 HTTP Server 在依赖就绪前开始接受请求

**文件**: `cmd/agent/main.go:102-108`

HTTP server 在 goroutine 中立即启动，而 GitHub skill store 的缓存加载在另一个 goroutine 中异步进行。在加载完成前，任何涉及 skill 的请求都可能拿到空的 skill context。

`/health` 端点通过 `github.DefaultStore.Ready()` 区分了 "starting" 和 "ready" 状态，但其他端点没有做类似检查。

**建议**：在 dispatcher 或 runner 入口处增加 readiness 检查，拒绝在 store 未就绪时处理请求。

---

### DESIGN-08 每次 HTTP 工具调用都创建新的 HTTP Client

**文件**: `internal/runner/wecom_builtin_tools.go:145, 168`
**文件**: `internal/engine/registry.go:87, 138`
**文件**: `internal/engine/tavily.go:165`

`sharedlogger.NewClient(...)` 在每次工具调用时创建新的 `*http.Client`。虽然 Go 的 HTTP client 有连接池，但每次新建 client 意味着不能复用已建立的连接。

**建议**：复用 package-level 或 registry-level 的 HTTP client 实例。

---

### DESIGN-09 `processOneEvent` 函数过于庞大

**文件**: `internal/runner/processor.go:646-1095`

`processOneEvent` 接近 450 行，承担了：
- Agent 配置加载
- LLM client 构建
- 工具注册（17+ 个工具）
- 历史消息重建
- Agent 循环执行
- 上下文压缩
- 消息吸纳（absorb）

**建议**：拆分为多个职责清晰的函数或 struct method，如 `buildToolRegistry`、`loadHistory`、`runWithAbsorb`、`checkpointContext` 等。

---

### DESIGN-10 没有 API 鉴权

所有 `/api/*` 端点和 `/api/channels/incoming` 端点没有任何认证或授权机制。任何能访问端口的人都能：
- 读写所有 agent 配置
- 注入消息到任意 session
- 修改 LLM API key 等敏感设置
- 删除 session 和消息

对于内部服务可以接受，但如果暴露到公网则是严重安全问题。

---

### DESIGN-11 错误处理策略不一致

项目中对错误有三种处理方式混用：
1. **返回 error**：如 `storage.GetSession()` 返回 `(*SessionData, error)`
2. **返回 nil 表示不存在**：如 `GetSession` 在 `ErrNoRows` 时返回 `(nil, nil)`
3. **静默忽略**：如 `_, _ = storage.SaveMessage(...)`、`_ = storage.UpdateSession(...)`

第 2 种模式（nil, nil 表示不存在）容易让调用方忘记检查 nil，导致空指针 panic。例如 `processOneEvent:651-652`：

```go
agentConfig, err := storage.GetAgentConfig(request.AgentID)
if err != nil || agentConfig == nil {
```

调用方需要同时检查 err 和 nil，这增加了认知负担和遗漏风险。

**建议**：对于"不存在"的情况，考虑返回明确的 sentinel error（如 `ErrNotFound`），统一错误处理模式。

---

### DESIGN-12 `recall_context` 吞掉数据库错误

**文件**: `internal/engine/ostools/recall.go:94-100`

```go
compaction, err := storage.GetLatestCompaction(sessionID)
if err != nil {
    return map[string]interface{}{
        "found":   false,
        "message": "No compressed context found for this session",
    }, nil   // ← 真实 DB 错误被当作"没有数据"处理
}
```

`GetLatestCompaction` 可能因数据库损坏、锁超时等原因失败，但这里将所有错误统一当作"没有 compaction"返回。

**建议**：区分 `sql.ErrNoRows`（真正的无数据）和其他错误（真正的故障）。

---

### DESIGN-13 Provider 判断逻辑脆弱

**文件**: `internal/runner/processor.go:684-696`

```go
if provider == "claude" || strings.Contains(credentials.BaseURL, "anthropic") {
    llmClient = engine.NewAnthropicClient(...)
} else {
    llmClient = engine.NewOpenAICompatibleClient(...)
}
```

provider 名称匹配依赖硬编码字符串，且只检查 "claude"（不含 "anthropic"）。如果用户在 agent 配置中写的 provider 是 "anthropic"，会走到 OpenAI 兼容分支，格式不对导致 API 调用失败。

同时，`providerCredentialsKey`（agents.go）中 "anthropic" 和 "claude" 都映射到同一组 key，但这里的 if 判断只检查 "claude"。

**建议**：统一使用 provider 注册表，而非在各处硬编码条件判断。

---

## 第三部分：汇总

| 编号 | 严重程度 | 类型 | 位置 | 简述 |
|------|---------|------|------|------|
| BUG-01 | 严重 | 并发 | worker.go:264 | GracefulShutdown 无锁遍历 map |
| BUG-02 | 严重 | 并发 | registry.go:183 | 向已关闭 channel 发送导致 panic |
| BUG-03 | 严重 | 逻辑 | worker.go:140 | executionStatus 被无条件覆写为 completed |
| BUG-04 | 严重 | 数据 | compactions.go:19 | SaveCompaction 缺少 created_at |
| BUG-05 | 严重 | 数据 | registry.go:89 | SendToChannel 缺少 created_at |
| BUG-06 | 中等 | 并发 | dispatcher.go:255 | 已取消的 request context 被 goroutine 使用 |
| BUG-07 | 中等 | 设计 | loop.go:105 | RunAgentLoop 错误被静默吞没 |
| BUG-08 | 中等 | 数据 | recall.go:155 | 除零风险 |
| BUG-09 | 中等 | 数据 | messages.go:258 | SaveMessage 返回值缺少 CreatedAt |
| BUG-10 | 中等 | 并发 | registry.go:142 | HTTPAdapter.client 懒初始化竞争 |
| BUG-11 | 低等 | 并发 | worker.go:148 | shuttingDown 检查的 TOCTOU 窗口 |
| DESIGN-01 | — | 重复 | runner+dispatcher | Session key 逻辑重复定义 |
| DESIGN-02 | — | 架构 | 全局 | 全局可变状态过多 |
| DESIGN-03 | — | 内存 | subagent | Jobs map 永不清理 |
| DESIGN-04 | — | 契约 | engine/loop | 错误契约不明确 |
| DESIGN-05 | — | 绕过 | channels | 绕过 storage 层直接写 DB |
| DESIGN-06 | — | 扩展 | storage/db | SQLite 单连接瓶颈 |
| DESIGN-07 | — | 启动 | cmd/main | 依赖未就绪时接受请求 |
| DESIGN-08 | — | 性能 | 多处 | 每次调用创建新 HTTP client |
| DESIGN-09 | — | 可维护 | processor | processOneEvent 过于庞大 |
| DESIGN-10 | — | 安全 | api | 无 API 鉴权 |
| DESIGN-11 | — | 一致性 | storage | 错误处理策略不一致 |
| DESIGN-12 | — | 错误处理 | recall.go | 吞掉数据库错误 |
| DESIGN-13 | — | 可维护 | processor | Provider 判断逻辑脆弱 |

---

**建议优先级**：BUG-01/02 可直接导致进程 crash，应最先修复。BUG-03/04/05 影响数据正确性，属于第二优先级。其余中低等级 bug 和设计问题可纳入技术债务清理。
