## ADDED Requirements

### Requirement: Session facts 持久化存储

系统 SHALL 提供 `session_facts` 数据库表，用于存储 session 级别的事实记录。每条事实包含原文内容、分类标签和时间戳，以 session_id 为主维度索引。

#### Scenario: 保存单条事实

- **WHEN** agent 调用 `save_memory` 工具，传入 `facts: ["用户决定使用 SQLite"]`
- **THEN** 系统在 `session_facts` 表中插入一条记录，fact 字段为原文 `"用户决定使用 SQLite"`，category 为 `"general"`

#### Scenario: 保存多条带分类的事实

- **WHEN** agent 调用 `save_memory` 工具，传入 `facts: ["使用 Go 技术栈", "日志保留 30 天"], category: "decision"`
- **THEN** 系统插入两条记录，每条 category 均为 `"decision"`，fact 为对应原文

#### Scenario: 查询 session 的所有事实

- **WHEN** 系统需要加载 session 的事实记录
- **THEN** 按 `created_at` 正序返回该 session_id 下所有 facts

---

### Requirement: save_memory 工具注册

系统 SHALL 为 agent 注册 `save_memory` 内置工具。工具接受 `facts`（字符串数组）和可选 `category`（字符串）参数，将事实写入 `session_facts` 表。

#### Scenario: 正常保存

- **WHEN** agent 在对话中调用 `save_memory`，传入有效 facts
- **THEN** 所有 facts 写入数据库，返回 `{ saved: N }`

#### Scenario: 空 facts 数组

- **WHEN** agent 调用 `save_memory`，传入 `facts: []`
- **THEN** 返回 `{ saved: 0 }`，不写入任何记录

#### Scenario: 工具在 subagent 中可用

- **WHEN** subagent 被创建，继承父 session 的 sessionID
- **THEN** subagent 也能调用 `save_memory`，facts 写入同一 session 的 facts 表

---

### Requirement: Pre-compaction memory flush

系统 SHALL 在执行 `CompactContext` 之前触发一次静默的 memory flush，让 agent 有机会提取即将被归档的消息中的关键事实。

#### Scenario: 正常 flush 流程

- **WHEN** `ShouldCompact` 返回 true
- **THEN** 系统在调用 `CompactContext` 之前，使用 cheap model 执行一次 mini agent loop，prompt 中包含即将被归档的消息摘要，工具列表仅含 `save_memory`，最大迭代数为 3

#### Scenario: Flush 成功保存事实

- **WHEN** mini agent loop 中 agent 调用了 `save_memory`
- **THEN** facts 写入 `session_facts` 表，flush 的消息不追加到主 context

#### Scenario: Flush 失败降级

- **WHEN** flush 的 LLM 调用失败或超时
- **THEN** 跳过 flush，继续执行正常的 `CompactContext` 流程，不阻塞主流程

#### Scenario: Flush 判断无需保存

- **WHEN** mini agent loop 中 agent 判断没有需要保存的事实，回复文本（不调用工具）
- **THEN** flush 正常结束，不写入任何 facts，继续 compaction

#### Scenario: 每个 compaction 周期只 flush 一次

- **WHEN** 同一个 compaction 检查点触发 flush
- **THEN** flush 只执行一次，即使 compaction 在 flush 后仍需继续

---

### Requirement: Session 启动时 facts 加载

系统 SHALL 在 session 处理新消息时，从 `session_facts` 表加载该 session 的所有事实，注入到 context 中作为 agent 的记忆。

#### Scenario: 有已保存的 facts

- **WHEN** session 加载 history 后，`session_facts` 表中存在该 session 的记录
- **THEN** 构造 `[Session Memory]` 格式的消息，包含所有 facts（按时间正序，带分类标签），注入到 history messages 开头

#### Scenario: 无已保存的 facts

- **WHEN** `session_facts` 表中无该 session 的记录
- **THEN** 不注入任何消息，正常继续

#### Scenario: Facts 超过 token 预算

- **WHEN** facts 总文本长度超过 context window 的 10%
- **THEN** 保留最新的 facts（按 created_at 倒序截取），丢弃最早的，确保不超过预算

---

### Requirement: 硬截断保护 compaction 摘要

`truncateByFullTurns` SHALL 在截断时保护 `[Context Compact]` 摘要消息及其后续的 assistant 确认消息，确保 compaction 成果不被丢弃。

#### Scenario: 历史中包含 compaction 摘要

- **WHEN** history messages 的开头是 `[Context Compact]` 摘要消息 + assistant ack 消息，后面跟着超过 maxTurns 的用户轮次
- **THEN** 保留摘要消息对（2 条），对其后的消息执行正常的 turn 截断逻辑

#### Scenario: 历史中无 compaction 摘要

- **WHEN** history messages 不以 `[Context Compact]` 开头
- **THEN** 行为与当前完全一致，无变化

---

### Requirement: System prompt 记忆指令

System prompt SHALL 包含指令，引导 agent 在对话过程中主动使用 `save_memory` 工具保存关键事实。

#### Scenario: System prompt 包含记忆指令

- **WHEN** agent 的 system prompt 被构建
- **THEN** 包含指令文本，告知 agent：当对话中产生关键决策、结论、需求、技术细节时，应调用 `save_memory` 保存
