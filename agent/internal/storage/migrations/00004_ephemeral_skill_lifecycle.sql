-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS session_participants (
    id                     BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    session_id              VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    channel_user_id         VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    user_id                 VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    first_seen_message_id   BIGINT NOT NULL DEFAULT 0,
    first_seen_seq          BIGINT NOT NULL DEFAULT 0,
    created_at              BIGINT NOT NULL DEFAULT 0,
    updated_at              BIGINT NOT NULL DEFAULT 0,
    UNIQUE KEY uk_session_participant (session_id, channel_user_id),
    KEY idx_session_participants_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS agent_user_seen (
    agent_id       VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    user_id        VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    first_seen_at  BIGINT NOT NULL DEFAULT 0,
    last_seen_at   BIGINT NOT NULL DEFAULT 0,
    seen_count     BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (agent_id, user_id),
    KEY idx_agent_user_seen_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

ALTER TABLE agent_hooks
    ADD COLUMN scope_type VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN expires_after_events INT NOT NULL DEFAULT 0;

ALTER TABLE session_active_skills
    ADD COLUMN source_event VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN scope_type VARCHAR(32) NOT NULL DEFAULT 'session',
    ADD COLUMN scope_key VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    ADD COLUMN status VARCHAR(32) NOT NULL DEFAULT 'active',
    ADD COLUMN activated_at_seq BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN expires_after_events INT NOT NULL DEFAULT 0,
    ADD COLUMN completed_at BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN completion_reason VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN updated_at BIGINT NOT NULL DEFAULT 0;

ALTER TABLE session_active_skills DROP INDEX uk_session_active_skills;
ALTER TABLE session_active_skills ADD KEY idx_session_active_skills_scope (session_id, skill_id, scope_type, scope_key, status);

UPDATE session_active_skills
SET source_event = source,
    updated_at = created_at
WHERE updated_at = 0;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE session_active_skills DROP INDEX idx_session_active_skills_scope;
ALTER TABLE session_active_skills ADD UNIQUE KEY uk_session_active_skills (session_id, skill_id);
ALTER TABLE session_active_skills
    DROP COLUMN source_event,
    DROP COLUMN scope_type,
    DROP COLUMN scope_key,
    DROP COLUMN status,
    DROP COLUMN activated_at_seq,
    DROP COLUMN expires_after_events,
    DROP COLUMN completed_at,
    DROP COLUMN completion_reason,
    DROP COLUMN updated_at;

ALTER TABLE agent_hooks
    DROP COLUMN scope_type,
    DROP COLUMN expires_after_events;

DROP TABLE IF EXISTS agent_user_seen;
DROP TABLE IF EXISTS session_participants;

-- +goose StatementEnd
