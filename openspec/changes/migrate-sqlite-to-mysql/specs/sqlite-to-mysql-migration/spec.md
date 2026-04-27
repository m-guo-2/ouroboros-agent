## ADDED Requirements

### Requirement: 独立的一次性 CLI

本次迁移 SHALL 通过仓库根目录下的 `cmd/migrate-sqlite-to-mysql` Go 命令执行，MUST 是一次性、停机、离线使用的工具。MUST NOT 为任何常驻服务逻辑调用。迁移完成后源 `.db` 文件 MUST 保持原样不被工具修改或删除。

#### Scenario: CLI 退出码
- **WHEN** 迁移过程中任一步骤失败
- **THEN** 进程 MUST 以非零状态码退出；MUST NOT 留下部分提交的目标表（scope 级事务或 per-table 已对账）

#### Scenario: CLI 不清理源文件
- **WHEN** 迁移成功完成
- **THEN** 源 SQLite `.db` 文件 MUST 仍存在且未被改写

---

### Requirement: CLI 参数契约

CLI MUST 支持以下参数：

- `--scope {agent|qiwei|all}`：必填
- `--sqlite PATH`：源 SQLite 文件；MUST 必填且文件存在；agent 时指向 `config.db`，qiwei 时指向 `qiwei.db`
- `--mysql-dsn DSN`：目标 MySQL DSN；MUST 必填
- `--batch-size INT`：每批行数，默认 500
- `--truncate-before`：目标表非空时，先 `TRUNCATE` 再写；默认 `false`
- `--dry-run`：仅读源 + 打印计划，不写目标；默认 `false`
- `--verify-only`：跳过写入，只比对源 / 目标行数

#### Scenario: 目标表非空且未授权清空
- **WHEN** `--truncate-before` 未设置且目标任一表行数 > 0
- **THEN** CLI MUST 在所有表开始前退出并提示 `target table not empty; pass --truncate-before to overwrite`

#### Scenario: `--scope all` 顺序
- **WHEN** `--scope all`
- **THEN** CLI MUST 按 `agent → qiwei` 顺序执行；agent 失败时 qiwei 不启动

---

### Requirement: 表级迁移顺序与对账

每个 scope 内，CLI MUST 按依赖拓扑排序逐表迁移。每张源表完成后 MUST 立即执行 `COUNT(*)` 对账：

- 未拆分 JSON 的表：源行数 MUST 等于目标父表行数
- 拆分 JSON 的表：源父表行数 MUST 等于目标父表行数；子表行数 MUST 等于源 JSON 数组元素总和

行数不等 MUST 立即终止迁移，日志输出源/目标计数差。

**agent scope 顺序**：
1. `settings / users / user_channels / models`
2. `skills` + 同步写入 `skill_triggers / skill_tools`
3. `agent_configs` + 同步写入 `agent_skill_bindings / agent_hooks / agent_channels / agent_subagent_models / agent_subagent_skill_bindings`
4. `agent_personas` + 同步写入 `persona_skill_bindings / persona_subagent_models / persona_subagent_skill_bindings`
5. `channel_groups / group_persona_assignments`
6. `agent_sessions / session_active_skills / session_events / session_facts`
7. `messages` + 同步写入 `message_tool_calls / message_attachments`
8. `context_compactions / context_compaction_archives` + `context_compaction_archived_messages`
9. `processed_messages / delayed_tasks / user_memory / user_memory_facts`

**qiwei scope 顺序**：
1. `qiwei_accounts`
2. `qiwei_contacts` + 同步写入 `qiwei_contact_followers`
3. `qiwei_rooms / qiwei_room_members / qiwei_identity_links / qiwei_known_rooms`

#### Scenario: 行数对不齐
- **WHEN** 源 `agent_configs` 有 10 行，目标插入后 `COUNT(*) = 9`
- **THEN** CLI MUST 报错退出；日志 MUST 包含 `agent_configs: source=10, target=9`

#### Scenario: JSON 子表对账
- **WHEN** 源 `messages.tool_calls` JSON 数组元素总和为 1200
- **THEN** `message_tool_calls` 表的行数 MUST 等于 1200

---

### Requirement: 数据清洗规则

在写入目标表之前，CLI MUST 执行下列清洗：

**1. 时间戳解析**

- SQLite 中以 `TEXT` 存的时间（`agent_personas.created_at` 等）：按 UTC 解析 `YYYY-MM-DD HH:MM:SS` / `YYYY-MM-DDTHH:MM:SSZ` 格式为 epoch ms；解析失败 MUST 终止迁移。
- 空字符串 / NULL → `0`。
- SQLite 中以 `INTEGER` 存的时间：原值直接写入（单位已是 ms；若个别列是秒粒度，CLI 内部 hard-code 已知列名做 `*1000` 转换）。

**2. JSON 拆行**

- 源 `agent_configs.skills` 支持两种格式并归一：
  - 老格式 `[{"id":"x","mode":"y"}]` → `agent_skill_bindings (skill_id=x, mode=y)`
  - 新格式 `["x","y"]` → `agent_skill_bindings (skill_id=x, mode='')`
- 源 `agent_configs.subagent_skills` 结构为 `{subagentKey: [skillID, ...]}`，每个元素 `(subagent_key, skill_id, position=数组下标)` 写入 `agent_subagent_skill_bindings`。
- 源 `agent_configs.subagent_models` 结构为 `{subagentKey: {"provider":"p","model":"m"}}`，每个 entry 写入 `agent_subagent_models`。
- 源 `messages.tool_calls` / `messages.attachments_json`、`context_compaction_archives.archived_messages`、`qiwei_contacts.follow_user_json`、`skills.triggers` / `skills.tools`、`agent_personas` 的三个 JSON 列、`agent_configs.hooks / channels`：均按对应子表 schema 逐元素展开；`position / seq` MUST 赋值为数组下标。
- 源 JSON 列 NULL / 空字符串 / `"[]"` / `"{}"`：跳过（不报错，子表 0 行）。
- 源 JSON 解析失败 MUST 终止迁移，日志打印父表主键 + 列名。

**3. `session_active_skills.activation_order` 回填**

- 如果源列值为 `0` 或空，按 `rowid ASC` 的顺序在每个 `session_id` 分组内赋值 0, 1, 2, ...（等价替代老 `backfillSessionActiveSkillOrder`）。
- 如果源列值已 > 0，原样搬运。

**4. 软删列初始化**

- 所有新表行 `deleted_at = 0`。
- 源表 `enabled = 0` 的行 MUST NOT 自动标记 `deleted_at > 0`（业务语义不同）。

**5. 布尔类型**

- SQLite `INTEGER` 0/1 → MySQL `TINYINT(1)` 0/1，原样搬。

#### Scenario: 老 skills 格式归一
- **WHEN** 源 `agent_configs.skills = '[{"id":"a","mode":"auto"},{"id":"b"}]'`
- **THEN** `agent_skill_bindings` MUST 插入两行：`(skill_id=a, mode=auto, position=0)` 与 `(skill_id=b, mode='', position=1)`

#### Scenario: TEXT 时间戳解析失败
- **WHEN** 源 `agent_personas.created_at = 'not-a-timestamp'`
- **THEN** CLI MUST 终止；错误消息 MUST 包含 persona id 与原始值

#### Scenario: activation_order 回填
- **WHEN** 源 3 行 `session_active_skills` 同 `session_id` 且 `activation_order` 皆为 0
- **THEN** 目标 MUST 按原 `rowid ASC` 赋值为 0、1、2

---

### Requirement: 事务与批处理

每张目标表内的写入 MUST 按 `--batch-size` 分批，每批一个事务。JSON 拆分出的子表 MUST 在同一事务中插入，保证父行存在时子行也存在（或一起失败）。

#### Scenario: 批内失败回滚
- **WHEN** 某批次第 300 行违反唯一键约束
- **THEN** 整批事务 MUST 回滚；CLI 退出；源数据未被改动

#### Scenario: 父子原子
- **WHEN** `messages` 某行插入成功但其 `message_tool_calls` 展开时 provider 返回超时
- **THEN** 当前批事务 MUST 回滚，父行与子行都不存在于目标；MUST NOT 出现 message 有记录但 tool_calls 缺失的中间态

---

### Requirement: 幂等重跑

当 `--truncate-before` 设置时，CLI MUST 在每个 scope 开始时按目标依赖倒序 `TRUNCATE` 所有目标表（包括拆分出来的子表），然后重新迁移。允许对同一 scope 无限次重跑。

#### Scenario: 重跑后结果等价
- **WHEN** 首次迁移成功后再次执行 `--truncate-before --scope agent`
- **THEN** 目标表最终行数 MUST 与首次相同

---

### Requirement: 迁移前置检查

CLI 启动后、开始迁移前 MUST 执行下列校验：

1. SQLite 源文件可读且能 `sqlite3_open`。
2. MySQL DSN 可连；权限足够（能 `CREATE / DROP / TRUNCATE / INSERT`）。
3. 目标 database 中 goose migrations MUST 已经推到最新版本（通过查询 `goose_db_version` 表判断）。
4. 孤儿行检查：例如 `agent_configs.model_id NOT IN (SELECT id FROM models)` 为 0；若 > 0 打印清单并退出（除非 `--allow-orphans` 显式指定，清洗时 orphan 置为 NULL/空）。

任一检查失败 MUST 阻止迁移开始。

#### Scenario: goose 未跑
- **WHEN** `moli_agent.goose_db_version` 表不存在
- **THEN** CLI MUST 报错退出，提示先启动一次 agent 二进制触发 goose.Up

#### Scenario: 孤儿行检查
- **WHEN** SQLite 中有 `agent_configs.model_id = 'm1'` 但 `models` 中无 `id = 'm1'`
- **THEN** CLI MUST 报错退出（默认），或在 `--allow-orphans` 下把 `model_id` 置为空并记录日志

---

### Requirement: 部署契约

部署流程 MUST 遵循：

1. 预先在目标 MySQL 建好 `moli_agent` / `moli_qiwei`（`utf8mb4_bin`）。
2. 启动一次 `agent` 与 `channel-qiwei` 二进制（或用 goose CLI）触发 migration 建表。
3. 停止所有业务进程。
4. 运行 `migrate-sqlite-to-mysql --scope all --sqlite-agent PATH --sqlite-qiwei PATH --mysql-dsn-agent ... --mysql-dsn-qiwei ...`。
5. 对账通过后启动业务进程（env 已指向 MySQL）。

#### Scenario: 业务进程未停
- **WHEN** 迁移工具启动时检测到源 SQLite 有 WAL 文件正在被其他进程写入
- **THEN** CLI MUST 告警并退出（避免读到半完成事务）

#### Scenario: 冒烟检查清单
- **WHEN** 部署完成
- **THEN** 运维 MUST 执行：企微消息 round-trip、webui 对话一次、delayed task 创建一次、admin 前端 list 一次；任一失败 MUST 触发回滚流程
