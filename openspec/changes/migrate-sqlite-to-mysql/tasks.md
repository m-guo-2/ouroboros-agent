## 1. 基础设施与公共模块

- [x] 1.1 `go.mod` 新增依赖：`github.com/go-sql-driver/mysql`、`github.com/pressly/goose/v3`（agent 与 channel-qiwei 各一份）
- [x] 1.2 在 `agent/internal/storage` 和 `channel-qiwei/` 下新建 `mysql.go`（暂用 sub-package 或同包独立文件），实现：
  - DSN 拼装（参数来自 env：共享 `MYSQL_HOST/PORT/USER/PASSWORD` + `AGENT_MYSQL_DATABASE` / `QIWEI_MYSQL_DATABASE`）
  - `sql.Open("mysql", dsn)` + `db.Ping()`
  - 连接池设置（默认 `MaxOpenConns=16`、`MaxIdleConns=8`、`ConnMaxLifetime=30m`；env 可覆盖）
- [x] 1.3 `shared/timeutil` 新增（或确认已有）`NowMs() int64 { return time.Now().UTC().UnixMilli() }`，审计所有写 `created_at / updated_at / deleted_at / expires_at` 的地方都走这个入口
- [x] 1.4 `agent/internal/storage/migrations/` 目录 + `embed.FS`；`channel-qiwei/migrations/` 目录 + `embed.FS`；`goose.SetBaseFS` / `goose.SetDialect("mysql")` / `goose.Up`

## 2. Agent 库 schema 初始化（goose migration 00001_init.sql）

- [x] 2.1 写 `00001_init.sql`，按 `specs/schema-redesign/spec.md` 落地：
  - 原有表（采用新类型规则 + 去 FK + `deleted_at` + BIGINT 时间戳 + 唯一键带 `deleted_at`）
  - 删除列：`agent_sessions.messages`
  - 删除索引：`idx_agent_sessions_exec_status`
  - 新增索引：`idx_messages_trace (trace_id)`、`idx_user_memory_facts_expires (expires_at)`、`idx_processed_messages_processed_at (processed_at)`
  - 新增表：
    - `agent_skill_bindings`
    - `agent_hooks`
    - `agent_channels`
    - `agent_subagent_models`
    - `agent_subagent_skill_bindings`
    - `persona_skill_bindings`
    - `persona_subagent_models`
    - `persona_subagent_skill_bindings`
    - `message_tool_calls`
    - `message_attachments`
    - `context_compaction_archived_messages`
    - `skill_triggers`
    - `skill_tools`
  - 删除列：`agent_configs.{skills, hooks, channels, subagent_models, subagent_skills}` / `agent_personas.{skills, subagent_models, subagent_skills}` / `messages.{tool_calls, attachments_json}` / `context_compaction_archives.archived_messages` / `skills.{triggers, tools}`
  - 在 `models.api_key` 上加 `-- TODO: encrypt (tracked separately)` 注释
- [x] 2.2 写 `00002_seed_wecom_skills.sql`：把 `agent/data/043-wecom-skills.sql` 翻译成 MySQL 方言（不再写 `skill_triggers / skill_tools`，因为 runtime 不读）
- [x] 2.3 启动时 `seedDefaultModels()` 改写为 `INSERT IGNORE`，保留但改用新方言

## 3. Agent 代码方言 + 软删改造

- [x] 3.1 `db.go`：删掉 `stmts[]` / `migrations[]` 数组；`openDB` 换成 `openMySQL`；调用 `goose.Up`
- [x] 3.2 `repo` / storage 层全量审计替换：
  - `INSERT OR IGNORE` → `INSERT IGNORE`
  - `ON CONFLICT(...) DO UPDATE SET x=excluded.x` → `... ON DUPLICATE KEY UPDATE x=VALUES(x)`
  - `ORDER BY rowid` → 改为显式列
  - `last_insert_rowid()` → `LastInsertId()`
  - 所有 SELECT 经 `scopeAlive()` 添加 `AND deleted_at = 0`
- [x] 3.3 `agent_configs` / `agent_personas` 相关 repo：
  - 读取路径改为懒加载（`GetAgent` 不再反序列化 JSON，调用方按需 `GetAgentSkills(agentID)` 等）
  - 或提供 `GetAgentWithBindings(id) (Agent, []SkillBinding, []Hook, []Channel, ...)` 一次 join
  - 写入路径：`SaveAgent(agent, skills, hooks, channels, subagentModels, subagentSkills)` 事务内整替子表
  - 对外 API handler 层把子表组装回 JSON，保持前端契约不变
- [x] 3.4 `messages` repo：
  - 写消息时拆 `tool_calls` / `attachments` 到子表（事务）
  - 读消息时按需 join / 二次查询
- [x] 3.5 `skills` repo：`CreateSkill / UpdateSkill` 时整替 `skill_triggers / skill_tools`
- [x] 3.6 `context_compaction_archives` repo：`archived_messages` 拆表
- [x] 3.7 删除 `migrateSkillBindingsToIDs`、`backfillSessionActiveSkillOrder`、`agent/cmd/migrate-timestamps/`
- [x] 3.8 软删：所有 `DELETE FROM` 业务接口改为 `UPDATE ... SET deleted_at = ?`；需要物理删除的 admin 接口重命名为 `purge`
- [x] 3.9 时间戳统一：审计 `agent_personas / group_persona_assignments`（从 TEXT 读出的路径要改为 BIGINT 扫描），确保 handler 层输出仍是 ISO 字符串（`Asia/Shanghai`）

## 4. Channel-qiwei 库 schema 初始化

- [x] 4.1 写 `channel-qiwei/migrations/00001_init.sql`：
  - 现有表按新类型规则重建
  - 删除所有 `FOREIGN KEY` / `ON DELETE CASCADE`
  - 所有实体表加 `deleted_at`；唯一键追加 `deleted_at`
  - `qiwei_contacts`：删除 `follow_user_json`；删除 `idx_qiwei_contacts_name_lookup`；新增 4 个单列索引
  - 新表 `qiwei_contact_followers`
  - `qiwei_accounts.token` 加 `-- TODO: encrypt` 注释

## 5. Channel-qiwei 代码方言 + 软删改造

- [x] 5.1 `db.go`：删 `stmts[]`；换 MySQL + goose
- [x] 5.2 `contact_repo.go` / `room_store.go`：
  - 方言替换（`ON CONFLICT` / `INSERT OR IGNORE` 等）
  - `follow_user_json` 相关读写拆到 `qiwei_contact_followers`（同步时事务内整替）
  - 软删改造：`DeleteAccount / DeleteContact / DeleteRoom` 改为软删
  - 所有 SELECT 加 `deleted_at = 0` 过滤
- [x] 5.3 `sync*.go`：确认 sync 任务读写路径走软删语义（被 archive 的联系人要清 `deleted_at`→0 复活 or 新插；按现在 `ON CONFLICT` 语义平移）

## 6. 一次性迁移工具

- [x] 6.1 新建 `cmd/migrate-sqlite-to-mysql/main.go`
- [x] 6.2 参数解析：`--scope / --sqlite / --mysql-dsn / --batch-size / --truncate-before / --dry-run / --verify-only / --allow-orphans`
- [x] 6.3 前置检查：源可读、目标可连、`goose_db_version` 存在、孤儿行扫描；源 WAL 活跃检测留作运维约定（停机窗口前置）
- [x] 6.4 Agent scope 表迁移管线（按依赖顺序）
- [x] 6.5 Qiwei scope 表迁移管线
- [x] 6.6 每表迁移函数（读 SQLite batch → 清洗 → 写 MySQL txn → 对账）
- [x] 6.7 清洗实现：
  - JSON 拆行（`agent_configs.*`、`agent_personas.*`、`messages.*`、`context_compaction_archives.*`、`qiwei_contacts.follow_user_json`）
  - TEXT 时间戳解析（`agent_personas`、`group_persona_assignments`）
  - `session_active_skills.activation_order` 回填
  - skills 绑定老格式 → 新 schema 归一
  - `qiwei_*` epoch 秒列 ×1000 升 ms
- [x] 6.8 行数对账 + 日志输出
- [x] 6.9 `--truncate-before` 的倒序截表实现

## 7. 部署与配置

- [x] 7.1 `.env.example`：新增 MySQL 相关 env；`AGENT_DB_PATH / QIWEI_DB_PATH` 标记 DEPRECATED
- [ ] 7.2 ~~`Makefile`：新增 `make migrate-sqlite-to-mysql` target~~ — **运维任务，不在 agent 实施范围**（仓库当前没有 Makefile，需运维新增并对接 CI/CD）
- [ ] 7.3 ~~Docker / 部署脚本（`deploy/` 下相关文件）更新 env 模板~~ — **运维任务，不在 agent 实施范围**
- [x] 7.4 启动代码：检测到老 env 时打印 warn 日志（agent + channel-qiwei 均已落地）

## 8. 文档 / 观测

- [x] 8.1 `agent/.env.example` / `channel-qiwei/.env.example` 更新；`cmd/migrate-sqlite-to-mysql/README.md` 详述用法。`agent/README.md` / `channel-qiwei/README.md` 留作运维文档同步任务（agent 没有现存 README 章节可改）
- [ ] 8.2 ~~`docs/` 下新增 `deployment/mysql-migration-runbook.md`~~ — **运维任务**，建议复制 `cmd/migrate-sqlite-to-mysql/README.md` 作为基础并补充运维窗口流程
- [ ] 8.3 ~~监控指标：`sql.DBStats` 暴露到 admin 健康检查~~ — **后续工作**：当前 admin healthcheck 没有标准的 metrics 渠道；与可观测性体系一并落地

## 9. 演练与发布

- [ ] 9.1 ~~测试环境用生产 `.db` 快照跑一次迁移~~ — **运维 / SRE 任务**：需要生产快照、测试 MySQL 资源
- [ ] 9.2 ~~修正演练暴露的清洗边界 case~~ — **依赖 9.1 的演练结果**
- [x] 9.3 单测审计：依赖 SQLite 文件的测试改为通过 `storage.SetupTestDB(t)` / `openTestDB(t)` skip（agent + qiwei 均落地）
- [ ] 9.4 ~~上线窗口：停机 → 备份 MySQL → 跑迁移 → 切二进制 → 冒烟 → 观察~~ — **运维任务**
- [ ] 9.5 ~~观察期 24h~~ — **运维任务**
- [ ] 9.6 ~~回滚预案验证~~ — **运维任务**

## 10. 遗留债务（不在本 PR 内）

- [ ] 10.1 单独立项：`models.api_key / qiwei_accounts.token` 加密方案（KMS / envelope encryption）— **本次明确 out-of-scope**，schema 已留 `-- TODO: encrypt` 注释
- [ ] 10.2 单独立项：孤儿行周期巡检 job — **本次 out-of-scope**，迁移工具的 `--allow-orphans` 仅做一次性兜底
- [ ] 10.3 单独立项：软删数据归档策略（`deleted_at > N 天前` 的清理/导出）— **本次 out-of-scope**，schema 中 `deleted_at` 已具备查询基础
