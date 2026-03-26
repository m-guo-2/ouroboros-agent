## Context

当前 Agent 系统通过 `ToolRegistry` 管理所有 Tool（builtin、skill、MCP），LLM 可以调用 `shell` builtin tool 在宿主机上执行命令。这对 Agent 自身的运维操作没有问题，但如果要执行 LLM 生成的任意代码（数据分析脚本、依赖安装、文件处理等），在宿主机上直接跑存在安全风险。

用户提供了基于 Daytona 的沙箱系统设计文档，明确了技术选型（Daytona Go SDK）、生命周期管理（lazy per-session）、环境变量三层模型、Tool 定义和安全策略。本设计文档基于该方案，聚焦与现有代码的集成点和关键实现决策。

现有 Agent 关键路径：ToolRegistry（`engine/registry.go`）注册 tool → LLM 选择 tool → ToolRegistry.Execute 分发执行 → 返回结果。Session 由 Runner（`runner/processor.go`）驱动，session 结束时无统一的清理钩子。

## Goals / Non-Goals

**Goals:**
- 引入 `agent/internal/sandbox` package，封装 Daytona SDK 的沙箱生命周期管理
- 沙箱 Tool（execute_command、write_file、read_file、list_files）通过 ToolRegistry 注册，对 LLM 透明
- 同一 session 内多次 tool call 共享同一沙箱环境（有状态）
- 沙箱按需创建、自动回收，成本可控
- 支持可选的沙箱初始化流程（上传文件、安装依赖）

**Non-Goals:**
- 不替换现有 shell builtin tool — 沙箱 Tool 是新增能力，不是替代
- 不实现沙箱快照/恢复 — Phase 2 增强
- 不实现文件下载回传 — Phase 2 增强
- 不实现多镜像选择 — MVP 使用单一默认镜像
- 不实现沙箱内 SubAgent — 当前只做 Tool 模式
- 不做自托管 Daytona 部署 — 使用 Daytona Cloud

## Decisions

### D1: 新增 `agent/internal/sandbox` package

独立 package，不混入 engine 或 runner。包含：
- `SandboxManager` — 管理所有 session 的沙箱生命周期
- `SessionSandbox` — 单个 session 的沙箱封装
- `SandboxConfig` — 配置参数
- `ToolHandler` — 4 个 Tool 的执行逻辑

**为什么独立 package？** 沙箱管理涉及外部 SDK 依赖、goroutine 生命周期、并发控制，职责清晰地隔离在一个包内，避免 engine 包膨胀。engine 只负责注册 tool，sandbox 负责执行。

### D2: Tool 注册方式 — 通过 ToolRegistry.RegisterBuiltin

沙箱的 4 个 Tool 作为 builtin 注册到 ToolRegistry，source 为 `"builtin"`，sourceName 为 `"sandbox"`。Tool executor 内部持有 `SandboxManager` 引用和当前 `sessionID`。

**为什么不用 skill 或 MCP？** 沙箱 Tool 是系统核心能力，不是用户配置的外挂。通过 builtin 注册，启动时即可用，无需外部服务依赖。

**sessionID 如何传递？** Tool executor 签名是 `func(ctx context.Context, input map[string]interface{}) (interface{}, error)`，没有 sessionID 参数。方案：通过 `context.Value` 传递 sessionID，Runner 在调用 ToolRegistry.Execute 前将 sessionID 注入 ctx。

```go
type ctxKey string
const sessionIDKey ctxKey = "sessionID"

func WithSessionID(ctx context.Context, sid string) context.Context {
    return context.WithValue(ctx, sessionIDKey, sid)
}

func SessionIDFromContext(ctx context.Context) string {
    if v, ok := ctx.Value(sessionIDKey).(string); ok {
        return v
    }
    return ""
}
```

### D3: 沙箱生命周期 — Lazy per-session + 后台巡检回收

沿用用户设计文档的方案：
- **创建**: 第一次 sandbox tool call 时 lazy 创建（GetOrCreate），不预创建
- **复用**: 同一 session 内所有 sandbox tool call 共享同一沙箱
- **回收**: 三个触发点 —— session 结束主动销毁、idle 超时巡检回收（默认 15min）、maxTTL 强制回收（默认 2h）
- **shutdown**: SandboxManager.Shutdown() 在进程退出时销毁所有活跃沙箱

GetOrCreate 使用 double-checked locking（读锁快路径 + 写锁慢路径）。

### D4: 并发控制 — 单沙箱串行执行

SessionSandbox 内置 `sync.Mutex`，同一沙箱内的命令串行执行。原因：
- 沙箱内文件系统共享，并发写同一文件会冲突
- LLM 的 parallel function calling 可能同时发出多个 sandbox tool call
- 串行化保证确定性，简化调试

不同 session 的沙箱天然隔离，无需额外同步。

### D5: 输出截断 — 保留首尾

沙箱命令输出可能很大，必须截断后返回给 LLM。截断策略保留首尾各一半（默认阈值 10KB），因为错误信息通常在末尾。

### D6: Session 结束时沙箱清理 — Runner 通知 SandboxManager

Runner 在 session 结束（正常结束或异常终止）时调用 `SandboxManager.Destroy(sessionID)`。这是除巡检回收外的主动清理路径。

需要在 `processor.go` 的 session 结束路径上添加此调用。SandboxManager 通过依赖注入传入 Runner。

### D7: 配置管理 — 通过环境变量 + SandboxConfig

Daytona API Key 通过环境变量 `DAYTONA_API_KEY` 读取。其他沙箱参数（镜像、超时、输出截断阈值等）通过 `SandboxConfig` 结构体配置，默认值内置，可通过环境变量覆盖。

沙箱功能按需启用：如果 `DAYTONA_API_KEY` 未设置，SandboxManager 不启动，沙箱 Tool 不注册，系统行为不变。

## Risks / Trade-offs

- **[risk] Daytona 服务不可用** — 沙箱创建或执行时 Daytona Cloud 可能超时或故障。→ 缓解：Tool 返回明确的错误信息给 LLM，LLM 可以选择重试或换方案。不做自动重试，避免级联延迟。
- **[risk] 沙箱泄漏** — 如果 Agent 进程异常退出（kill -9），内存中的 session→sandbox 映射丢失，Daytona 侧沙箱变成孤儿。→ 缓解：创建沙箱时打 `session_id` label，可通过定期 Daytona API 清理孤儿沙箱（不在 MVP 实现，作为运维手段）。
- **[risk] context.Value 传递 sessionID 不够显式** — 可能被遗忘或错传。→ 可接受：sandbox tool executor 在 sessionID 为空时返回明确错误；且 sessionID 注入点只有一处（Runner 调用 Execute 前）。
- **[trade-off] 单一默认镜像** — MVP 不支持按任务类型选镜像。如果 LLM 需要 Go 或 Rust 工具链，镜像里没有会失败。→ 可接受：MVP 覆盖 Python + Node.js 高频场景，按用户反馈扩展镜像或在 Phase 3 引入多镜像。
- **[trade-off] 串行执行降低吞吐** — 同一 session 的并发 tool call 被串行化。→ 可接受：单 session 通常不需要高并发沙箱执行；如果未来需要，可引入命令队列或放宽为按目录粒度加锁。
