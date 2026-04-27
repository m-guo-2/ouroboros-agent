## ADDED Requirements

### Requirement: MySQL 作为 agent 与 channel-qiwei 的持久化后端

`agent` 和 `channel-qiwei` 进程 SHALL 使用 MySQL 8.0 作为唯一业务持久化后端。两个服务 MUST 使用独立 database：`agent` 使用 `moli_agent`、`channel-qiwei` 使用 `moli_qiwei`。两个 database MUST 采用 `utf8mb4` 字符集与 `utf8mb4_bin` 排序规则。

#### Scenario: 启动成功连接
- **WHEN** 服务启动且读取到完整的 MySQL 连接配置
- **THEN** 服务 MUST 使用 `github.com/go-sql-driver/mysql` 建立连接池并自动执行 `goose.Up`

#### Scenario: 连接配置缺失
- **WHEN** 进程启动但任一共享连接环境变量缺失
- **THEN** 服务 MUST 以非零状态码退出；MUST NOT 回退到 SQLite

#### Scenario: 旧 SQLite 路径配置仍存在
- **WHEN** 启动时 `AGENT_DB_PATH` 或 `QIWEI_DB_PATH` 仍被设置
- **THEN** 服务 MUST 忽略这些变量并输出一条 `warn` 日志，指引运维清理

---

### Requirement: 连接配置：共享凭证 + 独立 database

连接参数 SHALL 通过共享环境变量 `MYSQL_HOST / MYSQL_PORT / MYSQL_USER / MYSQL_PASSWORD` 表达。每个服务 MUST 各自持有 `*_MYSQL_DATABASE` 变量：`AGENT_MYSQL_DATABASE`（默认 `moli_agent`）、`QIWEI_MYSQL_DATABASE`（默认 `moli_qiwei`）。DSN 字符串 MUST 在进程内部拼接，MUST NOT 通过单一 env 提供完整 DSN。

#### Scenario: 默认 database 名
- **WHEN** 部署者只设置共享凭证变量
- **THEN** `agent` MUST 连接 `moli_agent`，`channel-qiwei` MUST 连接 `moli_qiwei`

#### Scenario: 覆盖 database 名
- **WHEN** 部署者设置 `QIWEI_MYSQL_DATABASE=moli_qiwei_staging`
- **THEN** `channel-qiwei` MUST 连接 `moli_qiwei_staging`，MUST 不影响 `agent` 所用 database

---

### Requirement: Schema 版本化管理

全部 schema 变更 SHALL 通过 `github.com/pressly/goose/v3` 管理。`agent` 与 `channel-qiwei` MUST 各自维护独立的 migration 目录，通过 `embed.FS` 嵌入二进制。进程启动时 MUST 自动执行 `goose.Up` 到最新版本；失败 MUST 以非零状态码退出，错误日志 MUST 包含失败文件名。

MUST 从代码库中删除以下模式：

- `agent/internal/storage/db.go` 中的 `stmts []string` 与 `migrations []string` 数组。
- 任何通过 `db.Exec(ALTER TABLE ...)` 且忽略返回错误的增量迁移写法。
- `channel-qiwei/db.go::runSchema` 中的 `stmts` 数组。

初始 migration（`00001_init.sql`）MUST 以最终态一次性声明 schema，MUST NOT 保留 SQLite 专用语法（`PRAGMA / rowid`）。

#### Scenario: 首次空库启动
- **WHEN** 服务连到空的 `moli_agent`
- **THEN** 服务 MUST 先跑完 goose migrations 后才接受业务请求

#### Scenario: Migration 执行失败
- **WHEN** goose 某个 `.sql` 报错
- **THEN** 进程 MUST 立即退出，错误日志 MUST 指明失败的 migration 文件名

---

### Requirement: SQL 方言为 MySQL

所有应用代码中进入 MySQL 的 SQL 语句 MUST 使用 MySQL 方言，MUST NOT 出现以下 SQLite 构造：

- `INSERT OR IGNORE` / `INSERT OR REPLACE`
- `ON CONFLICT (...) DO UPDATE SET col = excluded.col`
- `rowid` 隐式列（在 ORDER BY / WHERE / SELECT 中）
- `PRAGMA` 语句
- `last_insert_rowid()` 函数（改用 `LastInsertId()` 从驱动获取）

**替换规则**：

- `INSERT OR IGNORE INTO t ...` → `INSERT IGNORE INTO t ...`
- `INSERT OR REPLACE INTO t ...` → `REPLACE INTO t ...`（纯 seed）或 `INSERT ... ON DUPLICATE KEY UPDATE`（业务 upsert）
- `INSERT ... ON CONFLICT(k) DO UPDATE SET c=excluded.c` → `INSERT ... ON DUPLICATE KEY UPDATE c=VALUES(c)`
- 排序中 `rowid` → 显式 `id` 列或业务时间列

#### Scenario: 联系人 upsert
- **WHEN** `contact_repo.go` 写入一条已存在 `(account_id, user_id)` 的联系人
- **THEN** 使用 `INSERT ... ON DUPLICATE KEY UPDATE nickname=VALUES(nickname), ...`；`first_seen_at` MUST 通过 `first_seen_at=first_seen_at` 保持不变

#### Scenario: 会话激活 skill 排序
- **WHEN** `agent` 按激活顺序查询某会话的 active skill
- **THEN** ORDER BY MUST 不含 `rowid`，MUST 使用 `activation_order ASC, created_at ASC, id ASC`

---

### Requirement: 类型映射规则

Schema 中列类型 MUST 遵守以下映射：

- 业务 ID / 外部 ID 引用列：`VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL`
- 外部系统消息 ID / trace ID（`channel_message_id / trace_id / external_user_id / channel_user_id`）：`VARCHAR(128)`
- 枚举 / 状态 / 短标识（`role / level / status / source / channel`）：`VARCHAR(32)`
- 业务文本（`content / system_prompt / notes`）：`TEXT`，字符集 `utf8mb4`
- 大文本（`raw_json / archived_messages / summary`）：`MEDIUMTEXT`
- 时间戳列（所有 `*_at`）：`BIGINT NOT NULL DEFAULT 0`
- 布尔：`TINYINT(1) NOT NULL DEFAULT 0`
- 自增 PK：`BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY`
- 浮点（`temperature`）：`DOUBLE`

MUST NOT 在任何业务表使用 MySQL `JSON` 类型。原始 payload 列统一用 `MEDIUMTEXT`，由应用层 parse。

所有 UNIQUE / INDEX 所覆盖的列 MUST 是 `VARCHAR(n)`，MUST NOT 是 `TEXT`。

#### Scenario: ID 列 collation
- **WHEN** 检查迁移后 `agent_sessions.id` 的 DDL
- **THEN** 列定义 MUST 为 `VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL`

#### Scenario: 索引键长度
- **WHEN** 检查任一 UNIQUE 或 INDEX 所覆盖的列
- **THEN** 列 MUST NOT 是 `TEXT`；单索引 key 总长 MUST ≤ 3072 字节

#### Scenario: 业务表禁用 JSON 类型
- **WHEN** 检查任意业务表的列类型
- **THEN** MUST NOT 出现 `JSON` 类型声明（`MEDIUMTEXT` 替代）

---

### Requirement: 不使用数据库外键

两个 database 中 MUST NOT 声明任何 `FOREIGN KEY` 约束。`channel-qiwei` 原有的 `FOREIGN KEY (account_id) REFERENCES qiwei_accounts(id) ON DELETE CASCADE` 等 MUST 在迁移时移除。

引用关系的完整性 MUST 由 repo 层封装保障，业务代码 MUST NOT 绕过 repo 层直接 `DB.Exec` 写入有引用关系的表。

#### Scenario: 迁移后 qiwei DDL 检查
- **WHEN** 在 `moli_qiwei` 执行 `SHOW CREATE TABLE qiwei_contacts`
- **THEN** 输出 MUST NOT 包含 `FOREIGN KEY` 或 `ON DELETE CASCADE` 字样

#### Scenario: agent DDL 检查
- **WHEN** 在 `moli_agent` 任一表执行 `SHOW CREATE TABLE`
- **THEN** 输出 MUST NOT 包含 `FOREIGN KEY` 约束

---

### Requirement: 软删通用规则

所有被标记为"实体表"或"m2m 绑定表"的表 MUST 声明 `deleted_at BIGINT NOT NULL DEFAULT 0` 列。`0` 代表存活行，`>0` 代表软删时间戳（UTC epoch ms）。

所有 UNIQUE KEY MUST 把 `deleted_at` 追加为组合键的最后一列，使软删后重新创建同 natural key 不产生冲突。

Repo 层 MUST 提供 `scopeAlive()` 辅助，所有业务 SELECT MUST 通过它注入 `AND deleted_at = 0`。业务代码 MUST NOT 在 repo 层之外裸写 `SELECT`。

"硬删"接口 MUST 重命名为 `purge`，其实现 MUST 显式执行 `DELETE FROM`，MUST NOT 和软删共用函数名。

软删表清单和非软删表清单 MUST 按 `specs/schema-redesign/spec.md` 的约定执行。

#### Scenario: 软删后重建同 natural key
- **WHEN** 某 `user_channels` 行被软删（`deleted_at` 被赋值为 `t1 > 0`）后，业务再次创建同 `(channel_type, channel_user_id)` 的绑定
- **THEN** INSERT MUST 成功（唯一键 `(channel_type, channel_user_id, deleted_at)` 允许新行 `deleted_at=0`）

#### Scenario: 默认查询自动过滤软删
- **WHEN** 业务调用 `GetUser(id)` 查询一个已软删的 user
- **THEN** 返回 MUST 为"未找到"；repo 层 MUST 拼接 `WHERE deleted_at = 0`

#### Scenario: 显式硬删接口
- **WHEN** admin API 调用 `purgeAccount(id)`
- **THEN** 该实现 MUST 执行 `DELETE FROM`，MUST NOT 只更新 `deleted_at`

---

### Requirement: 时间戳使用 UTC epoch milliseconds

所有时间戳列 MUST 声明为 `BIGINT NOT NULL DEFAULT 0`；`0` 表示未设置，`>0` 为有效时间戳。业务代码写入时 MUST 使用统一入口 `timeutil.NowMs()`（定义为 `time.Now().UTC().UnixMilli()`）。

MUST NOT 在任何表使用 MySQL `DATETIME / TIMESTAMP` 类型。MUST NOT 使用 `DEFAULT CURRENT_TIMESTAMP`。MySQL DSN MUST NOT 包含 `time_zone` 参数，MUST NOT 设置 `parseTime=true`（业务不读取 DATETIME）。

展示层（admin API / 前端）MUST 在 handler 层把 epoch ms 转换为 `Asia/Shanghai` 时区的 ISO-8601 / 本地化字符串，MUST NOT 让 DB 承担格式化职责。

#### Scenario: Schema 不含 DATETIME
- **WHEN** 检查任一业务表的列类型
- **THEN** MUST NOT 出现 `DATETIME` 或 `TIMESTAMP` 类型

#### Scenario: 写入时间戳一致性
- **WHEN** 业务任何地方写 `created_at / updated_at / deleted_at / expires_at`
- **THEN** 值 MUST 来自 `timeutil.NowMs()` 或显式的 UTC ms 计算

#### Scenario: 历史 TEXT 时间戳迁移
- **WHEN** 迁移工具搬运 `agent_personas.created_at` 值为 `'2025-03-12 10:00:00'`
- **THEN** 目标 MySQL 对应列 MUST 存入 `1741773600000`（按 UTC 解析的 epoch ms）

---

### Requirement: 连接池参数

连接池默认参数 MUST 为 `MaxOpenConns=16`、`MaxIdleConns=8`、`ConnMaxLifetime=30m`。代码中 MUST NOT 再出现 `db.SetMaxOpenConns(1)` 及类似 SQLite 单写者假设。参数 MUST 可通过环境变量覆盖（`AGENT_MYSQL_MAX_OPEN_CONNS` 等）。

#### Scenario: 默认参数
- **WHEN** 进程启动未设置相关 env
- **THEN** `MaxOpenConns` MUST 为 16

#### Scenario: env 覆盖
- **WHEN** 设置 `AGENT_MYSQL_MAX_OPEN_CONNS=64`
- **THEN** `MaxOpenConns` MUST 为 64

---

### Requirement: shared/logger 不受影响

`shared/logger` MUST 继续使用 SQLite 按天分库（`{logDir}/sqlite/{YYYY-MM-DD}.db`）。本次变更 MUST NOT 修改 `shared/logger` 的公共接口、持久化路径、保留策略或 BLOB 列结构。

#### Scenario: agent 切 MySQL 后 logger 仍写 SQLite
- **WHEN** `agent` 已使用 MySQL，执行一次 trace
- **THEN** `shared/logger` MUST 正常把 events / llm_io 写入当天 `.db` 文件
