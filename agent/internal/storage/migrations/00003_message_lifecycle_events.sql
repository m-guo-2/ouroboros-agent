-- +goose Up
-- +goose StatementBegin

CREATE TABLE message_lifecycle_events (
    id                  BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    session_id           VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    message_id           BIGINT NOT NULL DEFAULT 0,
    trace_id             VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    channel_message_id   VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    stage                VARCHAR(64) NOT NULL,
    status               VARCHAR(32) NOT NULL DEFAULT 'success',
    outcome              VARCHAR(32) NOT NULL DEFAULT '',
    summary              VARCHAR(512) NOT NULL DEFAULT '',
    payload_json         MEDIUMTEXT NOT NULL DEFAULT ('{}'),
    created_at           BIGINT NOT NULL DEFAULT 0,
    KEY idx_lifecycle_session_created (session_id, created_at),
    KEY idx_lifecycle_message (message_id),
    KEY idx_lifecycle_trace (trace_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS message_lifecycle_events;

-- +goose StatementEnd
