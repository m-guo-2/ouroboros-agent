## Why

当前上下文压缩（Context Compaction）机制在多轮长对话中导致 agent 丧失记忆。根因有三：

1. **加载时硬截断**：`truncateByFullTurns(historyMessages, 10)` 在 session 加载时无差别截断，连 compaction 生成的摘要消息都会被砍掉。
2. **摘要有损且嵌套退化**：200 词摘要本身信息密度低，且多次 compaction 后产生"摘要的摘要"，具体事实彻底消失。
3. **无持久化记忆层**：所有记忆完全依赖 session context 这个会被反复压缩的通道，没有独立的事实存储。

实际表现：昨晚讨论的内容，今天 agent 完全记不得。

## What Changes

- **新增 session facts 持久化存储**：agent 在对话中可随时将重要事实以原文形式写入数据库，独立于 session context 生命周期。
- **新增 compaction 前静默 flush 机制**：当 token 接近压缩阈值时，在真正 compact 之前插入一个静默 agentic turn，让 agent 提取即将被丢弃的上下文中的关键事实并持久化。
- **新增 session 启动时 facts 加载**：session 启动时从 facts 表加载本 session 的记忆事实，注入到 context 中，让 agent"记得"之前的内容。
- **修复硬截断 bug**：`truncateByFullTurns` 不再截断 `[Context Compact]` 摘要消息。

## Capabilities

### New Capabilities
- `memory-facts-store`: Session 级事实的持久化存储、agent 工具接口（save_memory / recall_memory）、compaction 前 flush 机制、session 启动时 facts 加载。

### Modified Capabilities

## Impact

- `agent/internal/storage/`：新增 `session_facts` 表及 CRUD
- `agent/internal/engine/ostools/`：新增 `save_memory` 工具，修改 `recall_context` 工具
- `agent/internal/runner/compact.go`：新增 pre-compaction flush 逻辑
- `agent/internal/runner/processor.go`：修复硬截断 bug，新增 session 启动时 facts 加载
- `agent/internal/storage/db.go`：新增 DDL migration
