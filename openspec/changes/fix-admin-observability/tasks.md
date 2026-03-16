## 1. 数据库 Schema 改造

- [x] 1.1 `messages` 表改为自增 ID：修改 `db.go` 中 CREATE TABLE，`id` 改为 `INTEGER PRIMARY KEY AUTOINCREMENT`，添加 `channel_meta TEXT` 列
- [x] 1.2 `context_compactions` 表改为自增 ID：`id` 改为 `INTEGER PRIMARY KEY AUTOINCREMENT`
- [x] 1.3 `session_facts`、`user_memory_facts`、`user_channels`、`delayed_tasks` 表改为自增 ID
- [x] 1.4 `session_events.message_id` 类型从 TEXT 改为 INTEGER
- [x] 1.5 添加索引 `CREATE INDEX idx_messages_session ON messages(session_id)`（自增 ID 作为聚簇键后只需 session_id 索引）

## 2. 后端 Go 存储层适配

- [x] 2.1 `storage/types.go`：`MessageData.ID` 从 `string` 改为 `int64`；`CompactionData.ID`、`SessionFact` 等同步改
- [x] 2.2 `storage/messages.go`：`SaveMessage` 去掉 `newID()` 调用，改用 `last_insert_rowid()` 获取自增 ID；所有 SELECT 查询去掉 `channel_meta` 的 scan 错误处理（列已存在）
- [x] 2.3 `storage/messages.go`：`GetLatestSessionMessages` 和 `GetRecentMessagesBefore` 的分页从 `created_at < ?` 改为 `id < ?`，ORDER BY 改为 `id DESC`
- [x] 2.4 `storage/compactions.go`：`SaveCompaction` 适配自增 ID
- [x] 2.5 其他表（session_facts、user_memory_facts 等）的 Save 函数适配自增 ID

## 3. 后端 API 层适配

- [x] 3.1 `api/sessions.go`：`getSessionMessages` 的 `before` 参数语义从 timestamp 改为 message ID（int64），调用对应的 storage 函数
- [x] 3.2 `api/traces.go`：`completedTraceCache` 添加 maxSize=500 限制和 FIFO 淘汰逻辑
- [x] 3.3 `shared/logger/store_sqlite.go`：`ReadTraceEvents` 找到事件后最多再查一天然后停止；`ReadLLMIO` 和 `ListLLMIORefs` 同理

## 4. 前端 API 和类型修正

- [x] 4.1 `api/client.ts`：`fetchApi` 在 `!response.ok` 或 `success: false` 时 throw Error
- [x] 4.2 `api/types.ts`：所有时间戳字段改为 `number`；`MessageData.id` 改为 `number`；`CompactionData.id` 改为 `number`
- [x] 4.3 `api/sessions.ts`：`getMessages` 的 `before` 参数从 timestamp 改为 message ID

## 5. 前端 Hook 和分页逻辑适配

- [x] 5.1 `hooks/use-sessions.ts`：`MESSAGES_PAGE_SIZE` 改为 50；`getNextPageParam` 改为使用 `oldest.id` 作为游标
- [x] 5.2 `hooks/use-monitor.ts`：`getNextPageParam` 中 `new Date(last.updatedAt).getTime()` 简化为直接用 `last.updatedAt`（已是 number）
- [x] 5.3 逐个检查前端使用 `createdAt` 做 `new Date()` 的地方，确认类型改为 number 后无编译错误

## 6. 验证

- [x] 6.1 编译后端 Go 代码，确认无编译错误
- [x] 6.2 编译前端 TypeScript，确认无类型错误
- [ ] 6.3 启动应用，验证 Monitor 页面能正常显示 session 列表、消息、trace
