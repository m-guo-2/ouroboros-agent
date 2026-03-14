# 可观测性优化：轻量 llm_call 事件 + Iteration 分组 Monitor

- **日期**：2026-02-26
- **类型**：架构决策 + 代码变更
- **状态**：已实施

## 背景

上一轮重构（017）完成了 context-based trace propagation，但遗留 4 个问题：
1. `model_io` 事件记录完整的 systemPrompt + messages + tools，每轮 LLM 调用日志体积巨大且包含敏感信息
2. `totalCostUsd` 始终为 0（从未计算），展示误导
3. `TraceStarted` 字段冗余（在 `EnqueueProcessRequest` 设为 true 后 `processOneEvent` 里的 fallback 分支永远不执行）
4. 前端 Monitor 以平铺方式展示所有 steps，无法直观看出"每轮做了什么"

## 决策

1. **`model_io` → `llm_call`**：只记录 `model/inputTokens/outputTokens/durationMs/stopReason/costUsd`，去掉完整 I/O payload
2. **增加 LLM 耗时测量**：在 `RunAgentLoop` 中用 `time.Now()` 包围 `Chat()` 调用
3. **增加 `estimateCost` 函数**：内置主流模型定价表（Claude/GPT-4o）粗估 USD，累积后写入 `done` 事件
4. **前端按 Iteration 分组**：每个 Iteration 独立可折叠，header 显示 model/tokens/duration；tool_call + tool_result 配对展示

## 变更内容

| 文件 | 变更 |
|------|------|
| `agent/internal/engine/loop.go` | 新增 `estimateCost()`；用 `llm_call` 替换 `model_io`；测量 LLM 耗时；用 `totalInputTokens/totalOutputTokens/totalCostUsd` 替换 `cumulativeUsage` struct |
| `agent/internal/runner/worker.go` | 删除 `TraceStarted` 字段；清理遗留注释 |
| `agent/internal/runner/processor.go` | 删除 `if !request.TraceStarted` 备用分支 |
| `server/src/services/agent-log-reader.ts` | 将 `model_io` case 替换为 `llm_call`；`ExecutionStep` 增加 `model/durationMs/stopReason/costUsd` 字段 |
| `admin/src/api/types.ts` | `ExecutionStep.type` 中 `model_io` → `llm_call`；增加对应字段 |
| `admin/src/components/features/monitor/monitor-page.tsx` | 新增 `groupStepsByIteration()`、`IterationData`、`ToolPair` 接口；新增 `IterationGroup`、`ToolPairView`、`ThinkingView` 组件；`ExchangeCard` 改用 iteration 分组渲染 |

## 前端 Iteration 分组交互设计

```
▼ Iteration 1  sonnet-4-5  ⚡1034↑256↓  ⏱2.1s  2 工具
    Think: "I need to search for..."
    🔧 search_web  123ms ✅
       [Input / Result 可展开]
▼ Iteration 2  sonnet-4-5  ⚡2089↑128↓  ⏱1.3s
    Think: "Based on the results..."
    🔧 send_channel_message  45ms ✅
```

- 运行中：最新 iteration 自动展开
- 完成后且 ≤3 轮：所有 iteration 默认展开；>3 轮默认折叠
- tool_call + tool_result 通过 `toolCallId` 配对，Input/Result 分色块展示

## 考虑过的替代方案

- **保留 `model_io` 加 debug flag**：改为 slog.Debug 可用 LOG_LEVEL 控制，但 debug 日志默认也会写入文件，且 reader 侧还是要处理大行。最终决定直接替换为轻量事件。
- **前端只加 iteration 分隔线**：已有实现（旧版），但不支持独立折叠每轮，仍然噪声大。

## 影响

- Agent JSONL 日志体积大幅减小（每轮减少 1 个大 JSON 行）
- 新增 `costUsd` 字段在定价表覆盖的模型上生效，未知模型返回 0
- 前端不再渲染 `model_io` 类型 step，旧日志文件中的 `model_io` 行会被忽略（`agent-log-reader.ts` 无对应 case，直接 continue）
