## Why

当前上下文压缩机制（Decision 031）解决了「从硬截断到 LLM 摘要」的基础问题，但在 Context Engineering 层面仍然简陋——压缩后的上下文结构是"摘要+尾部消息"的平铺拼接，缺乏对信息类型的区分和注意力位置的考量。核心命题是：**为模型每一步决策构造最小充分输入**，以正确的结构组织，放在注意力能有效覆盖的位置。

当前的具体问题：
1. **保留策略粗糙**：按 token 预算从后往前逐轮保留，不区分消息的信息密度和时效性
2. **上下文结构单一**：压缩后只有 `[summary_user, ack] + retained` 三段拼接，缺乏层次
3. **会话记忆断裂**：压缩后 session facts 不重新注入，模型丢失已提取的持久化事实
4. **摘要内容无约束**：摘要 prompt 只说"summarize"，没有结构化的信息维度要求，摘要质量不稳定
5. **任务状态丢失**：压缩时不提取未完成任务，压缩后 agent 可能忘记正在做什么
6. **归档不可审计**：被归档的原始消息散落在 messages 表中，没有显式的归档快照

## What Changes

- **重新设计保留策略**：从"token 预算逐轮剥离"改为"最近 N 个用户事件 + 后续消息"的固定锚点策略，上限为总消息数的 50%
- **四层上下文组装**：压缩后 context 明确分为四层——摘要层、近期消息层、任务状态层、agent 记忆层——按三级记忆架构组织
- **结构化摘要合约**：摘要 LLM prompt 改为 5 维度结构化模板（目标/项目、当前状态、关键决策、失败/回退、未完成事项），不再自由发挥
- **压缩时任务状态提取**：压缩前调用 LLM 从被归档消息中提取未完成任务，作为独立的上下文层注入
- **归档快照备份**：被归档的原始消息写入 `context_compaction_archives` 表，提供完整的归档审计能力
- **压缩后 agent 记忆重注入**：压缩完成后重新加载 session facts，作为独立的上下文层注入

**架构铺垫**（本次不实现工具，但设计上预留）：
- 三级记忆架构的分层定义：Level 1 用户画像、Level 2 任务状态、Level 3 Agent 记忆
- 用户画像定位为"认知信息"放在 message context，而非 system prompt 中的"行为指令"
- System Prompt 职责收窄：只包含 agent 身份、静态技能、安全边界

## Capabilities

### New Capabilities
- `context-assembly-pipeline`: 压缩后的四层上下文组装逻辑——摘要层（5 维度结构化）、近期消息层（原样保留）、任务状态层（LLM 提取的 todo）、agent 记忆层（重新加载的 session facts）
- `compaction-archive-backup`: 被归档消息的快照备份机制，写入专用表供审计和回溯
- `task-state-extraction`: 压缩前从被归档消息中提取未完成任务，作为独立上下文层注入压缩后的 context

### Modified Capabilities
- `subagent-context-compression`: 子 agent 的压缩流程需适配新的保留策略和四层组装结构

## Impact

- **agent/internal/runner/compact.go**：核心重构——保留策略、摘要 prompt（5 维度）、任务提取、四层组装
- **agent/internal/runner/processor.go**：压缩检查点的记忆重注入逻辑
- **agent/internal/storage/**：新增 `context_compaction_archives` 表和相关 CRUD
- **agent/internal/subagent/manager.go**：适配新的压缩流程
- **无破坏性变更**：`recall_context`、`save_memory`、`session_facts` 等现有接口不变
- **无新外部依赖**
