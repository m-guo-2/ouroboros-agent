## 1. 存储层

- [x] 1.1 在 `storage/db.go` 的 migrations 中新增 `session_facts` 表 DDL 和索引
- [x] 1.2 新增 `storage/session_facts.go`：`SaveSessionFacts(sessionID string, facts []string, category string) (int, error)` 和 `GetSessionFacts(sessionID string) ([]SessionFact, error)` 方法

## 2. Agent 工具

- [x] 2.1 新增 `engine/ostools/memory.go`：实现 `save_memory` 工具定义和执行器，调用 `storage.SaveSessionFacts`
- [x] 2.2 在 `processor.go` 的工具注册流程中注册 `save_memory`（主 agent）
- [x] 2.3 在 `subagent/manager.go` 中为 subagent 注入 `save_memory` 工具

## 3. Pre-compaction Memory Flush

- [x] 3.1 在 `runner/compact.go` 中新增 `FlushMemoryBeforeCompact` 函数：构造 flush prompt，执行 mini agent loop（max 3 轮，只注册 save_memory），失败时静默降级
- [x] 3.2 在 `processor.go` 的 Checkpoint 阶段，`ShouldCompact` 为 true 后、`CompactContext` 之前调用 `FlushMemoryBeforeCompact`

## 4. Session 启动时 Facts 加载

- [x] 4.1 在 `processor.go` 的 `processOneEvent` 中，加载 history 之后，调用 `GetSessionFacts` 获取本 session 的 facts
- [x] 4.2 如果 facts 非空，构造 `[Session Memory]` 格式消息，注入到 messages 开头（在 history 之前）
- [x] 4.3 实现 token 预算控制：facts 总量不超过 context window 的 10%，超出时按时间正序截断保留最新

## 5. 修复硬截断 bug

- [x] 5.1 修改 `truncateByFullTurns`：检测 messages 开头的 `[Context Compact]` 摘要消息对，截断时保留它们不动

## 6. System Prompt 记忆指令

- [x] 6.1 在 system prompt 构建逻辑中追加记忆指令段落，引导 agent 主动使用 `save_memory` 保存关键事实
