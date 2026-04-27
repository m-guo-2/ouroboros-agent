## Why

当前 `agent/data/config.db` 与 `channel-qiwei/qiwei.db` 是单机 SQLite，除常见的"单写锁 + 无跨机共享"瓶颈外，schema 本身也积攒了显著债务：

1. 历史用 "`ALTER TABLE` 数组 + 吞错误" 做增量迁移，MySQL 8.0 下每次启动会刷 duplicate-column 错误。
2. 大量 JSON 列（`messages.tool_calls / attachments_json`、`agent_configs.skills / hooks / subagent_*`、`context_compaction_archives.archived_messages` 等）把多行实体压成一个文本字段，导致无法索引、无法反查、编辑只能读出整块重写。
3. 全库无 FK 声明却又有隐式引用，孤儿行随时发生；另一边 qiwei 库有 FK + `ON DELETE CASCADE`，两库风格不一致。
4. 时间戳格式混用：多数表 `BIGINT` ms，`agent_personas / group_persona_assignments` 却用 `TEXT DEFAULT CURRENT_TIMESTAMP`，跨表比较时需要解析转换。
5. 无软删语义，删除即物理销毁，无法审计、无法恢复。
6. `agent_sessions.messages TEXT DEFAULT '[]'` 等死列一直带着没清。

借迁库的停机窗口一次性还清这批债务，比以后单独排期成本更低——迁移工具本就要逐行搬运，附加 JSON 拆分、时间格式归一等一次性清洗逻辑是顺水人情。

## What Changes

### 基础设施

- **BREAKING**：`agent` / `channel-qiwei` 的业务数据库从 SQLite 切到 **MySQL 8.0**，两库独立（`moli_agent` / `moli_qiwei`），共享连接凭证（`MYSQL_HOST / PORT / USER / PASSWORD`）+ 各自 `*_MYSQL_DATABASE`。
- 引入 `goose` 版本化 migration，`embed.FS` 嵌入；淘汰 `db.go` 里的 `ALTER TABLE + 吞错误` 写法。
- `shared/logger` 保持 SQLite 不变。

### Schema 原则（全仓统一）

- **去外键**：删除 qiwei 库现有 FK；agent 库也不加 FK。完整性靠 repo 层封装 + 周期性一致性巡检。
- **软删**：所有实体表加 `deleted_at BIGINT NOT NULL DEFAULT 0`；所有 UNIQUE KEY 带 `deleted_at`；repo 层统一 `WHERE deleted_at = 0` 过滤。
- **时间戳**：全部 `BIGINT` 存 UTC epoch ms；禁用 `DATETIME / TIMESTAMP / CURRENT_TIMESTAMP`；展示层转本地时区。
- **字符集**：库级 `utf8mb4 / utf8mb4_bin`；ID 列 `VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin`。
- **JSON 存储**：避免使用 MySQL `JSON` 类型；自由 payload 一律 `MEDIUMTEXT` + 应用层 parse。

### JSON 列 → 14 张新关联表

- 从 `agent_configs` 拆：`agent_skill_bindings / agent_hooks / agent_channels / agent_subagent_models / agent_subagent_skill_bindings`。
- 从 `agent_personas` 拆：`persona_skill_bindings / persona_subagent_models / persona_subagent_skill_bindings`。
- 从 `messages` 拆：`message_tool_calls / message_attachments`。
- 从 `context_compaction_archives` 拆：`context_compaction_archived_messages`。
- 从 `skills` 拆：`skill_triggers / skill_tools`。
- 从 `qiwei_contacts` 拆：`qiwei_contact_followers`。

### Schema 债务清理

- **BREAKING**：删除 `agent_sessions.messages`（死列，grep 确认无 SQL 引用）。
- `agent_personas / group_persona_assignments` 的 TEXT 时间列改为 BIGINT epoch ms。
- 删除单列低基数索引 `idx_agent_sessions_exec_status`；`qiwei_contacts` 的 4 列复合名字索引拆成 4 个独立单列索引。
- 新增索引：`idx_messages_trace (trace_id)`、`idx_user_memory_facts_expires (expires_at)`、`idx_processed_messages_processed_at (processed_at)`。
- 删除运行时历史兼容代码：`migrateSkillBindingsToIDs` / `backfillSessionActiveSkillOrder` / `agent/cmd/migrate-timestamps`。

### SQL 方言改写

- `INSERT OR IGNORE` → `INSERT IGNORE`；`INSERT OR REPLACE` → `REPLACE INTO` / `ON DUPLICATE KEY UPDATE`；`ON CONFLICT ... DO UPDATE` → `ON DUPLICATE KEY UPDATE`；移除 `rowid` / `PRAGMA` / `SetMaxOpenConns(1)`。

### 迁移工具

- 新增 `cmd/migrate-sqlite-to-mysql`：一次性停机搬运，自带 JSON 拆行、TEXT 时间戳解析为 epoch ms、skill 绑定格式清洗、`activation_order` 回填，以及逐表行数对账。

### 非本次范围

- 敏感字段加密（`models.api_key` / `qiwei_accounts.token` 明文落库）— schema 里打 TODO，另立项。
- 测试 — 老单测若依赖 SQLite 文件直接标 `t.Skip`，不投入。
- 老 `.db` 文件不删、不迁移、不维护。

## Capabilities

### New Capabilities

- `mysql-persistence`：MySQL 连接、goose migration、SQL 方言约束、类型映射、软删语义、时间戳语义、连接池。
- `schema-redesign`：具体表变更清单——JSON 拆表的 14 张新表结构、被删字段/索引、重命名/类型变更。
- `sqlite-to-mysql-migration`：一次性迁移工具的契约、表清单、数据清洗规则、对账规则。

### Modified Capabilities

- 无。`openspec/specs/` 下现有 capability 不涉及持久化层的表级 behavior。

## Impact

**代码变更面（大）**

- `agent/internal/storage/`：`db.go` 重写为 MySQL + goose；几乎所有 `*.go` 涉及的 SQL 都要改方言；新增多份 repo（新表）。
- `channel-qiwei/`：`db.go / contact_repo.go / room_store.go` 方言改写；delete FK DDL。
- `agent/cmd/agent/main.go` / `channel-qiwei/main.go`：启动链接 MySQL 配置。
- `agent/internal/storage/types.go` / `models.go` 相应调整：`AgentConfig.Skills / Hooks / ...` 等字段从 `[]X` 改为通过 repo 懒加载或显式 join。
- 新增 `cmd/migrate-sqlite-to-mysql/main.go`（根目录一次性工具）。
- 删除 `agent/cmd/migrate-timestamps/main.go`。

**依赖变更**

- 新增 `github.com/go-sql-driver/mysql`、`github.com/pressly/goose/v3`（两模块 go.mod 各一份）。
- `modernc.org/sqlite`：`shared/logger` 保留；迁移工具内部保留读老库；`agent / channel-qiwei` 移除。

**部署**

- DBA 预先建 `moli_agent` / `moli_qiwei`（`utf8mb4 / utf8mb4_bin`），授权 DDL。
- `.env.example`、`Makefile`、`deploy/` 更新 env；停机窗口跑迁移工具；切流；回滚只在窗口当日。

**不受影响面**

- `shared/logger` 按天分库 SQLite。
- Agent ↔ Channel HTTP 契约。
- admin 前端 API 契约（虽然底层存储换了，但 handler 层输出的 JSON 形态保持）。
