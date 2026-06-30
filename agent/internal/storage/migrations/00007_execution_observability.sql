-- +goose Up
-- +goose StatementBegin

ALTER TABLE messages
    ADD COLUMN execution_id VARCHAR(128) NOT NULL DEFAULT '' AFTER trace_id,
    ADD COLUMN processing_status VARCHAR(32) NOT NULL DEFAULT '' AFTER status,
    ADD COLUMN processing_outcome VARCHAR(32) NOT NULL DEFAULT '' AFTER processing_status,
    ADD KEY idx_messages_execution (execution_id),
    ADD KEY idx_messages_processing_created (processing_status, created_at);

CREATE TABLE agent_executions (
    id                  VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    session_id          VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    agent_id            VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    user_id             VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    channel             VARCHAR(32) NOT NULL DEFAULT '',
    provider            VARCHAR(32) NOT NULL DEFAULT '',
    model               VARCHAR(128) NOT NULL DEFAULT '',
    status              VARCHAR(32) NOT NULL DEFAULT 'running',
    delivery_status     VARCHAR(32) NOT NULL DEFAULT 'not_started',
    failure_stage       VARCHAR(32) NOT NULL DEFAULT '',
    error_message       VARCHAR(1024) NOT NULL DEFAULT '',
    request_count       INT NOT NULL DEFAULT 0,
    llm_call_count      INT NOT NULL DEFAULT 0,
    tool_call_count     INT NOT NULL DEFAULT 0,
    tool_failure_count  INT NOT NULL DEFAULT 0,
    input_tokens        BIGINT NOT NULL DEFAULT 0,
    output_tokens       BIGINT NOT NULL DEFAULT 0,
    total_cost_usd      DECIMAL(18,8) NOT NULL DEFAULT 0,
    cost_known          TINYINT(1) NOT NULL DEFAULT 1,
    queued_at           BIGINT NOT NULL DEFAULT 0,
    started_at          BIGINT NOT NULL DEFAULT 0,
    completed_at        BIGINT NOT NULL DEFAULT 0,
    replied_at          BIGINT NOT NULL DEFAULT 0,
    duration_ms         BIGINT NOT NULL DEFAULT 0,
    llm_duration_ms     BIGINT NOT NULL DEFAULT 0,
    tool_duration_ms    BIGINT NOT NULL DEFAULT 0,
    created_at          BIGINT NOT NULL DEFAULT 0,
    updated_at          BIGINT NOT NULL DEFAULT 0,
    KEY idx_executions_started (started_at),
    KEY idx_executions_status_started (status, started_at),
    KEY idx_executions_agent_started (agent_id, started_at),
    KEY idx_executions_channel_started (channel, started_at),
    KEY idx_executions_session (session_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS agent_executions;

ALTER TABLE messages
    DROP KEY idx_messages_processing_created,
    DROP KEY idx_messages_execution,
    DROP COLUMN processing_outcome,
    DROP COLUMN processing_status,
    DROP COLUMN execution_id;

-- +goose StatementEnd
