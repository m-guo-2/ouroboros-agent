## Context

当前 agent 的上下文管理依赖单一通道：`session.context`（JSON 序列化的 messages 数组）。Compaction 机制在 token 超过 60% 窗口时触发，通过 LLM 摘要 + 消息裁剪来缩减 context。但存在三个硬伤：

1. **加载时硬截断**：`processor.go` 的 `truncateByFullTurns(historyMessages, 10)` 在 session 加载时无条件截断到 10 轮，连 `[Context Compact]` 摘要消息都会被丢弃。
2. **摘要质量差且嵌套退化**：`buildMessagesDigest` 将输入截断到 8000 字符后生成 200 词摘要，多次 compaction 后具体事实全部丢失。
3. **没有独立记忆层**：context window 既是工作台又是仓库，压缩即丢失。

现有基础设施：
- `context_compactions` 表记录压缩元数据和摘要
- `recall_context` 工具可从 messages 表检索归档消息（被动，agent 不知道该搜什么）
- `user_memory` / `user_memory_facts` 表存在但未被 Go agent 使用，且是 user 维度而非 session 维度

## Goals / Non-Goals

**Goals:**
- Agent 在长对话中不丢失关键事实（决策、结论、技术细节、用户指令）
- 事实以原文形式持久化到数据库，不经过摘要压缩
- Compaction 前 agent 有机会主动提取和保存事实
- Session 启动时自动加载已保存的事实，agent 无需额外操作即可"记得"
- 修复硬截断 bug，保护 compaction 摘要消息不被丢弃

**Non-Goals:**
- 跨 session 记忆（不做。记忆属于 session 的主观认知，不做 user 维度聚合）
- 向量搜索 / 语义搜索（不做。简单 SQL 查询即可）
- Markdown 文件记忆（不做。用数据库）
- 记忆去重、合并、冲突解决（不做。追加即可，由 agent 在写入时自行判断）

## Decisions

### 1. 存储：新增 `session_facts` 表

```sql
CREATE TABLE IF NOT EXISTS session_facts (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    fact TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'general',
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_session_facts_session ON session_facts(session_id);
```

字段说明：
- `fact`：原文事实，不做压缩或改写。由 agent 自行提取，长度不限。
- `category`：分类标签，辅助加载时的排序展示。预设值：`decision`（决策）、`requirement`（用户需求）、`context`（技术背景）、`action`（行动项）、`general`（通用）。

不复用现有 `user_memory_facts` 表——那个表是 user 维度的，语义和索引方向都不对。session_facts 是全新的 session 维度表。

**考虑过的替代方案**：复用 `user_memory_facts` 并加 session_id 字段。但该表的设计语义是用户画像（跨 session 的持久知识），与 session 级主观记忆不同。混用会导致查询和清理逻辑纠缠。

### 2. Agent 工具：`save_memory`

注册为内置工具，定义：

```
save_memory:
  input:
    facts: string[]     # 一次可保存多条事实
    category?: string   # 默认 "general"
  output:
    saved: number       # 成功保存的条数
```

工具由 agent 在对话过程中随时调用。System prompt 中增加指令：当讨论中产生关键决策、结论、需求时主动保存。

不增加 `recall_memory` 工具——facts 在 session 启动时已注入 context，agent 直接可见。如果将来 facts 量很大（数百条），再考虑分页或搜索。

**考虑过的替代方案**：自动提取（后处理每条消息自动抽取事实）。但 agent 自己判断什么重要远比自动提取准确，且自动提取会引入额外 LLM 调用和延迟。

### 3. Pre-compaction Memory Flush（压缩前静默 flush）

在 `CompactContext` 执行之前，插入一个静默 agentic turn：

```
流程：
  ShouldCompact(estimate) == true
    → 1. 构造 flush 消息（system + user），附上即将被归档的消息摘要
    → 2. 执行一次 mini agent loop（最多 3 轮迭代，只注册 save_memory 工具）
    → 3. flush 产生的消息不追加到主 context（静默，用户不可见）
    → 4. 继续正常 CompactContext 流程
```

Flush 的 user prompt：
```
上下文即将被压缩，以下较早的对话内容将被归档。请提取其中的关键事实（决策、结论、需求、技术细节）并调用 save_memory 保存。如果没有需要保存的内容，直接回复 NO_SAVE。
```

Flush 使用与 compaction 相同的 cheap model（Haiku / GPT-4o-mini），控制成本。

Flush 失败（LLM 调用报错、超时）不阻塞主流程——继续正常 compaction，降级到当前行为。

**考虑过的替代方案**：不做 flush，完全依赖 agent 在对话中主动 save_memory。问题是 agent 经常忘记保存，或者在紧凑的工具调用流程中没有保存的时机。Flush 是安全网。

### 4. Session 启动时 Facts 加载

在 `processOneEvent` 中，加载 history 之后、构造 messages 之前：

```
流程：
  1. 从 session_facts 表查询 session_id 的所有 facts，按 created_at 正序
  2. 如果 facts 非空，构造一条 [Session Memory] 格式的消息注入到 context 开头
  3. 注入位置：在 history messages 之前，确保 agent 最先看到
```

注入格式：
```
[Session Memory]
以下是本次对话中保存的关键事实，请作为上下文参考：

[decision] 用户决定用 SQLite 作为日志存储...
[context] 当前项目使用 Go + SQLite 技术栈...
[requirement] 日志保留 30 天，每天自动轮转...
```

Token 预算控制：facts 注入的 token 量纳入 `EstimateTokens` 计算。如果 facts 本身已经占用了大量 token（极端场景），按时间正序截断，保留最新的 facts。初始上限设为 facts 总量不超过 context window 的 10%。

### 5. 修复硬截断 bug

修改 `truncateByFullTurns`，使其跳过以 `[Context Compact]` 开头的消息——这些消息是 compaction 生成的摘要，丢弃它们等于丢弃之前所有 compaction 的成果。

修改逻辑：
```go
func truncateByFullTurns(messages []types.AgentMessage, maxTurns int) []types.AgentMessage {
    // 找到第一条非 [Context Compact] 消息的位置
    // 保护 [Context Compact] + ack 对不被截断
    // 对剩余消息执行原有的 turn 截断逻辑
}
```

## Risks / Trade-offs

- **[Flush 增加延迟]** → 每次 compaction 前多一轮 LLM 调用（cheap model，~500ms）。可接受：compaction 本身已经有 LLM 摘要调用，flush 相当于将部分摘要成本转为 facts 提取。且 flush 只在触发 compaction 时执行，不影响正常对话延迟。

- **[Facts 持续增长]** → 长 session 可能积累数百条 facts。初期不做清理，通过 token 预算（10% context window）在加载时截断。如果将来成为问题，可以加 `consolidate_memory` 工具让 agent 合并旧 facts。

- **[Agent 不调用 save_memory]** → System prompt 指令不能保证 agent 100% 执行。Flush 机制是安全网：即使 agent 在对话中没有主动保存，compaction 前的 flush 仍会兜底提取。

- **[Flush 消息对 agent 不可见]** → Flush 是静默的 mini loop，产生的消息不追加到主 context。这意味着 agent 在 flush 后的主 context 中看不到 flush 过程。但这没关系：facts 已经写入数据库，下次加载时会注入。
