## 1. Worker 层：popAllPending 原子操作

- [ ] 1.1 在 `agent/internal/runner/worker.go` 中新增 `popAllPending(worker *SessionWorker) []QueuedRequest` 函数：`workerMutex.Lock` → copy queue → clear queue → unlock → return
- [ ] 1.2 新增 `maxAbsorbRounds` 常量（默认值 5），放在 `worker.go` 顶部与 `SessionIdleTimeoutMs` 同级

## 2. Processor 层：Execute-Checkpoint-Absorb 循环

- [ ] 2.1 在 `processOneEvent` 中，将从 `RunAgentLoop` 调用到函数末尾的逻辑重构为 `for absorbRound := 0; ; absorbRound++` 外层循环，分为三个阶段：Execute → Checkpoint → Absorb-or-Exit
- [ ] 2.2 **Execute 阶段**：`RunAgentLoop` 返回后，将非空 `loopResult.FinalText` 作为 `{role: "assistant", content: [{type: "text", text: FinalText}]}` 无条件追加到 messages；更新 WorkDir
- [ ] 2.3 **Checkpoint 阶段**：每轮执行 `EstimateTokens` → 如果 `ShouldCompact` 为 true 则调用 `CompactContext` 并替换 messages → 调用 `UpdateSession(context, workDir)` 保存当前上下文快照
- [ ] 2.4 **Absorb-or-Exit 阶段**：检查 `absorbRound >= maxAbsorbRounds` 则 warn 日志 + break；调用 `popAllPending(worker)`，返回 nil 则 break；否则对每条 pending 消息调用 `formatUserMessage` 格式化后追加到 messages
- [ ] 2.5 添加 business 级别日志："发现新消息，重新规划"，包含 `absorbedCount` 和 `absorbRound` 字段
- [ ] 2.6 确认 `OnNewMessages` callback 在 re-plan 轮次中仍然生效（复用同一个 `registry` 和 callback 闭包）

## 3. 验证

- [ ] 3.1 编译通过：`cd agent && go build ./...`
- [ ] 3.2 手动场景验证：启动 agent，发送消息 A，在处理期间发送消息 B，确认日志中出现"发现新消息，重新规划"且 B 被吸纳到同一轮处理
- [ ] 3.3 边界场景验证：连续快速发送 6+ 条消息，确认 absorb 在 5 轮后退出，剩余消息由 drainWorker 兜底
