## Context

当前 agent runner 的会话处理采用 per-session 串行队列模型：

- `SessionWorker` 持有一个 `Queue []QueuedRequest`，由 `workerMutex` 保护
- `drainWorker` 循环：pop 一条 → `processOneEvent` → pop 下一条
- `processOneEvent` 内部：加载配置 → 构建历史 → 注册工具 → `RunAgentLoop` → 保存上下文

每条排队消息都触发一次完整的 `processOneEvent`，包括重建 LLM client、重新注册 tools、重新加载历史。当用户在 Agent 推理期间发了新消息，该消息要等当前 `processOneEvent` 完全结束后才开始独立处理。

`final.md` 确立的设计哲学是"信息透传 + Agent 自决"——系统只负责把信息送到 Agent 面前，决策权交给 Agent。当前实现违背了这个原则：系统把连续的用户消息拆成了独立的处理单元，而非让 Agent 在一个上下文中完整看到。

**涉及的关键文件**：
- `agent/internal/runner/worker.go` — SessionWorker、drainWorker、EnqueueProcessRequest
- `agent/internal/runner/processor.go` — processOneEvent（约 260 行）
- `agent/internal/engine/loop.go` — RunAgentLoop（纯 LLM 执行循环，不改）

## Goals / Non-Goals

**Goals:**
- LLM 执行结束后，在同一个 `processOneEvent` 生命周期内检查并吸纳新到达的消息
- 吸纳后带入完整上下文重新规划（re-plan），让 Agent 看到新消息并自主决策
- 复用当前处理周期已初始化的 LLM client、tools、config，避免重复初始化
- 保持 `RunAgentLoop`（engine 层）的纯粹性——不感知队列存在

**Non-Goals:**
- 不修改 `RunAgentLoop` 内部逻辑（engine 层不感知消息队列）
- 不修改 dispatcher 层（消息入队流程不变）
- 不修改 channel 层（飞书/企微消息处理不变）
- 不做 LLM 执行过程中的实时中断（mid-loop interruption）——本次只在 loop 结束后检查
- 不引入意图分类路由层（`final.md` 中明确排除的方案）

## Decisions

### Decision 1: 在 processOneEvent 内增加外层 for 循环

**选择**：`processOneEvent` 在 `RunAgentLoop` 返回后，检查 `worker.Queue`，如果有 pending 消息则吸纳并继续循环。

**替代方案**：在 `drainWorker` 层面做批量合并——pop 多条，合并后传入 `processOneEvent`。

**选择理由**：
- `processOneEvent` 内已持有初始化好的 LLM client、tools、registry，循环内复用成本为零
- `drainWorker` 层合并需要在外层理解消息格式、处理 traceID 等细节，职责越界
- 外层 for 循环自然兼容 subagent 完成通知（也通过 EnqueueProcessRequest 入队）

### Decision 2: FinalText 必须先回写到 messages 再检查 pending

**选择**：`RunAgentLoop` 返回时，`loopResult.Messages` 不包含 `FinalText` 对应的 assistant message。在检查 pending 之前，需要手动追加。

**理由**：保证对话连续性。如果 LLM 回复了 "好的，我来处理" 然后用户说 "等等，改一下"，LLM 需要在上下文中看到自己之前的回复。

序列：`[history] → [user A] → [assistant tool_use] → [tool_result] → [assistant "好的..."] → [user B "等等..."]`

### Decision 3: popAllPending 原子操作

**选择**：新增 `popAllPending(worker *SessionWorker) []QueuedRequest`，在 `workerMutex` 保护下一次性取出所有待处理消息。

**理由**：
- 最小锁粒度：lock → copy → clear → unlock
- 一次性取出避免逐条 pop 的竞态
- 返回空 slice 表示无 pending，作为外层循环退出条件

### Decision 4: drainWorker 保持不变作为兜底

**选择**：不修改 `drainWorker` 的循环结构。

**理由**：
- `processOneEvent` 内循环已消化绝大多数 pending 消息
- 极端时序下（最后一次 `RunAgentLoop` 执行期间又来新消息），`drainWorker` 自然兜底
- 两层循环不冲突：内层循环消化快、外层循环兜底慢，语义清晰

### Decision 5: pending 消息无需再次 SaveMessage

**选择**：dispatcher 已将用户消息保存到 DB。`processOneEvent` 吸纳 pending 消息时，只需 `formatUserMessage` 追加到内存 messages，不重复写 DB。

**理由**：
- dispatcher.Dispatch 在 step 5 已调用 `storage.SaveMessage`
- 重复写入会导致 DB 中出现重复消息
- 内存 messages 与 DB messages 职责不同：内存是 LLM 上下文，DB 是审计日志

### Decision 6: 每轮 Checkpoint——水位检查 + context 保存

**选择**：每轮 `RunAgentLoop` 结束后，立即执行 Checkpoint：检查 token 水位，超过阈值则 compact，然后保存 context 到 session。

**替代方案**：只在最终退出时执行一次 compact 和 context 保存。

**放弃替代方案的原因**：
- **安全性**：每轮 RunAgentLoop 最多 25 个 iteration，产生大量 tool_use + tool_result。第 1 轮用了 60% context window，第 2 轮可能直接撞上限导致 API 报错。必须在进入下一轮前保证水位安全
- **持久性**：进程崩溃时，上次 Checkpoint 后的工作全部丢失。每轮保存意味着最多丢一轮
- **成本可控**：compact 只在超过水位时触发，不是每轮都做。多数轮次只是一次 `UpdateSession` 写入

**Checkpoint 流程**：
```
estimate = EstimateTokens(messages, model)
if ShouldCompact(estimate):
    messages = CompactContext(messages)   // 调 LLM 压缩
saveContextToSession(sessionID, messages) // 写 DB
```

### Decision 7: FinalText 始终写入 messages（不仅限于 absorb 场景）

**选择**：无论是否有 pending 消息，`RunAgentLoop` 返回后都将非空 `FinalText` 作为 assistant message 追加到 messages。

**替代方案**：只在有 pending 消息时追加（仅为 absorb 服务）。

**选择理由**：
- 这是一个上下文完整性的修正。当前代码即使在单轮处理中，保存到 `session.Context` 的内容也不包含 LLM 的最终文本回复，下次加载历史时缺失
- 统一处理比条件判断更简单、更不容易出错

## Risks / Trade-offs

**[无限循环]** → 消息持续涌入（群聊刷屏），processOneEvent 永远不退出。**Mitigation**：`maxAbsorbRounds = 5` 硬上限，超过后退出，剩余消息由 drainWorker 兜底。

**[compact 延迟]** → compact 调用 LLM（用廉价模型），每次约 1-3 秒。如果每轮都触发 compact，会增加总延迟。**Mitigation**：compact 是条件触发（仅超水位时），正常情况下 2-3 轮内不会触发。即使触发，用户体验优于 context window 溢出导致的错误。

**[traceID 归属]** → 被吸纳的消息有自己的 traceID，但在同一个 processOneEvent 中执行。**Mitigation**：吸纳时记录 pending 消息的原始 traceID 到日志中。LLM 执行使用第一条消息的 traceID 作为主 trace。

**[subagent 通知的吸纳]** → subagent 完成通知也通过 EnqueueProcessRequest 入队，会被吸纳。**Mitigation**：这正是期望行为——Agent 在同一上下文中看到 subagent 结果和新用户消息，统一决策。
