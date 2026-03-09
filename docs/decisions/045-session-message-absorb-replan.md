# Session 消息吸纳与重新规划

- **日期**：2026-03-05
- **类型**：架构决策 / 代码变更
- **状态**：已实施

## 背景

Agent 处理用户消息期间，如果用户发了新消息（补充信息、取消指令等），新消息被排队后触发独立的 `processOneEvent`——完整重建上下文、初始化 LLM、注册工具、跑一轮新的 `RunAgentLoop`。LLM 无法在当前处理周期内感知新消息并重新规划，违背了 `final.md` 确立的"信息透传 + Agent 自决"设计哲学。

## 决策

在 `processOneEvent` 内增加 Execute-Checkpoint-Absorb 三阶段循环：每轮 `RunAgentLoop` 结束后，先做 checkpoint（水位检查 + compact + 保存上下文），再检查队列中是否有新消息。如果有，格式化后追加到当前上下文，直接重新跑 `RunAgentLoop`，让 Agent 在同一个处理周期内看到所有消息并自主决策。

## 变更内容

- `agent/internal/runner/worker.go`：新增 `MaxAbsorbRounds = 5` 常量和 `popAllPending()` 原子出队函数
- `agent/internal/runner/processor.go`：`processOneEvent` 重构为三阶段循环——Execute（RunAgentLoop + FinalText 回写 + WorkDir 更新）→ Checkpoint（EstimateTokens + ShouldCompact/CompactContext + UpdateSession）→ Absorb-or-Exit（轮次上限检查 + popAllPending + 消息格式化追加）

## 考虑过的替代方案

- **drainWorker 层批量合并**：在外层理解消息格式、处理 traceID，职责越界。不如在已持有 LLM client 和 tools 的 `processOneEvent` 内循环
- **只在最终退出时做 checkpoint**：多轮吸纳可能导致 context window 溢出或崩溃丢失全部进度。每轮 checkpoint 保证安全性和持久性

## 影响

- `drainWorker` 不需要修改，作为极端时序的兜底
- `RunAgentLoop`（engine 层）保持纯粹，不感知队列存在
- Dispatcher 和 Channel 层无变化
- `MaxAbsorbRounds = 5` 防止群聊刷屏导致无限循环
- FinalText 现在始终写入 messages，修正了原有的上下文完整性缺失
