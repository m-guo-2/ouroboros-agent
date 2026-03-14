# 上下文压缩（Context Compaction）

- **日期**：2026-03-03
- **类型**：架构决策 / 代码变更
- **状态**：已实施

## 背景

当前 Agent 的上下文管理依赖 `truncateByFullTurns(messages, 10)` 进行硬截断——保留最近 10 个用户轮次，更早的直接丢弃。这存在两个根本问题：

1. **信息丢失不可逆**：被截断的对话内容彻底消失，Agent 无法在后续轮次中回忆关键决策或上下文。
2. **粒度粗糙**：固定 10 轮不区分模型窗口大小（Claude 200K vs 8K 小模型），也不感知实际 token 消耗。

随着多轮对话和工具密集型任务增多，需要一套 token 感知、可回溯、有摘要能力的压缩机制。

## 决策

### 触发时机

- **Lazy 策略**：在 `RunAgentLoop` 结束后、保存 `session.context` 之前检查 token 量。
- **触发阈值**：预估 token > 模型上下文窗口的 60%。
- **压缩目标**：降到窗口的 50% 以下，为下一轮对话留出足够空间。
- 不使用凌晨批量压缩，避免引入 cron 基础设施和并发复杂度。

### Token 预估

分层策略，覆盖 80% 快速路径：

1. **快速预检**：`len(json(messages)) / 4`，偏差 ±20%。远低于阈值直接跳过，远高于直接触发。
2. **精确计算**：模糊区间（50%~70%）时使用 `tiktoken-go/tokenizer` 本地计算。支持 `cl100k_base`（Claude 近似）和 `o200k_base`（GPT-4o）。
3. 不使用 Anthropic Count Tokens API（需网络调用 + RPM 限制，不适合热路径）。

### 压缩策略

1. **自适应 N**：从前往后逐轮剥离完整 turn，直到 token 量降到目标阈值以下。不再固定保留 N 轮。
2. **LLM 摘要**：对被归档的旧消息使用便宜模型（如 Haiku / GPT-4o-mini）生成 ≤200 字的结构化摘要。摘要模型通过 `compact_model` 配置。
3. **长 tool_result 截断**：保留在活跃上下文中但超过 1KB 的 tool_result 截断为前 1KB + 提示文本。完整内容可通过 `recall_context` 从 messages 表取回。
4. **双向孤儿检测**：
   - 孤儿 tool_result（无对应 tool_use）→ 移除
   - 孤儿 tool_use（对应 tool_result 被归档）→ 将 tool_use + tool_result 一起保留或一起归档，保证 turn 完整性
5. **Summary 消息格式**：`role=user`，以 `[Context Compact]` 为前缀，紧跟一条 assistant 确认消息维持交替。

### 归档存储

- **messages 表天然作为归档**：`toPersistableMessages` 已将每条消息（含完整 tool_result）写入 messages 表。压缩只修改 `session.context`，messages 表不动。
- **新增 `context_compactions` 元数据表**：记录每次压缩的摘要、归档边界时间、压缩前后 token 量、使用的摘要模型。
- **`recall_context` 工具**：Agent 和 Subagent 均可调用，从 messages 表检索被压缩的历史消息。

### 降级策略

- 摘要 LLM 调用失败 → 使用 `[Earlier context archived, details available via recall_context]` 占位文本替代摘要。
- 归档元数据写入失败 → 放弃本次压缩，回退到 `truncateByFullTurns` 硬截断兜底。
- 保底约束：任何情况下至少保留最后 1 个完整 turn。

## 变更内容

### 新增文件

- `agent/internal/runner/tokencount.go`：Token 估算器（字符快估 + tiktoken 精确计算 + 模型窗口大小映射）
- `agent/internal/runner/compact.go`：压缩核心逻辑（`CompactContext`、turn 边界识别、tool_result 截断、LLM 摘要生成、孤儿检测）
- `agent/internal/storage/compactions.go`：`context_compactions` 表 CRUD
- `agent/internal/engine/ostools/recall.go`：`recall_context` 工具实现

### 修改文件

- `agent/internal/storage/db.go`：新增 `context_compactions` 建表 DDL + migration
- `agent/internal/storage/messages.go`：新增 `GetMessagesBefore(sessionID, beforeTime, limit)` 和 `SearchMessages(sessionID, query, beforeTime)` 查询方法
- `agent/internal/runner/processor.go`：在 `processOneEvent` 的 Loop 结束后、UpdateSession 之前插入压缩检查点
- `agent/internal/subagent/manager.go`：subagent 启动时注册 `recall_context` 工具
- `agent/go.mod`：新增 `tiktoken-go/tokenizer` 依赖

### 数据流

```
RunAgentLoop 完成
  → loopResult.Messages (完整上下文)
  → quickEstimateTokens(messages)
  → 超过 60% 阈值?
    ├─ No → 直接 UpdateSession(context: messages)
    └─ Yes → CompactContext(messages, modelContextWindow)
              ├─ 1. 识别 turn 边界，逐轮剥离直到 < 50%
              ├─ 2. 截断保留区的大 tool_result (>1KB)
              ├─ 3. 双向孤儿检测与修复
              ├─ 4. LLM 摘要生成 (便宜模型)
              ├─ 5. 组装: [summary_user, summary_ack] + retained_messages
              ├─ 6. 写入 context_compactions 元数据
              └─ 7. UpdateSession(context: compacted_messages)
```

## 考虑过的替代方案

1. **凌晨 2-6 点批量压缩**：需要 cron 基础设施、处理并发写入、处理用户在线场景。复杂度远超收益，未采用。
2. **固定 N=5 保留策略**：不感知 token 量和模型窗口差异，对大窗口模型浪费空间，对小窗口模型可能不够。未采用。
3. **纯截断无摘要**：信息完全丢失，Agent 在后续轮次中表现会断崖式下降。未采用。
4. **使用主 Agent 同款模型做摘要**：成本高 10-20 倍，且在同步路径上增加 1-3 秒延迟。摘要是信息提取任务，便宜模型即可胜任。未采用。
5. **独立文件存储归档内容**：messages 表已天然包含完整历史，额外文件存储是冗余。未采用。

## 影响

- Agent 对长对话的上下文保持能力显著提升：旧信息以摘要形式保留，按需可通过 `recall_context` 取回完整内容。
- Subagent 继承父 session 的压缩上下文 + recall 能力，不会因父 context 压缩而丢失关键信息。
- 新增 `tiktoken-go/tokenizer` 外部依赖（~4MB 内嵌词表，纯 Go，无 CGO）。
- `context_compactions` 表提供压缩审计能力，可追溯每次压缩的 token 变化。
