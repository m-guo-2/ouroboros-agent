## Why

Admin Monitor 页面存在多个影响可用性的问题：消息不展示、空白页面、历史消息查询为空、整体速度慢。根因排查发现是前后端全链路上的多个 bug 叠加——数据库缺列导致查询失败、前端静默吞掉 API 错误、TypeScript 类型与后端不匹配、缺少数据库索引、分页设计缺陷等。需要一次性系统修复。

## What Changes

- **修复 `messages` 表缺失 `channel_meta` 列**：添加 ALTER TABLE 迁移，使所有消息的 SELECT/INSERT 正常工作
- **`messages` 表 ID 改为数据库自增**：`id` 从随机 TEXT 改为 `INTEGER PRIMARY KEY AUTOINCREMENT`，为分页提供天然有序游标；同步改造 `context_compactions`、`session_facts` 等纯内部表
- **添加复合数据库索引**：为 `messages(session_id)` 建立覆盖索引（自增 ID 作为聚簇键后只需 session_id 索引）
- **修复前端 API 错误处理**：`fetchApi` 在 HTTP 错误时抛异常而非静默返回，使 React Query 的 retry 和 error 机制生效
- **修正 TypeScript 类型声明**：所有 `createdAt`/`updatedAt`/`archivedBeforeTime` 从 `string` 改为 `number`，与后端 `int64` 对齐
- **修复分页游标**：基于自增 ID 的 `WHERE id < ?` 替代 `created_at < ?`，消除同时间戳跳过 bug
- **增大消息分页 page size**：从 10 调至 50，减少请求次数
- **Trace 查询性能优化**：`ReadTraceEvents` 找到数据后提前退出，避免扫描所有日期 DB
- **Trace 缓存加上限**：为 `completedTraceCache` 添加 LRU 淘汰或大小上限

## Capabilities

### New Capabilities

- `admin-monitor-reliability`: 覆盖 API 错误处理、类型安全、分页正确性等前端可靠性修复
- `messages-autoincrement-id`: 覆盖 messages 表 ID 体系从随机 TEXT 改为数据库自增的 schema 变更和全链路适配

### Modified Capabilities

（无已有 spec 需要修改）

## Impact

- **数据库 schema**：`messages` 表结构变更（id 类型、新增 channel_meta 列）；`context_compactions`、`session_facts` 等表同步改造。**BREAKING**：现有数据库需要重建 messages 表（无历史数据负担，可直接 DROP + CREATE）
- **后端 Go 代码**：`storage/db.go`（schema）、`storage/messages.go`（查询和 SaveMessage）、`api/sessions.go`（分页参数）、`api/traces.go`（缓存和查询优化）、`shared/logger/store_sqlite.go`（trace 查询优化）
- **前端 TypeScript**：`api/client.ts`（错误处理）、`api/types.ts`（类型修正）、`hooks/use-sessions.ts`（分页逻辑）、`hooks/use-monitor.ts`（分页逻辑）、`components/features/monitor/`（适配新数据结构）
- **API 契约**：`GET /api/agent-sessions/{id}/messages` 的分页参数从 `before`（timestamp）改为 `before`（message id），响应中 `createdAt` 类型不变（本来就是 number，只是 TS 声明错误）
