## Context

仓库内 SQLite 使用三处：`agent/data/config.db`、`channel-qiwei/qiwei.db`、`shared/logger/sqlite/*.db`。前两个是业务持久化，后者是按天分库的观测日志。本次只迁前两个到 MySQL 8.0，后者不动。

除数据库引擎切换，借停机窗口一次性做以下 schema 层改造：去 FK、全表软删、时间戳统一 UTC epoch ms、14 个 JSON 列拆成关系表、清理若干死列和历史兼容代码。

## Goals / Non-Goals

**Goals**

- `agent` 与 `channel-qiwei` 切换到 MySQL 8.0，两独立 database 共享连接凭证。
- schema 层去 FK、软删、UTC epoch ms、JSON 拆表四项原则统一落地。
- `goose` 管理 schema，替代现有"`ALTER` 数组 + 吞错误"方式。
- 一次性迁移工具完成数据搬运 + 清洗（JSON 拆行、时间戳解析、遗留格式归一），逐表行数对账。
- 业务层 HTTP 契约 / 前端契约 / Agent ↔ Channel 协议均不变。

**Non-Goals**

- 不做 `shared/logger` MySQL 化。
- 不做 `models.api_key / qiwei_accounts.token` 加密（另立项，本次 schema 留 TODO 注释）。
- 不做在线双写 / 灰度，停机窗口内一次性切。
- 不做分库分表、读写分离、ORM 接入。
- 不新增或改造测试；已有依赖 SQLite 文件的单测直接 `t.Skip`。
- 不清理老 `.db` 文件（保留用于回滚）。

## Decisions

### D1. 库拆分：两独立 database，共享连接凭证

- `moli_agent` / `moli_qiwei`，字符集 `utf8mb4`，排序 `utf8mb4_bin`。
- 连接参数：`MYSQL_HOST / MYSQL_PORT / MYSQL_USER / MYSQL_PASSWORD` 共享；`AGENT_MYSQL_DATABASE`（默认 `moli_agent`）/ `QIWEI_MYSQL_DATABASE`（默认 `moli_qiwei`）各自持有。
- DSN 由进程内部拼接，不对外暴露。
- **替代方案**：单库表前缀。放弃——两服务独立 `go.mod`、独立 schema 演进节奏，合库会引入隐式耦合。

### D2. Schema migration：goose + embed.FS

- `agent/internal/storage/migrations/*.sql`、`channel-qiwei/migrations/*.sql` 分别用 `embed.FS` 嵌入。
- `00001_init.sql` 直接写最终态（基于当前 SQLite 终态 + 本次 schema 原则改造后的结构），不保留历史迁移分片。
- 启动时 `goose.Up`；失败即退出。
- 删除 `db.go` 里的 `stmts[]` / `migrations[]` 数组，`runSchema` 本体消失。
- **替代方案**：`golang-migrate`。放弃——goose 原生支持 Go migration 便于少量复杂清洗，心智负担更低。

### D3. SQL 方言改写

统一替换规则：

| SQLite | MySQL |
|---|---|
| `INSERT OR IGNORE` | `INSERT IGNORE` |
| `INSERT OR REPLACE` | `REPLACE INTO`（纯 seed）/ `INSERT ... ON DUPLICATE KEY UPDATE` |
| `ON CONFLICT(k) DO UPDATE SET c=excluded.c` | `ON DUPLICATE KEY UPDATE c=VALUES(c)` |
| `rowid` 列 | 改用显式 `id` 列（涉及 `session_active_skills` 排序、`backfillSessionActiveSkillOrder` 回填——后者放迁移工具一次性做完） |
| `PRAGMA *` | 全删 |
| `SetMaxOpenConns(1)` | 删；默认 `MaxOpenConns=16`、`MaxIdleConns=8`、`ConnMaxLifetime=30m`，可 env 覆盖 |

### D4. 类型映射

| 语义 | 类型声明 |
|---|---|
| 业务 ID / 外键引用 | `VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL` |
| 枚举 / 状态 / 短标识 | `VARCHAR(32)` |
| 外部 ID（`channel_message_id / trace_id / external_user_id`） | `VARCHAR(128)` |
| 业务文本（content / prompt / notes） | `TEXT`，`utf8mb4` |
| 大文本（archived_messages / raw_json） | `MEDIUMTEXT` |
| 时间戳（`*_at`） | `BIGINT NOT NULL DEFAULT 0`（0 = 未设置） |
| 布尔 | `TINYINT(1) NOT NULL DEFAULT 0` |
| 自增 PK | `BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY` |
| 浮点（`temperature`） | `DOUBLE` |
| 原始 JSON payload | `MEDIUMTEXT`（**不用** MySQL `JSON` 类型，避免 schema 校验失败） |

**索引收口**：所有 UNIQUE / INDEX 列必须是 `VARCHAR(n)`，禁止 `TEXT` 做索引键；单索引 key 长度预留 ≤ 3072 字节。

### D5. 去外键

- `channel-qiwei` 库现有 `FOREIGN KEY (...) ON DELETE CASCADE` 全部**删除**，与 agent 库拉齐。
- 理由：
  - FK 与软删语义冲突（FK 约束物理行，软删保留物理行；`ON DELETE CASCADE` 在软删世界里永远不触发，留着是死代码）。
  - InnoDB 的 FK 会在父表行上加 shared lock，子表写放大；`messages / session_events` 这种高频表尤其吃亏。
  - 未来任何拆库 / 分片路径都需要先拆 FK。
- **兜底**：
  - repo 层封装 upsert / 软删，禁止业务代码裸 `DB.Exec`。
  - 周期性 "孤儿行巡检" job（本次不做，tasks.md 留条目）。
  - 迁移工具在搬运前做一次性孤儿检查，不达标不放行。

### D6. 软删模型

**列定义**：`deleted_at BIGINT NOT NULL DEFAULT 0`，`0` 代表存活，`>0` 为软删时间戳（epoch ms UTC）。

**唯一键规则**：所有 UNIQUE KEY 都把 `deleted_at` 追加进去。软删后重新创建同 natural key 不冲突。例：

```sql
-- before
UNIQUE KEY uk_user_channels (channel_type, channel_user_id)
-- after
UNIQUE KEY uk_user_channels (channel_type, channel_user_id, deleted_at)
```

**表分类**：

- **软删表**（16 张）：`users / agent_configs / agent_personas / agent_sessions / skills / models / channel_groups / group_persona_assignments / user_channels / user_memory / qiwei_accounts / qiwei_contacts / qiwei_rooms / qiwei_room_members / qiwei_identity_links / qiwei_known_rooms`。
- **m2m 绑定表**（亦软删）：`agent_skill_bindings / agent_channels / agent_subagent_models / agent_subagent_skill_bindings / persona_skill_bindings / persona_subagent_models / persona_subagent_skill_bindings`。
- **纯"父表的数组属性"子表**（不软删，父表更新时 DELETE+INSERT in txn）：`agent_hooks / skill_triggers / skill_tools / message_tool_calls / message_attachments / qiwei_contact_followers / context_compaction_archived_messages`。
- **不软删**（状态机 / 不可变流水 / TTL）：`settings / messages / session_events / session_facts / context_compactions / context_compaction_archives / processed_messages / delayed_tasks / session_active_skills / user_memory_facts`。

**Repo 层约定**：

- 所有 SELECT 必须通过 `scopeAlive()` 辅助函数注入 `AND deleted_at = 0`；裸 `DB.Query` 只允许在迁移工具和巡检 job 出现。
- "硬删"接口（已在管理 API 里的 `hard_delete`）重命名为 `purge`，显式 `DELETE FROM`。

### D7. 时间戳模型

- **全部列**用 `BIGINT` 存 UTC epoch ms。0 = 未设置，>0 = 有效。
- **单一入口**：`timeutil.NowMs() = time.Now().UTC().UnixMilli()`（现有；审计无其他 `time.Now().Unix()` / `time.Now().UnixNano()` 绕过）。
- **禁用**：`DATETIME` / `TIMESTAMP` / `DEFAULT CURRENT_TIMESTAMP`。`agent_personas / group_persona_assignments` 现有 TEXT 时间列在迁移时解析为 ms。
- **DSN 不含 `time_zone` 参数**（不依赖 MySQL 格式化时间）。
- **展示层**：admin / API / 前端按 `Asia/Shanghai` 格式化，handler 层负责转换。

### D8. JSON 列 → 关系表

共 14 张新表（详见 `specs/schema-redesign/spec.md`）。拆分原则：

- **拆**：有独立身份或需要反查/统计的集合（skills 绑定、tool_calls、attachments、archived_messages 等）。
- **留 MEDIUMTEXT**：自由 KV 元数据、上游原始 payload（`users.metadata / *.raw_json / *.meta_json / channel_meta` 等）；留的也不用 `JSON` 类型。

每个拆出来的子表要么是 m2m 绑定（软删），要么是父表的数组属性（不软删，txn 整替）。

### D9. 死列 / 死代码清理

- **删列**：`agent_sessions.messages`（已 grep 确认无 SQL 引用）。
- **删索引**：`idx_agent_sessions_exec_status`（基数低）；`idx_qiwei_contacts_name_lookup`（改 4 个单列索引）。
- **加索引**：`idx_messages_trace (trace_id)`、`idx_user_memory_facts_expires (expires_at)`、`idx_processed_messages_processed_at (processed_at)`。
- **删代码**：
  - `agent/internal/storage/db.go::migrateSkillBindingsToIDs`（迁移工具一次性做完）
  - `agent/internal/storage/db.go::backfillSessionActiveSkillOrder`（同上）
  - `agent/cmd/migrate-timestamps/main.go`（SQLite 专用一次性工具，MySQL 化后无意义）

### D10. Seed 数据

- `seedDefaultModels()`：保留启动时跑，但 `INSERT OR IGNORE` → `INSERT IGNORE`。理由：默认模型列表会不定期新增，写 goose migration 每次改都要版本号递增不方便。
- `agent/data/043-wecom-skills.sql`：改写为 MySQL 方言并封装成 goose 业务 migration（`00002_seed_wecom_skills.sql`），启动副作用消除；模型数据拆表后 seed 里要同时写 `skills` + `skill_triggers` + `skill_tools`（txn）。

### D11. 一次性迁移工具

- 路径：`cmd/migrate-sqlite-to-mysql/main.go`（根仓库）。
- 两个 scope：`--scope agent` / `--scope qiwei`。
- 每张源表按"读 SQLite → 可选清洗 → 写 MySQL"管线，每批事务提交；表完成后 `COUNT(*)` 对账。
- 清洗逻辑：
  - **JSON 拆行**：父表插入时解析 JSON 列 → 按子表 schema 插入 N 行；删原 JSON 列。
  - **时间戳转换**：SQLite `YYYY-MM-DD HH:MM:SS` UTC 解析为 ms；空字符串 → 0。
  - **skills 绑定格式归一**：老 `[{"id":"x","mode":"y"}]` → `agent_skill_bindings` 行（mode 落到 `mode` 列）；新 `["x"]` → 行。
  - **activation_order 回填**：从源 `session_active_skills.rowid` 赋值。
  - **软删标记初始化**：所有新行 `deleted_at=0`；老库里 `enabled=0` 的行**不自动转软删**（语义不同：`enabled` 仍是业务开关，软删是生命周期）。
- 不支持部分表迁移；要么整个 scope 成功要么失败。
- `--truncate-before` 重跑支持；目标表非空且未授权清空时立即退出。

### D12. 运行时启动契约

- MySQL 连接失败 → `log.Fatal`。
- goose migration 失败 → `log.Fatal`，日志含失败文件名。
- 运行时连接断 → `database/sql` 自动重连；业务层继续处理。
- 启动时 **警告并忽略** 旧 env（`AGENT_DB_PATH / QIWEI_DB_PATH`），不回退 SQLite。

## Risks / Trade-offs

- **[R1] 迁移对账不等**：停机窗口强制生效；不一致直接失败人工介入。
- **[R2] 去 FK 后孤儿行**：代码改出 bug 不会被 DB 阻止。缓解：repo 层封装 + 一致性巡检 job（tasks.md 单列条目）。
- **[R3] JSON 拆表后读路径 N+1**：列表接口要为每个 agent 取 skills/hooks 得单独查。缓解：`GetAgentWithBindings()` 一把捞 + 应用层组装；必要时用 `IN (...)` 批量预取。
- **[R4] 软删后数据量膨胀**：归档策略另立项，短期不治理。缓解：`deleted_at` 带索引的表（有查询需求时）可加，其他靠运维监控 row count。
- **[R5] MySQL `utf8mb4_bin` 比较行为**：`'abc' = 'ABC'` 不再命中。代码层审计字符串比较（grep `LOWER(` / `LIKE '%'`）——业务上应该本就不期望这种宽容匹配。
- **[R6] `00001_init.sql` 与实际 SQLite 漂移**：本次初始 migration 从"SQLite 终态"推导，可能遗漏某些 `ALTER` 时没留痕迹的列。缓解：生成后用 `sqlite3 .schema` 对照 + 迁移演练兜底。
- **[R7] 回滚窗口受限**：新版本一旦跑过就无法回 SQLite（MySQL 增量无处回灌）。回滚只允许在部署当日，老 `.db` 未被新版本写过时。
- **[R8] 并发提升暴露潜在竞态**：SQLite 单写锁原本"顺序串行化"了许多路径；MySQL 并发下可能暴露 agent 端竞态。本次不压测，依赖线上 metric 发现后单独修。
- **[R9] 敏感字段仍明文**：`models.api_key / qiwei_accounts.token` 继续明文落库，已知债务。schema 加 `-- TODO: encrypt` 注释，单独立项追踪。

## Migration Plan

1. 分 PR 推进（见 `tasks.md`）；合并后先不部署。
2. DBA 建 `moli_agent / moli_qiwei`（`utf8mb4 / utf8mb4_bin`），账号 `ALL PRIVILEGES`（goose 要 DDL）。
3. 测试环境用生产 `.db` 快照跑一次完整迁移演练，记录耗时，对照源行数 vs 目标行数（注意 JSON 拆表后子表行数预期远大于父表）。
4. 停机窗口：
   1. 停 `agent / channel-qiwei` 所有实例。
   2. 备份 MySQL（空库也备）。
   3. 跑 `migrate-sqlite-to-mysql --scope agent` + `--scope qiwei`。
   4. 对账通过后部署新二进制（env 切 MySQL）。
   5. 冒烟：企微消息一来一回、webui 对话一次、delayed task 创建一次、admin 前端 list 一次。
5. 24 小时观察：慢查询、连接池、错误日志。
6. 回滚：回滚二进制 + 恢复老 env；仅限部署当日。
