## Context

当前 subagent 的执行数据分布在三个位置：
1. **内存**：`subagent.Manager.jobs` map 存储 Job 元数据和 Impacts
2. **文件系统**：`data/subagents/<jobId>/` 下有 `job.json`、`events.jsonl`、`result.txt`
3. **日志文件**：subagent 的 LLM 调用、工具调用等通过 `logger.WithTrace(ctx, subTraceId, sessionId)` 写入结构化日志

Monitor 页面的 traces API 基于 `shared/logger.LogReader` 从日志文件读取 trace 事件，按 traceId 组装执行步骤。理论上 subagent 的 `subtrace-*` 已经在日志中，但 UI 没有入口查看，也没有 parent/child 关联。

admin-session-observability change 正在做 session facts 和 delayed tasks 的面板，与本 change 无冲突。

## Goals / Non-Goals

**Goals:**
- 在 Monitor 中能看到一个 session 所有 subagent job 的状态和关键信息
- 能从主 agent trace 的 `run_subagent_async` 工具调用直接跳转到 subagent 的完整执行 trace
- Subagent 自身的 trace（LLM 调用、工具调用、re-entry、compact）完整可查
- API 层提供 subagent job 查询能力，前端无需直接读文件系统

**Non-Goals:**
- 不将 subagent job 迁移到 SQLite（保持现有内存+文件存储，避免 schema 变更和迁移复杂度）
- 不做跨 session 的 subagent 全局搜索/聚合
- 不做 subagent 实时流式输出推送（当前无 WebSocket 基础设施）
- 不在本次解决 `Manager.jobs` 内存无上限的问题（CODE_REVIEW DESIGN-03，后续独立处理）

## Decisions

### D1: Subagent Job 数据来源 — 内存 + 文件系统混合读取

从 `Manager` 暴露 `ListBySession(sessionID) []*Job` 方法读取内存中的 job 列表。对于 job 详情（impacts、events timeline），从内存 Job 对象读取基本信息，从 `events.jsonl` 读取事件时间线。

**为什么不用 SQLite**：subagent job 生命周期与 session worker 绑定，进程重启后 job 数据不再有运维价值（旧 job 的日志文件仍在磁盘可回溯）。引入 DB 表会增加迁移和维护成本，当前阶段不值得。

**为什么不只读文件系统**：内存中的 Job 对象包含最新状态（running 中的 job 尚未写入终态到 job.json），需要内存数据保证实时性。

### D2: Trace 关联 — 利用已有的 subTraceId

`run_subagent_async` 工具的返回值中已包含 `subTraceId`。在 traces.go 的 `buildTrace` 中，当遇到 `tool_result` 且 `tool=run_subagent_async` 时，从 `toolResult` 中提取 `subTraceId` 并写入 `executionStep`。前端 Decision Inspector 检测到此字段后渲染为可点击的链接。

不需要额外的 log 事件或数据写入，纯粹利用已有数据。

### D3: Subagent Trace 事件补全

在 `traces.go` 的 `buildTrace` switch 中补充对 `subagent_reentry` 事件的处理，将其映射为一个新的 step type。这样通过 `subtrace-*` 查询 trace 时，subagent 的 re-entry 节点可见。

### D4: 前端组件结构

在 Decision Inspector 的 step 列表中，当 `tool_call` 的 `toolName` 为 `run_subagent_async` 时：
1. 显示 subagent 信息卡片（profile、task 摘要、当前状态）
2. 提供"查看执行过程"按钮，点击后在同一个 Decision Inspector 面板内加载 subagent 的 trace（通过 `subTraceId` 调用 `GET /api/traces/{subTraceId}`）
3. 用面包屑导航支持返回主 trace

这复用了已有的 trace 展示组件（`RoundDetail`、`FlatEventRow`），不需要为 subagent 造新的 trace 渲染器。

### D5: Session Subagent Jobs 面板

在 Monitor 页面新增一个 tab（与 Memory、Delayed Tasks 并列），展示当前 session 的所有 subagent job。列表展示：job name、profile、status、创建时间、耗时、impact 数量。点击单个 job 可跳转到其 trace。

## Risks / Trade-offs

- **[内存数据丢失]** → 进程重启后 `Manager.jobs` 清空，API 返回空列表。缓解：磁盘上 `job.json` 仍在，后续可加载已完成 job 的文件作为冷数据补充。当前接受此限制。
- **[subagent trace 日志量]** → subagent 的 LLM 调用产生与主 agent 同量级的日志，大量 subagent 任务会增加日志存储压力。缓解：现有日志按天轮转已覆盖此风险。
- **[trace 缓存污染]** → `subtrace-*` 的 trace 会被加入 `completedTraceCache`，占用 500 条缓存名额。缓解：subagent trace 通常在被查看后才会被加载和缓存，不会主动预热，影响可控。
