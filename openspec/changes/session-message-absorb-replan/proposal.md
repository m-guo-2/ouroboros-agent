## Why

当前 session 处理流程中，每条入队消息都触发一次独立的 `processOneEvent`——完整重建上下文、初始化 LLM client、注册 tools、跑一轮 `RunAgentLoop`。当用户在 Agent 处理期间发了补充消息（"再加上日期"、"算了不用了"），该消息被排队后要等当前处理完全结束，再从头走一遍完整流程。这导致：

1. **割裂体验**：LLM 无法在当前轮次感知新消息并重新规划，而是把每条消息当独立请求处理
2. **资源浪费**：每次都完整重建上下文、重新初始化，而非复用当前已有的处理状态
3. **延迟叠加**：用户补充信息后要等两轮完整处理（当前轮 + 新轮），而非在当前轮尾部吸纳

参考 `final.md` 中模型三（信息透传 + Agent 自决）的设计哲学，Agent 应该在一次处理周期内看到所有已到达的消息并自主决策，而非由系统拆成多个独立的处理单元。

## What Changes

- **processOneEvent 增加外层吸纳循环**：`RunAgentLoop` 结束后，原子检查 `SessionWorker.Queue` 中是否有待处理消息；如果有，将它们格式化追加到当前上下文，再跑一轮 `RunAgentLoop`（re-plan），直到队列为空
- **FinalText 回写上下文**：在检查 pending 消息之前，把 `loopResult.FinalText` 作为 assistant message 追加到 messages 中，保证对话连续性
- **新增 `popAllPending` 辅助函数**：在 worker.go 中提供原子操作，一次性从 `SessionWorker.Queue` 取出所有待处理请求
- **drainWorker 无需修改**：它仍作为兜底循环，处理 `processOneEvent` 内循环未覆盖到的极端时序

## Capabilities

### New Capabilities
- `message-absorb-replan`: Session 内消息吸纳与重新规划机制——在一次 processOneEvent 生命周期内，LLM 执行结束后检查并吸纳新到达的消息，带入上下文重新规划

### Modified Capabilities

## Impact

- `agent/internal/runner/processor.go` — `processOneEvent` 增加外层 for 循环和 pending 消息吸纳逻辑
- `agent/internal/runner/worker.go` — 新增 `popAllPending` 函数；`drainWorker` 逻辑不变但行为语义微调
- `agent/internal/engine/loop.go` — 无修改，`RunAgentLoop` 保持纯粹
- `agent/internal/dispatcher/dispatcher.go` — 无修改
- 日志系统 — 新增"吸纳新消息，重新规划"的 business 级别日志
