## ADDED Requirements

本 spec 定义具体的表级改造：JSON 列拆分的 14 张新表、被删除的列/索引、类型变更、新增索引。配合 `mysql-persistence` spec 中的通用原则（去 FK、软删、epoch ms、utf8mb4_bin）执行。

---

### Requirement: 从 `agent_configs.skills` 拆出 `agent_skill_bindings`

`agent_skill_bindings` MUST 成为 agent 与 skill 之间 m2m 绑定的唯一真源。`agent_configs.skills` JSON 列 MUST 在迁移后删除。

**结构**（列名 / 类型 / 约束）：

- `id VARCHAR(64) PRIMARY KEY`（`prefixedID("asb")`）
- `agent_id VARCHAR(64) NOT NULL`
- `skill_id VARCHAR(64) NOT NULL`
- `mode VARCHAR(32) NOT NULL DEFAULT ''`（承接老 `[{"id":"x","mode":"y"}]` 的 mode 字段；新格式 `["x"]` 对应空串）
- `position INT NOT NULL DEFAULT 0`（UI 排序）
- `created_at BIGINT NOT NULL DEFAULT 0`
- `deleted_at BIGINT NOT NULL DEFAULT 0`
- UNIQUE KEY `uk_agent_skill (agent_id, skill_id, deleted_at)`
- INDEX `idx_skill_id (skill_id)`（反查"哪些 agent 用了 skill X"）

#### Scenario: 迁移后老列不存在
- **WHEN** 在 `moli_agent` 执行 `SHOW COLUMNS FROM agent_configs`
- **THEN** 输出 MUST NOT 包含列 `skills`

#### Scenario: 老格式 mode 保留
- **WHEN** 源 `agent_configs.skills = '[{"id":"skill-a","mode":"auto"}]'`
- **THEN** 目标 `agent_skill_bindings` MUST 有一行 `(agent_id, "skill-a", mode="auto", position=0)`

---

### Requirement: 从 `agent_configs.hooks` 拆出 `agent_hooks`

`agent_hooks` MUST 存放 agent 配置的钩子列表，作为父表的有序数组属性（非 m2m，**不软删**）。父表更新 hooks 时 MUST 在事务内 `DELETE FROM agent_hooks WHERE agent_id=? ; INSERT ...`。`agent_configs.hooks` JSON 列 MUST 在迁移后删除。

**结构**：
- `id BIGINT AUTO_INCREMENT PRIMARY KEY`
- `agent_id VARCHAR(64) NOT NULL`
- `event VARCHAR(32) NOT NULL`
- `command TEXT NOT NULL`
- `position INT NOT NULL DEFAULT 0`
- `created_at BIGINT NOT NULL DEFAULT 0`
- INDEX `idx_agent_hooks_agent (agent_id)`

#### Scenario: 迁移后老列不存在
- **WHEN** `SHOW COLUMNS FROM agent_configs`
- **THEN** 输出 MUST NOT 包含列 `hooks`

---

### Requirement: 从 `agent_configs.channels` 拆出 `agent_channels`

`agent_channels` MUST 承载 agent 的 channel 绑定列表，作为 m2m 关系（**软删**）。`agent_configs.channels` JSON 列 MUST 在迁移后删除。

**结构**：
- `id VARCHAR(64) PRIMARY KEY`
- `agent_id VARCHAR(64) NOT NULL`
- `channel_type VARCHAR(32) NOT NULL`
- `channel_identifier VARCHAR(128) NOT NULL`
- `position INT NOT NULL DEFAULT 0`
- `created_at BIGINT NOT NULL DEFAULT 0`
- `deleted_at BIGINT NOT NULL DEFAULT 0`
- UNIQUE KEY `uk_agent_channel (agent_id, channel_type, channel_identifier, deleted_at)`
- INDEX `idx_agent_channels_channel (channel_type, channel_identifier)`

#### Scenario: 绑定查找
- **WHEN** 业务按 `(channel_type="qiwei", channel_identifier="guid-abc")` 查 agent
- **THEN** 查询 MUST 命中 `idx_agent_channels_channel` 索引

---

### Requirement: 从 `agent_configs.subagent_models` 拆出 `agent_subagent_models`

`agent_subagent_models` MUST 替代 JSON 对象 `{subagentKey: {provider, model}}`。**软删**。`agent_configs.subagent_models` JSON 列 MUST 在迁移后删除。

**结构**：
- `id VARCHAR(64) PRIMARY KEY`
- `agent_id VARCHAR(64) NOT NULL`
- `subagent_key VARCHAR(64) NOT NULL`
- `provider VARCHAR(32) NOT NULL DEFAULT ''`
- `model VARCHAR(64) NOT NULL DEFAULT ''`
- `created_at BIGINT NOT NULL DEFAULT 0`
- `updated_at BIGINT NOT NULL DEFAULT 0`
- `deleted_at BIGINT NOT NULL DEFAULT 0`
- UNIQUE KEY `uk_agent_subagent (agent_id, subagent_key, deleted_at)`

#### Scenario: 迁移后列不存在
- **WHEN** `SHOW COLUMNS FROM agent_configs`
- **THEN** 输出 MUST NOT 包含 `subagent_models`

---

### Requirement: 从 `agent_configs.subagent_skills` 拆出 `agent_subagent_skill_bindings`

存放 `{subagentKey: [skillID]}` 的拆表形态。**软删**。`agent_configs.subagent_skills` JSON 列 MUST 在迁移后删除。

**结构**：
- `id VARCHAR(64) PRIMARY KEY`
- `agent_id VARCHAR(64) NOT NULL`
- `subagent_key VARCHAR(64) NOT NULL`
- `skill_id VARCHAR(64) NOT NULL`
- `position INT NOT NULL DEFAULT 0`
- `created_at BIGINT NOT NULL DEFAULT 0`
- `deleted_at BIGINT NOT NULL DEFAULT 0`
- UNIQUE KEY `uk_agent_subagent_skill (agent_id, subagent_key, skill_id, deleted_at)`

#### Scenario: 老格式解析
- **WHEN** 源 `subagent_skills = '{"coder":["skill-a","skill-b"]}'`
- **THEN** 目标 MUST 插入两行：`(coder, skill-a, position=0)` 和 `(coder, skill-b, position=1)`

---

### Requirement: 从 `agent_personas` 拆出三张 persona 绑定子表

`agent_personas.skills / subagent_models / subagent_skills` 三列 MUST 分别拆为 `persona_skill_bindings / persona_subagent_models / persona_subagent_skill_bindings`，结构与 agent 对应三表一致（把 `agent_id` 换为 `persona_id`）。三表均 **软删**。三列在迁移后 MUST 从 `agent_personas` 删除。

#### Scenario: 迁移后老列不存在
- **WHEN** `SHOW COLUMNS FROM agent_personas`
- **THEN** 输出 MUST NOT 包含 `skills / subagent_models / subagent_skills`

---

### Requirement: 从 `messages.tool_calls` 拆出 `message_tool_calls`

`message_tool_calls` MUST 存放 LLM 生成的工具调用，作为 `messages` 的数组属性（**不软删**，因为 message 本身不可变）。`messages.tool_calls` JSON 列 MUST 在迁移后删除。

**结构**：
- `id BIGINT AUTO_INCREMENT PRIMARY KEY`
- `message_id BIGINT NOT NULL`
- `seq INT NOT NULL DEFAULT 0`
- `tool_call_id VARCHAR(128) NOT NULL DEFAULT ''`（provider 端 ID）
- `tool_name VARCHAR(64) NOT NULL`
- `arguments_json MEDIUMTEXT NOT NULL`（工具参数按 tool 结构不同，保留为文本）
- `result_json MEDIUMTEXT NOT NULL`
- `status VARCHAR(32) NOT NULL DEFAULT 'pending'`（pending / success / error）
- `created_at BIGINT NOT NULL DEFAULT 0`
- INDEX `idx_mtc_message (message_id)`
- INDEX `idx_mtc_tool_name (tool_name)`（"按工具统计调用" 类查询）

#### Scenario: 迁移后老列不存在
- **WHEN** `SHOW COLUMNS FROM messages`
- **THEN** 输出 MUST NOT 包含 `tool_calls`

#### Scenario: 工具统计查询
- **WHEN** admin 查"某 agent 在某时段的 read_file 工具调用次数"
- **THEN** 查询 MUST 能用 `idx_mtc_tool_name` 而非全表扫描

---

### Requirement: 从 `messages.attachments_json` 拆出 `message_attachments`

`message_attachments` MUST 存放消息附件列表（**不软删**）。`messages.attachments_json` JSON 列 MUST 在迁移后删除。

**结构**：
- `id BIGINT AUTO_INCREMENT PRIMARY KEY`
- `message_id BIGINT NOT NULL`
- `seq INT NOT NULL DEFAULT 0`
- `kind VARCHAR(32) NOT NULL`（image / file / audio / video）
- `url VARCHAR(1024) NOT NULL DEFAULT ''`
- `filename VARCHAR(256) NOT NULL DEFAULT ''`
- `mime_type VARCHAR(64) NOT NULL DEFAULT ''`
- `size_bytes BIGINT NOT NULL DEFAULT 0`
- `created_at BIGINT NOT NULL DEFAULT 0`
- INDEX `idx_ma_message (message_id)`

#### Scenario: 迁移后老列不存在
- **WHEN** `SHOW COLUMNS FROM messages`
- **THEN** 输出 MUST NOT 包含 `attachments_json`

---

### Requirement: 从 `context_compaction_archives.archived_messages` 拆出 `context_compaction_archived_messages`

`context_compaction_archived_messages` MUST 存放压缩前归档的消息行，与 `context_compaction_archives` 1:N（**不软删**，归档本身不可变）。`context_compaction_archives.archived_messages` 列 MUST 在迁移后删除。

**结构**：
- `id BIGINT AUTO_INCREMENT PRIMARY KEY`
- `archive_id BIGINT NOT NULL`（指向 `context_compaction_archives.id`）
- `seq INT NOT NULL DEFAULT 0`
- `original_message_id BIGINT NOT NULL DEFAULT 0`
- `role VARCHAR(32) NOT NULL`
- `content MEDIUMTEXT NOT NULL`
- `message_type VARCHAR(32) NOT NULL DEFAULT 'text'`
- `created_at BIGINT NOT NULL DEFAULT 0`
- INDEX `idx_ccam_archive (archive_id)`

#### Scenario: 迁移后老列不存在
- **WHEN** `SHOW COLUMNS FROM context_compaction_archives`
- **THEN** 输出 MUST NOT 包含 `archived_messages`

---

### Requirement: 从 `skills.triggers` / `skills.tools` 拆出 `skill_triggers` / `skill_tools`

两张新表存放 skill 的触发器模式和允许的工具列表，作为 skills 的数组属性（**不软删**；skill 更新 triggers / tools 时在事务内 DELETE+INSERT）。`skills.triggers` 和 `skills.tools` 列 MUST 在迁移后删除。

**`skill_triggers` 结构**：
- `id BIGINT AUTO_INCREMENT PRIMARY KEY`
- `skill_id VARCHAR(64) NOT NULL`
- `pattern VARCHAR(256) NOT NULL`
- `position INT NOT NULL DEFAULT 0`
- `created_at BIGINT NOT NULL DEFAULT 0`
- INDEX `idx_skill_triggers_skill (skill_id)`

**`skill_tools` 结构**：
- `id BIGINT AUTO_INCREMENT PRIMARY KEY`
- `skill_id VARCHAR(64) NOT NULL`
- `tool_name VARCHAR(64) NOT NULL`
- `position INT NOT NULL DEFAULT 0`
- `created_at BIGINT NOT NULL DEFAULT 0`
- INDEX `idx_skill_tools_skill (skill_id)`

#### Scenario: 迁移后老列不存在
- **WHEN** `SHOW COLUMNS FROM skills`
- **THEN** 输出 MUST NOT 包含 `triggers / tools`

---

### Requirement: 从 `qiwei_contacts.follow_user_json` 拆出 `qiwei_contact_followers`

`qiwei_contact_followers` MUST 存放联系人的"添加者" user_id 列表，作为 qiwei_contacts 的数组属性（**不软删**；contact sync 时整替）。`qiwei_contacts.follow_user_json` 列 MUST 在迁移后删除。

**结构**：
- `id BIGINT AUTO_INCREMENT PRIMARY KEY`
- `account_id VARCHAR(64) NOT NULL`
- `user_id VARCHAR(64) NOT NULL`（被添加的联系人 user_id）
- `follow_user_id VARCHAR(64) NOT NULL`（添加者 user_id）
- `position INT NOT NULL DEFAULT 0`
- `created_at BIGINT NOT NULL DEFAULT 0`
- UNIQUE KEY `uk_contact_follower (account_id, user_id, follow_user_id)`
- INDEX `idx_contact_follower_account_user (account_id, user_id)`

#### Scenario: 迁移后老列不存在
- **WHEN** `SHOW COLUMNS FROM qiwei_contacts`
- **THEN** 输出 MUST NOT 包含 `follow_user_json`

---

### Requirement: 软删列覆盖所有实体 / m2m 表

下列表 MUST 包含 `deleted_at BIGINT NOT NULL DEFAULT 0` 列，且所有 UNIQUE KEY MUST 把 `deleted_at` 追加为最后一列：

**实体表（16）**：`users / agent_configs / agent_personas / agent_sessions / skills / models / channel_groups / group_persona_assignments / user_channels / user_memory / qiwei_accounts / qiwei_contacts / qiwei_rooms / qiwei_room_members / qiwei_identity_links / qiwei_known_rooms`

**m2m 绑定表（7）**：`agent_skill_bindings / agent_channels / agent_subagent_models / agent_subagent_skill_bindings / persona_skill_bindings / persona_subagent_models / persona_subagent_skill_bindings`

下列表 MUST NOT 包含 `deleted_at` 列（状态机 / 不可变流水 / 数组属性 / TTL）：

**不软删**：`settings / messages / session_events / session_facts / context_compactions / context_compaction_archives / processed_messages / delayed_tasks / session_active_skills / user_memory_facts / agent_hooks / skill_triggers / skill_tools / message_tool_calls / message_attachments / qiwei_contact_followers / context_compaction_archived_messages`

#### Scenario: 实体表含软删列
- **WHEN** 在 `moli_agent` 执行 `SHOW COLUMNS FROM users`
- **THEN** 输出 MUST 包含 `deleted_at BIGINT NOT NULL DEFAULT 0`

#### Scenario: 消息表不含软删列
- **WHEN** `SHOW COLUMNS FROM messages`
- **THEN** 输出 MUST NOT 包含 `deleted_at`

#### Scenario: 唯一键含软删
- **WHEN** `SHOW CREATE TABLE user_channels`
- **THEN** 唯一键定义 MUST 为 `UNIQUE KEY ... (channel_type, channel_user_id, deleted_at)`

---

### Requirement: TEXT 时间戳列重构为 BIGINT

`agent_personas.created_at / updated_at` 与 `group_persona_assignments.created_at / updated_at` MUST 从 `TEXT DEFAULT CURRENT_TIMESTAMP` 改为 `BIGINT NOT NULL DEFAULT 0`（UTC epoch ms）。迁移工具 MUST 将源 `YYYY-MM-DD HH:MM:SS` 字符串按 UTC 解析为 ms；空字符串 MUST 转为 `0`。

#### Scenario: Schema 类型检查
- **WHEN** `SHOW COLUMNS FROM agent_personas`
- **THEN** `created_at / updated_at` 的 Type 列 MUST 为 `bigint`

---

### Requirement: 死列与死代码移除

下列列 / 索引 / 代码 MUST 在本次迁移中移除：

**删除的列**：
- `agent_sessions.messages`（已 grep 确认无 SQL 读写）

**删除的索引**：
- `idx_agent_sessions_exec_status`（低基数单列索引；保留 `(agent_id, session_key)` 复合即可）
- `idx_qiwei_contacts_name_lookup`（4 列文本复合无意义）

**删除的运行时代码**：
- `agent/internal/storage/db.go::migrateSkillBindingsToIDs`（迁移工具一次性清洗）
- `agent/internal/storage/db.go::backfillSessionActiveSkillOrder`（同上）
- `agent/cmd/migrate-timestamps/main.go`（SQLite 一次性工具）

#### Scenario: 死列不存在
- **WHEN** `SHOW COLUMNS FROM agent_sessions`
- **THEN** 输出 MUST NOT 包含 `messages`

#### Scenario: 死代码不存在
- **WHEN** 在 `agent/internal/storage/` 下搜索符号 `migrateSkillBindingsToIDs`
- **THEN** 结果 MUST 为空

---

### Requirement: 新增索引

下列索引 MUST 在迁移后的 schema 中存在：

- `messages.INDEX idx_messages_trace (trace_id)` —— trace 回放按 trace_id 查
- `user_memory_facts.INDEX idx_user_memory_facts_expires (expires_at)` —— 过期清理
- `processed_messages.INDEX idx_processed_messages_processed_at (processed_at)` —— 过期清理
- `qiwei_contacts`：用 4 个独立单列索引替代原 4 列复合索引：
  - `INDEX idx_contacts_nickname (account_id, nickname)`
  - `INDEX idx_contacts_real_name (account_id, real_name)`
  - `INDEX idx_contacts_alias (account_id, alias)`
  - `INDEX idx_contacts_remark (account_id, remark)`

#### Scenario: 新增索引存在
- **WHEN** `SHOW INDEX FROM messages`
- **THEN** 输出 MUST 包含 `idx_messages_trace`

---

### Requirement: 敏感字段 TODO 标注

`models.api_key` 和 `qiwei_accounts.token` MUST 在 `00001_init.sql` 中以 SQL 注释方式标注 `-- TODO: encrypt (tracked separately)`。本次迁移 MUST NOT 修改这两列的存储形式。

#### Scenario: 注释存在
- **WHEN** 阅读 `agent/internal/storage/migrations/00001_init.sql`
- **THEN** `models.api_key` 定义上方或同行 MUST 有 `TODO: encrypt` 字样

#### Scenario: 列仍为明文字符串
- **WHEN** 迁移完成后读 `models.api_key`
- **THEN** 值 MUST 与源 SQLite 的值一致（未加密）
