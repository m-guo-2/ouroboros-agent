## Context

Admin Monitor 是可观测的核心界面，当前全链路存在 11 个 bug 导致可用性严重下降。数据库层面 `messages` 表缺少 `channel_meta` 列且 ID 体系为随机 TEXT（不适合分页），前端 `fetchApi` 在 HTTP 错误时不抛异常导致 React Query 的 retry/error 机制完全失效，TypeScript 类型声明与后端 Go 的 `int64` 不匹配。用户确认数据库无历史数据负担，可以重建表结构。

涉及模块：`agent/internal/storage/`（schema + 查询）、`agent/internal/api/`（handler + trace 缓存）、`shared/logger/`（trace 查询）、`admin/src/`（前端全链路）。

## Goals / Non-Goals

**Goals:**
- 消除"消息不展示"和"空白页面"问题（修 channel_meta 缺列 + 错误处理）
- 消除"历史消息查询为空"（修分页游标 + 增大 page size）
- 改善查询性能（自增 ID + 索引 + trace 查询优化）
- 建立正确的类型契约（TS 类型与 Go 对齐）

**Non-Goals:**
- 不做 WebSocket 实时推送（当前手动刷新可接受，实时推送是独立 feature）
- 不改 `agent_sessions`、`users`、`models` 等语义化 ID 的表
- 不做前端 UI 重构（只修数据层和类型，不改组件结构）
- 不做全局日志系统重构（只优化 trace 查询路径）

## Decisions

### D1: `messages` 表 ID 改为 `INTEGER PRIMARY KEY AUTOINCREMENT`

**选择**：DROP 旧表 + CREATE 新表，ID 改为自增整数。

**备选**：
- A) 保持 TEXT ID，用 `(created_at, id)` 复合游标 → 查询复杂，API 需传两个参数，TEXT 比较性能差
- B) 加单独的 `seq` 自增列 → 冗余字段，两套 ID 维护成本高

**理由**：无历史负担，自增 ID 是最干净的方案。SQLite 的 `INTEGER PRIMARY KEY` 就是 ROWID，聚簇存储，天然有序，单个 `session_id` 索引即可覆盖所有查询。`SaveMessage` 改用 `RETURNING id` 获取生成的 ID。

### D2: 同步改造 `context_compactions`、`session_facts`、`user_memory_facts` 等纯内部表

**选择**：一并改为自增 ID。

**理由**：这些表 ID 纯内部使用，不被外部系统引用。统一改造避免后续逐个修。改动成本低（无历史数据）。

### D3: `fetchApi` 在非 2xx 时抛异常

**选择**：`fetchApi` 检测到 `!response.ok` 或 `success: false` 时 throw Error。

**备选**：
- A) 在每个 `queryFn` 里检查 `res.success` → 分散，容易遗漏
- B) 增加 interceptor 层 → 过度设计

**理由**：在 `fetchApi` 统一处理是最简洁的方案。React Query 要求 queryFn 在失败时 throw，这样 `retry`、`isError`、`error` 才能正常工作。修改一处，所有调用方自动受益。

### D4: TypeScript 类型修正策略

**选择**：直接修改 `api/types.ts` 中的类型声明，所有时间戳字段改为 `number`。

**理由**：后端 Go 的 `int64` JSON 序列化为 number，前端声明为 `string` 是错误。修正类型后 TypeScript 编译器会自动标出所有类型不兼容的调用点，逐个修复即可。

### D5: Trace 查询优化 — 提前退出

**选择**：`ReadTraceEvents` 在找到事件后继续检查下一天，如果连续一天无数据则停止扫描。

**备选**：
- A) 维护 trace→date 的索引表 → 额外维护成本
- B) 只查最近 N 天 → 已有 `Days` 参数但 `ReadTraceEvents` 没用

**理由**：trace 通常在一天内完成。找到数据后再查一天（覆盖跨午夜场景），之后停止。改动最小，效果显著。

### D6: Trace 缓存加 LRU 淘汰

**选择**：`completedTraceCache` 加 maxSize 限制（如 500），超出时淘汰最早插入的 entry。

**理由**：用简单的 map + slice 实现 FIFO 淘汰即可，不需要引入外部 LRU 库。500 个 trace 对象的内存开销可控。

### D7: 消息分页 page size 从 10 调至 50

**选择**：`MESSAGES_PAGE_SIZE` 改为 50。

**理由**：10 太小，一次完整交互（user + assistant + 中间 tool 消息）可能就超过 10 条。50 条覆盖大多数会话的最近几轮交互，减少"加载更多"的需要。

## Risks / Trade-offs

- **[DROP TABLE messages]** → 现有消息数据丢失。用户确认无历史负担，但需在 `reset.sh` 和文档中明确说明。迁移方式：schema 直接改 CREATE TABLE，旧数据库需手动 DROP 或用 reset 脚本
- **[fetchApi 抛异常]** → 所有未处理 error 的调用方会看到 unhandled rejection。React Query 会自动捕获并设置 `isError`，但自定义 `mutationFn` 需要确认有 `.catch()` 或 `onError` 回调。需逐个审查 mutation 调用
- **[page size 增大]** → 单次请求数据量变大。50 条消息的 JSON 体积约 50-100KB，对本地部署完全可接受
- **[Trace 提前退出]** → 极端情况下跨多天的 trace 可能丢失部分事件。通过"找到后再查一天"缓解
