## 1. 存储层——归档快照

- [x] 1.1 在 `storage/compactions.go` 中新增 `context_compaction_archives` 表的建表 DDL，在 `db.go` 的 migration 列表中注册
- [x] 1.2 修改 `SaveCompaction` 返回新插入记录的 ID（`int64`），更新所有调用方
- [x] 1.3 新增 `SaveCompactionArchive(compactionID int64, sessionID string, messages []types.AgentMessage)` 函数，将归档消息 JSON 写入 `context_compaction_archives`
- [x] 1.4 为上述新函数编写单元测试

## 2. 核心逻辑——保留策略重构

- [x] 2.1 在 `compact.go` 中新增常量 `recentUserTurns = 5` 和 `retainMaxRatio = 0.50`
- [x] 2.2 新增 `findRetainStart(messages []AgentMessage, recentTurns int, maxRatio float64) int` 函数：从后往前找到第 N 个用户 text turn 的起始位置，确保保留消息数不超过总数的 50%
- [x] 2.3 替换 `CompactContext` 中的 archiveEnd 计算逻辑：从"token 预算逐轮剥离"改为调用 `findRetainStart`
- [x] 2.4 为 `findRetainStart` 编写单元测试：覆盖正常、50% 上限、turn 不足 N 个三种场景

## 3. 核心逻辑——结构化摘要合约

- [x] 3.1 修改 `generateSummary` 的 system prompt 为中文："你是一个对话摘要助手，负责将长对话压缩成结构化的摘要"
- [x] 3.2 修改 user prompt 为 5 维度结构化模板：目标/项目、当前状态、关键决策、失败/回退、未完成事项
- [x] 3.3 保持 ≤200 词上限和 fallback 降级逻辑不变

## 4. 核心逻辑——任务状态提取

- [x] 4.1 新增 `ExtractTaskState(ctx, archived []AgentMessage, llmClient, model string) string` 函数
- [x] 4.2 实现 LLM prompt：中文 system prompt + 要求区分任务来源（user/agent）和完成状态
- [x] 4.3 提取失败返回空字符串，不阻断压缩流程
- [x] 4.4 为 `ExtractTaskState` 编写单元测试：有任务、无任务、LLM 失败三种场景

## 5. 核心逻辑——四层上下文组装

- [x] 5.1 新增 `buildSummaryLayer(archivedCount int, summary string) []AgentMessage` 函数：构造摘要层的 user + assistant ack 消息对，使用 `[历史上下文摘要]` 前缀
- [x] 5.2 新增 `buildTaskStateLayer(taskState string) []AgentMessage` 函数：构造任务状态层的 user + assistant ack 消息对，使用 `[任务状态]` 前缀；空 taskState 返回 nil
- [x] 5.3 新增 `buildAgentMemoryLayer(sessionID, model string) []AgentMessage` 函数：重新加载 session facts 并构造 user + assistant ack 消息对，使用 `[Agent Memory]` 前缀；复用 `prependSessionMemory` 的 token budget 逻辑
- [x] 5.4 重构 `CompactContext` 的组装阶段：按 `summaryLayer + retainedMessages + taskStateLayer + agentMemoryLayer` 顺序组装，各层独立退化

## 6. 归档快照集成

- [x] 6.1 在 `CompactContext` 中，`SaveCompaction` 成功后调用 `SaveCompactionArchive` 写入归档快照
- [x] 6.2 归档写入失败时仅记录 warn 日志，不影响压缩流程

## 7. Processor 集成

- [x] 7.1 修改 `processor.go` 中压缩后的消息处理：移除原有的 `prependSessionMemory` 对压缩后消息的重复注入（因为四层组装已包含 agent 记忆层）
- [x] 7.2 确认非压缩路径下 `prependSessionMemory` 仍正常工作

## 8. Subagent 适配

- [x] 8.1 修改 `subagent/manager.go` 中传给 `CompactContext` 的参数，确保使用主 session 的 `sessionID` 加载 session facts
- [x] 8.2 确认 subagent 路径不调用 `FlushMemoryBeforeCompact`（现有行为，验证不回退）

## 9. 集成测试

- [x] 9.1 编写 `compact_test.go` 集成测试：模拟完整压缩流程（20+ 条消息 → 触发压缩 → 验证四层结构、归档快照、任务提取、session facts 注入）
- [x] 9.2 编写退化测试：无 session facts、无任务、摘要 LLM 失败、任务提取失败
- [x] 9.3 编写保留策略测试：正常保留 5 轮、50% 上限、turn 不足 N 个
