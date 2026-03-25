## Context

当前上下文压缩（Decision 031, `compact.go`）的核心流程：
1. `RunAgentLoop` 完成后，`EstimateTokens` 计算 token 占比
2. 超过 60% 触发 → `FlushMemoryBeforeCompact` 提取记忆 → `CompactContext` 压缩
3. `CompactContext` 以 token 预算为目标，从前往后逐轮剥离 turn，直到剩余消息 token 量 < 50% 窗口
4. 对被归档消息调用便宜模型生成 ≤200 词摘要
5. 组装 `[summary_user, ack] + retained` 替换 `session.context`

问题不止在压缩算法本身，而在于缺乏一个清晰的**信息合约**——压缩后的 context 必须包含什么信息、以什么结构、agent 才能无损地继续工作。这导致摘要内容随机、任务状态丢失、持久化知识断裂。

已有基础设施可复用：
- `session_facts` 表 + `save_memory` / `GetSessionFacts` 已就绪
- `context_compactions` 元数据表已就绪
- `recall_context` 工具已就绪
- `FlushMemoryBeforeCompact` 的记忆提取流程保持不变
- `PreciseEstimateTokens` / `QuickEstimateTokens` 的分层估算保持不变

## Goals / Non-Goals

**Goals:**
- 定义压缩后 context 的**信息合约**：什么信息必须存在、以什么结构组织
- 将压缩后上下文结构化为四层：摘要层、近期消息层、任务状态层、agent 记忆层
- 用"最近 N 个用户事件"锚点替代"token 预算逐轮剥离"的保留策略
- 定义结构化摘要合约（5 个维度），替代当前的自由摘要
- 压缩时提取未完成任务，保证 agent 不因压缩忘记正在做什么
- 压缩后重新注入 session facts，保证持久化知识不因压缩断裂
- 新增归档快照备份，为被归档消息提供专用的审计和回溯能力
- 保持 subagent 压缩流程与主 agent 的一致性

**Non-Goals:**
- 不实现用户画像的存储和工具（`save_user_profile`）——属于后续"三级记忆"change
- 不实现动态 skill 学习工具（`save_skill`）——属于后续"三级记忆"change
- 不实现结构化 todo 管理工具（`save_todo` / `update_todo`）——本次用 LLM 提取兜底
- 不修改 `recall_context` 工具的接口和行为
- 不修改 token 估算的分层策略（quick + precise）
- 不修改 `FlushMemoryBeforeCompact` 的触发逻辑

## Decisions

### Decision 1: 三级记忆架构（设计框架）

本次不实现完整的三级记忆工具链，但在压缩管道和上下文组装中按此架构设计扩展点。

```
Level 1: 用户画像 (User Profile)
  ─ 偏好、风格、约束（建议性的，非指令）
  ─ 存储键: user_id（跨会话）
  ─ 本次: 不实现，依赖 session_facts 中的偏好类条目兜底

Level 2: 任务状态 (Task State)
  ─ 当前目标、todo 列表、任务来源
  ─ 存储键: session_id
  ─ 本次: 压缩时 LLM 提取，以文本形式注入 context

Level 3: Agent 记忆 (Agent Memory)
  ─ 事实性知识 (session_facts)、学习到的技能、主动任务
  ─ 存储键: agent_id (skill) / session_id (facts)
  ─ 本次: 重新加载 session_facts；动态 skill 不实现
```

**用户画像是认知信息，不是行为指令**：
Agent 作为独立角色，有自己的判断。用户偏好（如"用中文回复"、"代码要简洁"）应该是 agent 了解用户的信息，不是必须服从的规则。agent 可以基于场景判断是否遵循偏好——例如用户说"不引入新依赖"，但最优解需要引入时，agent 应该主动提出建议。

群聊场景进一步印证了这一点：多人偏好可能矛盾（A 喜欢详细解释、B 喜欢简洁回复），如果是指令就无法合并；如果是认知信息，agent 可以根据当前对话者自行调整。

**因此用户画像放 message context，不放 system prompt。**

### Decision 2: System Prompt 职责收窄

System Prompt 只放三类内容：
1. Agent 核心身份和行为准则
2. 静态技能（skillsSnippet）
3. 安全边界（不可违反的硬约束）

用户画像、任务状态、agent 记忆——全部放 message context。理由：
- System prompt 内容被模型视为"指令级权威"，而记忆类信息应该是"认知级参考"
- System prompt 不参与压缩，不应膨胀
- 记忆信息需要动态更新，system prompt 的静态特性不适合

当前 `BuildSystemPrompt(agentSystemPrompt, skillsSnippet)` 保持不变，不新增参数。

### Decision 3: 保留策略——"最近 N 个用户事件"锚点

**选择**：保留最近 5 个 user text turn（非 tool_result 的 user 消息）及其之后的所有 assistant/tool 消息。保留消息数上限为总消息数的 50%。

**为什么 5 个**：
- 对话的信息密度呈指数衰减，最近 5 轮覆盖绝大多数活跃上下文
- 固定锚点比 token 预算更可预测——开发者和使用者能准确预期"最近 5 轮一定在"
- 50% 总量上限防止短对话的 5 轮占比过高

**替代方案**：
- 保持当前"token 预算逐轮剥离"：token 精确但不可预测，且不同消息长度导致保留轮数差异大
- 百分比保留（保留后 50% 消息）：不区分 turn 边界，可能切断 tool_use/tool_result 对

### Decision 4: 结构化摘要合约（5 维度）

摘要不再自由发挥，必须覆盖以下 5 个维度：

| 维度 | 回答的问题 | 示例 |
|---|---|---|
| 目标/项目 | 用户在做什么？ | "重构上下文压缩机制" |
| 当前状态 | 做到哪一步了？ | "已完成存储层，正在改 compact.go" |
| 关键决策 | 选了什么方案、为什么？ | "选了固定 5 轮锚点，因为更可预测" |
| 失败/回退 | 什么试过不行？ | "试过 loop 内插入压缩检查点，会打断推理链" |
| 未完成事项 | 归档区有什么没做完的？ | "subagent 适配还没开始" |

注意：**不包含"约束/偏好"维度**——那属于用户画像（Level 1），不属于摘要。摘要只负责回答"这段被压缩的对话里发生了什么"。

摘要 LLM prompt 使用中文 system prompt，user prompt 要求按 5 维度逐项输出。≤200 词上限。

### Decision 5: 压缩时任务状态提取

**选择**：路径 A——压缩前让 LLM 从被归档消息中提取未完成任务，以结构化文本形式注入压缩后 context。

类似 `FlushMemoryBeforeCompact` 的模式——在同一个压缩窗口期内，额外跑一次 LLM 调用，但目标不同：
- `FlushMemoryBeforeCompact`：提取事实 → 写入 session_facts
- `ExtractTaskState`：提取未完成任务 → 返回文本 → 注入 context

提取结果格式：
```
[任务状态]
当前目标: <用户在做什么>

未完成任务:
- [ ] <task 1> (来源: user/agent)
- [x] <task 2> (已完成)
- [ ] <task 3> (来源: user/agent)
```

**为什么不用结构化 todo 工具（路径 B）**：需要 agent 在运行时主动管理 todo，增加工具复杂度和 system prompt 负担。LLM 提取作为第一步足够用，等积累了数据再考虑结构化工具。

**为什么区分来源**：用户明确要求的任务（user）和 agent 自己推断的任务（agent）权重不同——用户任务不可省略，agent 任务在 token 紧张时可降级。

### Decision 6: 四层上下文组装

压缩后 `context.messages` 替换为：

```
[摘要消息 + assistant ack]       ← 第一层：5 维度结构化历史摘要
[保留的近期消息]                 ← 第二层：原样保留的近期对话
[任务状态消息 + assistant ack]   ← 第三层：LLM 提取的 todo（Level 2）
[agent 记忆消息 + assistant ack] ← 第四层：重新加载的 session facts（Level 3）
```

**注意力位置设计**：
- 开头（primacy）：摘要——提供宏观框架，让模型先建立全局理解
- 中间：近期消息——保持时序连贯的完整对话
- 末尾（recency）：任务状态 + agent 记忆——模型对末尾最敏感，任务和事实放在这里确保被有效关注

**各层可独立退化**：
- 无 session facts → 第四层不注入
- 任务提取失败或无任务 → 第三层不注入
- 摘要 LLM 失败 → 第一层使用 fallback 摘要

**未来扩展点**：
- Level 1（用户画像）实现后，作为第五层追加到末尾（最稳定的信息，recency 位置持续提醒）
- Level 3 的动态 skill 实现后，与 session facts 合并到第四层
- 群聊场景下，用户画像按参与者分组呈现，以认知口吻（"以下是我对参与者的了解，供参考"）而非指令口吻

### Decision 7: 归档快照备份

**选择**：新增 `context_compaction_archives` 表，每次压缩时将被归档的原始消息序列化为 JSON 存入。

**表结构**：
```sql
CREATE TABLE IF NOT EXISTS context_compaction_archives (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    compaction_id INTEGER NOT NULL,
    archived_messages TEXT NOT NULL,  -- JSON 序列化的 []AgentMessage
    message_count INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);
```

**替代方案**：
- 不存归档（只依赖 messages 表）：messages 表的消息格式经过 `toPersistableMessages` 转换，难以还原压缩时的完整 context
- 存到文件系统：增加部署复杂度，SQLite 已足够

### Decision 8: 压缩检查点保持不变

保持当前的单一检查点——`RunAgentLoop` 完成后、`UpdateSessionContextAndCursor` 之前。

不在 loop 内部工具执行后插入额外检查点，原因：
- 当前 `RunAgentLoop` 每轮就是"工具执行完成后"，loop 退出时覆盖了两个检查点场景
- loop 内部插入压缩会中断 agent 的连续推理链
- `MaxIterations=25` 足以覆盖单轮需求，极端情况由 subagent 的 re-entry 机制兜底

## Risks / Trade-offs

**[Risk] 固定 5 轮可能不适配所有场景** → 将 `recentUserTurns` 提取为常量（默认 5），未来可配置化。短对话（<10 条消息）不触发压缩（ratio < 0.60），不受影响。

**[Risk] 任务提取 LLM 调用质量不稳定** → 提取失败时该层不注入，不影响压缩主流程。提取结果仅作为辅助上下文，不作为系统的 source of truth。后续可升级为结构化 todo 工具（路径 B）。

**[Risk] 归档快照表增加存储** → 单次归档通常是 10-50 条消息的 JSON，<100KB。SQLite 单库下性能无影响。长期可考虑 TTL 清理（非本次范围）。

**[Risk] 压缩时增加两次 LLM 调用（摘要 + 任务提取）** → 两次调用都使用便宜模型（Haiku/gpt-4o-mini），单次 <1s。`FlushMemoryBeforeCompact` 本身已是一次 LLM 调用，总共 3 次可接受。可合并摘要和任务提取为一次调用（同一个 prompt 输出两个部分），减少为 2 次——作为优化可后续实施。

**[Trade-off] 压缩后 context 比当前略大**：新增了任务状态层和 agent 记忆层。但各层都有 token budget 控制，总量可控。

**[Trade-off] 三级记忆本次只实现骨架**：Level 1 和 Level 3 的完整工具链（`save_user_profile`、`save_skill`、`save_todo`）留给后续 change。本次用 LLM 提取和 session_facts 兜底，保证架构到位但不过度实现。
