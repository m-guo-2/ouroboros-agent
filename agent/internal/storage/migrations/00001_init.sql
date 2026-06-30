-- +goose Up
-- +goose StatementBegin

-- Conventions (see openspec/changes/migrate-sqlite-to-mysql):
--   * Business IDs:       VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin
--   * Short enums:        VARCHAR(32)
--   * External / trace:   VARCHAR(128)
--   * Business text:      TEXT / MEDIUMTEXT (utf8mb4)
--   * Timestamps:         BIGINT NOT NULL DEFAULT 0   (UTC epoch ms, 0 = unset)
--   * Soft delete column: deleted_at BIGINT NOT NULL DEFAULT 0
--   * All UNIQUE KEYs end with deleted_at on soft-delete tables.
--   * No FOREIGN KEY constraints; integrity enforced at the repo layer.

-- ---------------------------------------------------------------
-- settings: global key/value store (no soft-delete)
-- ---------------------------------------------------------------
CREATE TABLE settings (
    `key`        VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    value        TEXT,
    updated_at   BIGINT NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- users: people / bots the agent converses with
-- ---------------------------------------------------------------
CREATE TABLE users (
    id           VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    name         VARCHAR(256) NOT NULL DEFAULT '',
    type         VARCHAR(32)  NOT NULL DEFAULT 'human',
    avatar_url   VARCHAR(1024) NOT NULL DEFAULT '',
    metadata     MEDIUMTEXT   NOT NULL DEFAULT ('{}'),
    created_at   BIGINT       NOT NULL DEFAULT 0,
    updated_at   BIGINT       NOT NULL DEFAULT 0,
    deleted_at   BIGINT       NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- user_channels: 1:N mapping from user to external channel accounts
-- ---------------------------------------------------------------
CREATE TABLE user_channels (
    id               BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id          VARCHAR(64)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    channel_type     VARCHAR(32)  NOT NULL,
    channel_user_id  VARCHAR(128) NOT NULL,
    display_name     VARCHAR(256) NOT NULL DEFAULT '',
    channel_meta     MEDIUMTEXT   NOT NULL DEFAULT ('{}'),
    created_at       BIGINT       NOT NULL DEFAULT 0,
    deleted_at       BIGINT       NOT NULL DEFAULT 0,
    UNIQUE KEY uk_user_channels (channel_type, channel_user_id, deleted_at),
    KEY idx_user_channels_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- user_memory: per-(agent,user) summary memory
-- ---------------------------------------------------------------
CREATE TABLE user_memory (
    id          VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    user_id     VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    agent_id    VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    summary     MEDIUMTEXT NOT NULL DEFAULT (''),
    updated_at  BIGINT     NOT NULL DEFAULT 0,
    deleted_at  BIGINT     NOT NULL DEFAULT 0,
    UNIQUE KEY uk_user_memory (agent_id, user_id, deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- user_memory_facts: structured facts extracted from conversations
-- (no soft-delete: TTL-based expiry)
-- ---------------------------------------------------------------
CREATE TABLE user_memory_facts (
    id                 BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    user_id            VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    agent_id           VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    category           VARCHAR(64) NOT NULL,
    fact               TEXT NOT NULL,
    source_channel     VARCHAR(32) NOT NULL DEFAULT '',
    source_session_id  VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    created_at         BIGINT NOT NULL DEFAULT 0,
    expires_at         BIGINT NOT NULL DEFAULT 0,
    KEY idx_memory_facts_agent_user (agent_id, user_id),
    KEY idx_memory_facts_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- processed_messages: inbound de-duplication
-- (no soft-delete: operational TTL)
-- ---------------------------------------------------------------
CREATE TABLE processed_messages (
    channel_message_id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    channel_type       VARCHAR(32) NOT NULL,
    processed_at       BIGINT NOT NULL DEFAULT 0,
    KEY idx_processed_messages_processed_at (processed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- models: provider / model configuration entries (admin-managed)
-- ---------------------------------------------------------------
CREATE TABLE models (
    id           VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    name         VARCHAR(128) NOT NULL,
    provider     VARCHAR(32)  NOT NULL,
    enabled      TINYINT(1)   NOT NULL DEFAULT 1,
    -- TODO: encrypt (tracked separately as security debt)
    api_key      VARCHAR(512) NOT NULL DEFAULT '',
    base_url     VARCHAR(512) NOT NULL DEFAULT '',
    model        VARCHAR(128) NOT NULL,
    max_tokens   INT          NOT NULL DEFAULT 4096,
    temperature  DOUBLE       NOT NULL DEFAULT 0.7,
    created_at   BIGINT       NOT NULL DEFAULT 0,
    updated_at   BIGINT       NOT NULL DEFAULT 0,
    deleted_at   BIGINT       NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- skills: skill metadata (triggers/tools are separate child tables)
-- ---------------------------------------------------------------
CREATE TABLE skills (
    id           VARCHAR(64)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    name         VARCHAR(128) NOT NULL,
    description  TEXT         NOT NULL,
    version      VARCHAR(32)  NOT NULL DEFAULT '1.0.0',
    type         VARCHAR(32)  NOT NULL DEFAULT 'knowledge',
    enabled      TINYINT(1)   NOT NULL DEFAULT 1,
    readme       MEDIUMTEXT   NOT NULL DEFAULT (''),
    metadata     MEDIUMTEXT   NOT NULL DEFAULT ('{}'),
    created_at   BIGINT       NOT NULL DEFAULT 0,
    updated_at   BIGINT       NOT NULL DEFAULT 0,
    deleted_at   BIGINT       NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE skill_triggers (
    id           BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    skill_id     VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    pattern      VARCHAR(256) NOT NULL,
    position     INT NOT NULL DEFAULT 0,
    created_at   BIGINT NOT NULL DEFAULT 0,
    KEY idx_skill_triggers_skill (skill_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE skill_tools (
    id           BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    skill_id     VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    tool_name    VARCHAR(64) NOT NULL,
    position     INT NOT NULL DEFAULT 0,
    created_at   BIGINT NOT NULL DEFAULT 0,
    KEY idx_skill_tools_skill (skill_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- agent_configs: per-agent tunables; JSON children split into
--   agent_skill_bindings / agent_hooks / agent_channels /
--   agent_subagent_models / agent_subagent_skill_bindings.
-- ---------------------------------------------------------------
CREATE TABLE agent_configs (
    id             VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    user_id        VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    display_name   VARCHAR(256) NOT NULL DEFAULT '',
    system_prompt  MEDIUMTEXT NOT NULL DEFAULT (''),
    model_id       VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    provider       VARCHAR(32) NOT NULL DEFAULT '',
    model          VARCHAR(128) NOT NULL DEFAULT '',
    sandbox_template_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'office-worker',
    is_active      TINYINT(1)  NOT NULL DEFAULT 1,
    created_at     BIGINT      NOT NULL DEFAULT 0,
    updated_at     BIGINT      NOT NULL DEFAULT 0,
    deleted_at     BIGINT      NOT NULL DEFAULT 0,
    KEY idx_agent_configs_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE agent_skill_bindings (
    id          VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    agent_id    VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    skill_id    VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    mode        VARCHAR(32) NOT NULL DEFAULT '',
    position    INT NOT NULL DEFAULT 0,
    created_at  BIGINT NOT NULL DEFAULT 0,
    deleted_at  BIGINT NOT NULL DEFAULT 0,
    UNIQUE KEY uk_agent_skill (agent_id, skill_id, deleted_at),
    KEY idx_agent_skill_skill (skill_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE agent_hooks (
    id          BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    agent_id    VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    event       VARCHAR(64) NOT NULL,
    action_type VARCHAR(32) NOT NULL DEFAULT '',
    skill_id    VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    position    INT NOT NULL DEFAULT 0,
    created_at  BIGINT NOT NULL DEFAULT 0,
    KEY idx_agent_hooks_agent (agent_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE agent_channels (
    id                  VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    agent_id            VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    channel_type        VARCHAR(32) NOT NULL,
    channel_identifier  VARCHAR(128) NOT NULL,
    position            INT NOT NULL DEFAULT 0,
    created_at          BIGINT NOT NULL DEFAULT 0,
    deleted_at          BIGINT NOT NULL DEFAULT 0,
    UNIQUE KEY uk_agent_channel (agent_id, channel_type, channel_identifier, deleted_at),
    KEY idx_agent_channels_channel (channel_type, channel_identifier)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE agent_subagent_models (
    id             VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    agent_id       VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    subagent_key   VARCHAR(64) NOT NULL,
    provider       VARCHAR(32) NOT NULL DEFAULT '',
    model          VARCHAR(128) NOT NULL DEFAULT '',
    created_at     BIGINT NOT NULL DEFAULT 0,
    updated_at     BIGINT NOT NULL DEFAULT 0,
    deleted_at     BIGINT NOT NULL DEFAULT 0,
    UNIQUE KEY uk_agent_subagent (agent_id, subagent_key, deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE agent_subagent_skill_bindings (
    id             VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    agent_id       VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    subagent_key   VARCHAR(64) NOT NULL,
    skill_id       VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    position       INT NOT NULL DEFAULT 0,
    created_at     BIGINT NOT NULL DEFAULT 0,
    deleted_at     BIGINT NOT NULL DEFAULT 0,
    UNIQUE KEY uk_agent_subagent_skill (agent_id, subagent_key, skill_id, deleted_at),
    KEY idx_agent_subagent_skill_skill (skill_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- agent_personas: profile overrides that can be assigned to groups.
-- JSON children split into three persona_* tables below.
-- ---------------------------------------------------------------
CREATE TABLE agent_personas (
    id             VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    agent_id       VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    display_name   VARCHAR(256) NOT NULL,
    system_prompt  MEDIUMTEXT   NOT NULL DEFAULT (''),
    provider       VARCHAR(32)  NOT NULL DEFAULT '',
    model          VARCHAR(128) NOT NULL DEFAULT '',
    sandbox_template_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
    created_at     BIGINT       NOT NULL DEFAULT 0,
    updated_at     BIGINT       NOT NULL DEFAULT 0,
    deleted_at     BIGINT       NOT NULL DEFAULT 0,
    KEY idx_agent_personas_agent (agent_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE persona_skill_bindings (
    id          VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    persona_id  VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    skill_id    VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    mode        VARCHAR(32) NOT NULL DEFAULT '',
    position    INT NOT NULL DEFAULT 0,
    created_at  BIGINT NOT NULL DEFAULT 0,
    deleted_at  BIGINT NOT NULL DEFAULT 0,
    UNIQUE KEY uk_persona_skill (persona_id, skill_id, deleted_at),
    KEY idx_persona_skill_skill (skill_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE persona_subagent_models (
    id             VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    persona_id     VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    subagent_key   VARCHAR(64) NOT NULL,
    provider       VARCHAR(32) NOT NULL DEFAULT '',
    model          VARCHAR(128) NOT NULL DEFAULT '',
    created_at     BIGINT NOT NULL DEFAULT 0,
    updated_at     BIGINT NOT NULL DEFAULT 0,
    deleted_at     BIGINT NOT NULL DEFAULT 0,
    UNIQUE KEY uk_persona_subagent (persona_id, subagent_key, deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE persona_subagent_skill_bindings (
    id             VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    persona_id     VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    subagent_key   VARCHAR(64) NOT NULL,
    skill_id       VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    position       INT NOT NULL DEFAULT 0,
    created_at     BIGINT NOT NULL DEFAULT 0,
    deleted_at     BIGINT NOT NULL DEFAULT 0,
    UNIQUE KEY uk_persona_subagent_skill (persona_id, subagent_key, skill_id, deleted_at),
    KEY idx_persona_subagent_skill_skill (skill_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- channel_groups: group metadata discovered from channels
-- ---------------------------------------------------------------
CREATE TABLE channel_groups (
    id                VARCHAR(64)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    agent_id          VARCHAR(64)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    channel           VARCHAR(32)  NOT NULL,
    channel_group_id  VARCHAR(128) NOT NULL,
    group_name        VARCHAR(256) NOT NULL DEFAULT '',
    status            VARCHAR(32)  NOT NULL DEFAULT 'active',
    created_at        BIGINT       NOT NULL DEFAULT 0,
    updated_at        BIGINT       NOT NULL DEFAULT 0,
    deleted_at        BIGINT       NOT NULL DEFAULT 0,
    UNIQUE KEY uk_channel_groups (agent_id, channel, channel_group_id, deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- group_persona_assignments: which persona serves a given group
-- ---------------------------------------------------------------
CREATE TABLE group_persona_assignments (
    id            VARCHAR(64)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    agent_id      VARCHAR(64)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    session_key   VARCHAR(128) NOT NULL,
    group_name    VARCHAR(256) NOT NULL DEFAULT '',
    persona_id    VARCHAR(64)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    sandbox_template_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
    created_at    BIGINT NOT NULL DEFAULT 0,
    updated_at    BIGINT NOT NULL DEFAULT 0,
    deleted_at    BIGINT NOT NULL DEFAULT 0,
    UNIQUE KEY uk_group_assignments (agent_id, session_key, deleted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- agent_sessions: conversation thread root
-- (soft-delete table; `messages` JSON column is dropped - dead column)
-- ---------------------------------------------------------------
CREATE TABLE agent_sessions (
    id                      VARCHAR(64)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    title                   VARCHAR(256) NOT NULL DEFAULT '新对话',
    sdk_session_id          VARCHAR(128) NOT NULL DEFAULT '',
    user_id                 VARCHAR(64)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    agent_id                VARCHAR(64)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    source_channel          VARCHAR(32)  NOT NULL DEFAULT 'webui',
    execution_status        VARCHAR(32)  NOT NULL DEFAULT 'idle',
    mode                    VARCHAR(32)  NOT NULL DEFAULT 'normal',
    channel_name            VARCHAR(256) NOT NULL DEFAULT '',
    channel_conversation_id VARCHAR(128) NOT NULL DEFAULT '',
    session_key             VARCHAR(128) NOT NULL DEFAULT '',
    work_dir                VARCHAR(512) NOT NULL DEFAULT '',
    sandbox_template_id     VARCHAR(64)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'office-worker',
    context                 MEDIUMTEXT   NOT NULL DEFAULT (''),
    event_cursor            BIGINT       NOT NULL DEFAULT 0,
    created_at              BIGINT       NOT NULL DEFAULT 0,
    updated_at              BIGINT       NOT NULL DEFAULT 0,
    deleted_at              BIGINT       NOT NULL DEFAULT 0,
    KEY idx_agent_sessions_agent (agent_id),
    KEY idx_agent_sessions_conv_id (channel_conversation_id),
    KEY idx_agent_sessions_session_key (session_key),
    KEY idx_agent_sessions_agent_key (agent_id, session_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- messages: per-session append-only record.
-- JSON children split into message_tool_calls / message_attachments.
-- (no soft-delete: immutable stream)
-- ---------------------------------------------------------------
CREATE TABLE messages (
    id                   BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    session_id           VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    role                 VARCHAR(32) NOT NULL,
    content              MEDIUMTEXT  NOT NULL DEFAULT (''),
    message_type         VARCHAR(32) NOT NULL DEFAULT 'text',
    channel              VARCHAR(32) NOT NULL DEFAULT '',
    channel_message_id   VARCHAR(128) NOT NULL DEFAULT '',
    reply_to_message_id  VARCHAR(128) NOT NULL DEFAULT '',
    trace_id             VARCHAR(128) NOT NULL DEFAULT '',
    initiator            VARCHAR(32)  NOT NULL DEFAULT '',
    sender_name          VARCHAR(256) NOT NULL DEFAULT '',
    sender_id            VARCHAR(128) NOT NULL DEFAULT '',
    channel_meta         MEDIUMTEXT   NOT NULL DEFAULT ('{}'),
    status               VARCHAR(32)  NOT NULL DEFAULT 'sent',
    created_at           BIGINT       NOT NULL DEFAULT 0,
    KEY idx_messages_session (session_id),
    KEY idx_messages_trace (trace_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE message_tool_calls (
    id              BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    message_id      BIGINT NOT NULL,
    seq             INT NOT NULL DEFAULT 0,
    tool_call_id    VARCHAR(128) NOT NULL DEFAULT '',
    tool_name       VARCHAR(64)  NOT NULL,
    arguments_json  MEDIUMTEXT   NOT NULL DEFAULT ('{}'),
    result_json     MEDIUMTEXT   NOT NULL DEFAULT (''),
    status          VARCHAR(32)  NOT NULL DEFAULT 'pending',
    created_at      BIGINT       NOT NULL DEFAULT 0,
    KEY idx_mtc_message (message_id),
    KEY idx_mtc_tool_name (tool_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE message_attachments (
    id              BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    message_id      BIGINT NOT NULL,
    seq             INT NOT NULL DEFAULT 0,
    attachment_id   VARCHAR(128) NOT NULL DEFAULT '',
    kind            VARCHAR(32)  NOT NULL DEFAULT '',
    resource_uri    VARCHAR(1024) NOT NULL DEFAULT '',
    display_name    VARCHAR(256) NOT NULL DEFAULT '',
    mime_type       VARCHAR(128) NOT NULL DEFAULT '',
    source_type     VARCHAR(32)  NOT NULL DEFAULT '',
    created_at      BIGINT       NOT NULL DEFAULT 0,
    KEY idx_ma_message (message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- session_events: append-only sequence index into messages
-- (no soft-delete: immutable cursor stream)
-- ---------------------------------------------------------------
CREATE TABLE session_events (
    seq         BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    session_id  VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    message_id  BIGINT NOT NULL,
    KEY idx_session_events_session_seq (session_id, seq)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- session_facts: short-lived facts scoped to a single session
-- ---------------------------------------------------------------
CREATE TABLE session_facts (
    id          BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    session_id  VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    fact        TEXT NOT NULL,
    category    VARCHAR(32) NOT NULL DEFAULT 'general',
    created_at  BIGINT NOT NULL DEFAULT 0,
    KEY idx_session_facts_session (session_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- session_active_skills: per-session dynamically-activated skills
-- (no soft-delete: turn-scoped lifecycle)
-- ---------------------------------------------------------------
CREATE TABLE session_active_skills (
    id                VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    session_id        VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    skill_id          VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    source            VARCHAR(32) NOT NULL DEFAULT 'hook',
    activation_order  BIGINT NOT NULL DEFAULT 0,
    created_at        BIGINT NOT NULL DEFAULT 0,
    UNIQUE KEY uk_session_active_skills (session_id, skill_id),
    KEY idx_session_active_skills_session (session_id),
    KEY idx_session_active_skills_session_order (session_id, activation_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- context_compactions: compaction run headers
-- ---------------------------------------------------------------
CREATE TABLE context_compactions (
    id                      BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    session_id              VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    summary                 MEDIUMTEXT NOT NULL DEFAULT (''),
    archived_before_time    BIGINT NOT NULL DEFAULT 0,
    archived_message_count  INT    NOT NULL DEFAULT 0,
    token_count_before      INT    NOT NULL DEFAULT 0,
    token_count_after       INT    NOT NULL DEFAULT 0,
    compact_model           VARCHAR(128) NOT NULL DEFAULT '',
    created_at              BIGINT NOT NULL DEFAULT 0,
    KEY idx_compactions_session (session_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- context_compaction_archives: archive header.
-- Archived messages moved to dedicated child table.
-- ---------------------------------------------------------------
CREATE TABLE context_compaction_archives (
    id              BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    session_id      VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    compaction_id   BIGINT NOT NULL,
    message_count   INT NOT NULL DEFAULT 0,
    created_at      BIGINT NOT NULL DEFAULT 0,
    KEY idx_compaction_archives_session (session_id),
    KEY idx_compaction_archives_compaction (compaction_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE context_compaction_archived_messages (
    id                    BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    archive_id            BIGINT NOT NULL,
    seq                   INT NOT NULL DEFAULT 0,
    original_message_id   BIGINT NOT NULL DEFAULT 0,
    role                  VARCHAR(32) NOT NULL,
    content               MEDIUMTEXT NOT NULL DEFAULT (''),
    message_type          VARCHAR(32) NOT NULL DEFAULT 'text',
    created_at            BIGINT NOT NULL DEFAULT 0,
    KEY idx_ccam_archive (archive_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- delayed_tasks: proactive task queue
-- (no soft-delete: status machine)
-- ---------------------------------------------------------------
CREATE TABLE delayed_tasks (
    id                       BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    session_id               VARCHAR(64)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    agent_id                 VARCHAR(64)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    user_id                  VARCHAR(64)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    channel                  VARCHAR(32)  NOT NULL DEFAULT '',
    channel_user_id          VARCHAR(128) NOT NULL DEFAULT '',
    channel_conversation_id  VARCHAR(128) NOT NULL DEFAULT '',
    task                     TEXT         NOT NULL,
    execute_at               BIGINT       NOT NULL,
    status                   VARCHAR(32)  NOT NULL DEFAULT 'pending',
    created_at               BIGINT       NOT NULL DEFAULT 0,
    updated_at               BIGINT       NOT NULL DEFAULT 0,
    KEY idx_delayed_tasks_status_time (status, execute_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS delayed_tasks;
DROP TABLE IF EXISTS context_compaction_archived_messages;
DROP TABLE IF EXISTS context_compaction_archives;
DROP TABLE IF EXISTS context_compactions;
DROP TABLE IF EXISTS session_active_skills;
DROP TABLE IF EXISTS session_facts;
DROP TABLE IF EXISTS session_events;
DROP TABLE IF EXISTS message_attachments;
DROP TABLE IF EXISTS message_tool_calls;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS agent_sessions;
DROP TABLE IF EXISTS group_persona_assignments;
DROP TABLE IF EXISTS channel_groups;
DROP TABLE IF EXISTS persona_subagent_skill_bindings;
DROP TABLE IF EXISTS persona_subagent_models;
DROP TABLE IF EXISTS persona_skill_bindings;
DROP TABLE IF EXISTS agent_personas;
DROP TABLE IF EXISTS agent_subagent_skill_bindings;
DROP TABLE IF EXISTS agent_subagent_models;
DROP TABLE IF EXISTS agent_channels;
DROP TABLE IF EXISTS agent_hooks;
DROP TABLE IF EXISTS agent_skill_bindings;
DROP TABLE IF EXISTS agent_configs;
DROP TABLE IF EXISTS skill_tools;
DROP TABLE IF EXISTS skill_triggers;
DROP TABLE IF EXISTS skills;
DROP TABLE IF EXISTS models;
DROP TABLE IF EXISTS processed_messages;
DROP TABLE IF EXISTS user_memory_facts;
DROP TABLE IF EXISTS user_memory;
DROP TABLE IF EXISTS user_channels;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS settings;
-- +goose StatementEnd
