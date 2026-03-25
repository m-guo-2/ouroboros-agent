## ADDED Requirements

### Requirement: 归档快照持久化
每次 `CompactContext` 执行压缩时，SHALL 将被归档的原始消息序列化为 JSON 存入 `context_compaction_archives` 表。

#### Scenario: 正常归档写入
- **WHEN** `CompactContext` 归档了 15 条消息
- **THEN** 系统将这 15 条消息的完整 `[]AgentMessage` JSON 写入 `context_compaction_archives`
- **THEN** 记录关联的 `compaction_id`（来自 `context_compactions` 表）、`session_id`、`message_count`

#### Scenario: 归档写入失败
- **WHEN** `context_compaction_archives` 写入失败
- **THEN** 压缩流程不中断（归档备份是 best-effort）
- **THEN** 日志记录写入失败信息

### Requirement: context_compaction_archives 表结构
`context_compaction_archives` 表 SHALL 包含以下字段：`id`（自增主键）、`session_id`（会话 ID）、`compaction_id`（关联的压缩记录 ID）、`archived_messages`（JSON 序列化的 `[]AgentMessage`）、`message_count`（归档消息数）、`created_at`（创建时间戳）。

#### Scenario: 表自动创建
- **WHEN** Agent 启动并执行数据库 migration
- **THEN** `context_compaction_archives` 表 SHALL 被自动创建（如不存在）

#### Scenario: 索引
- **WHEN** 表创建
- **THEN** `session_id` 字段 SHALL 有索引，用于按会话查询

### Requirement: SaveCompaction 返回压缩记录 ID
`storage.SaveCompaction` SHALL 返回新插入的 `context_compactions` 记录的 ID，供归档快照关联使用。

#### Scenario: 获取 compaction ID
- **WHEN** `SaveCompaction` 成功写入一条压缩记录
- **THEN** 返回该记录的自增 ID
- **THEN** 调用方使用此 ID 写入关联的 `context_compaction_archives` 记录
