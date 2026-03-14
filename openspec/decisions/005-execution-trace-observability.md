# 执行链路追踪：Agent 可观测性重构

- **日期**：2026-02-09
- **类型**：架构重构
- **状态**：已实施

## 背景

003 重构后 Agent 独立为 SDK-First 应用（端口 1996），但执行事件（思考、工具调用、工具结果）不再流经 Server 的 observation bus。MonitorView 的 DecisionTimeline、ToolCallDisplay、MessageTraceDetail 组件有完整的 UI，但收不到数据。具体问题：

1. 异步模式（`POST /process`）：Agent 生成事件后被静默消费
2. 流式模式（`POST /process/stream`）：事件通过 SSE 到直接调用方，但不进 observation bus
3. 历史查询依赖旧的 JSONL 日志，数据结构不匹配新的 Agent 事件流

## 决策

**Agent 主动上报执行事件到 Server，Server 负责存储 + 分发。**

核心原则：Agent 负责"发生了什么"，Server 负责"记住 + 转发给关心的人"。

## 变更内容

### 新增文件

| 文件 | 职责 |
|------|------|
| `server/src/services/execution-trace.ts` | 执行链路存储服务。SQLite 两张表（execution_traces + execution_steps），接收事件后同时写 DB + 推送 observation bus + 构建 decision_step |
| `server/src/routes/traces.ts` | API 路由。POST /api/traces/events（Agent 上报），GET /api/traces/:id（查询完整链路），GET /api/traces?sessionId=（列表查询） |

### 修改文件

| 文件 | 变更 |
|------|------|
| `server/src/index.ts` | 挂载 traces 路由 |
| `agent/src/services/server-client.ts` | 新增 `TraceEventPayload` 类型 + `reportTraceEvent()` 方法（fire-and-forget，不阻塞） |
| `agent/src/services/sdk-runner.ts` | processMessage 每一步上报事件：start → thinking → tool_call → tool_result → done |
| `admin/src/api/traces.ts` | traces API 模块（getById, getBySession, getRecent, getActive, getRecentSummaries） |
| `admin/src/components/features/monitor/monitor-page.tsx` | Monitor 页面内联渲染执行 trace（Think→Act→Observe），使用 TanStack Query 轮询 |

### 数据流

```
Agent processMessage()
  ├── yield event (原有：给流式调用者)
  └── server.reportTraceEvent() (新增：fire-and-forget POST 到 Server)
        ↓
Server POST /api/traces/events
  ├── SQLite: execution_traces + execution_steps (持久化)
  ├── observationBus.emit() (实时 SSE → MonitorView)
  └── buildDecisionStep() → decision_step event (→ DecisionTimeline)
```

### 数据库 Schema

```sql
execution_traces: id, session_id, agent_id, status, started_at, completed_at, tokens, cost
execution_steps:  trace_id, step_index, iteration, type, thinking, tool_*, content, error
```

## 考虑过的替代方案

1. **Server 订阅 Agent SSE**：Server 主动连接 Agent 的 /process/stream 并消费事件。否决原因：异步模式没有 SSE 流可订阅，且增加了 Server→Agent 的反向依赖。
2. **Agent 直接写 DB**：Agent 直连 Server 的 SQLite。否决原因：违反 Agent 无状态原则，且跨进程共享 SQLite 有锁竞争风险。

## 影响

- Monitor 页面可以查看每个会话的完整执行 trace（轮询刷新，processing 状态 2 秒间隔）
- 旧的 JSONL 日志系统仍然保留，Logs 页面可查看
- channel-dispatcher 的 `execution_start` emit 与 Agent 的 start 上报可能重复，观测总线可处理（幂等）
- 后续可基于 execution_traces 表做执行统计、费用追踪、性能分析
