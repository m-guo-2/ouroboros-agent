-- +goose Up
-- +goose StatementBegin

ALTER TABLE agent_configs
    ADD COLUMN sandbox_template_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'office-worker' AFTER model;

ALTER TABLE agent_personas
    ADD COLUMN sandbox_template_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER model;

ALTER TABLE group_persona_assignments
    ADD COLUMN sandbox_template_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER persona_id;

ALTER TABLE agent_sessions
    ADD COLUMN sandbox_template_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'office-worker' AFTER work_dir;

ALTER TABLE agent_executions
    ADD COLUMN sandbox_template_id VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'office-worker' AFTER channel;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE agent_executions
    DROP COLUMN sandbox_template_id;

ALTER TABLE agent_sessions
    DROP COLUMN sandbox_template_id;

ALTER TABLE group_persona_assignments
    DROP COLUMN sandbox_template_id;

ALTER TABLE agent_personas
    DROP COLUMN sandbox_template_id;

ALTER TABLE agent_configs
    DROP COLUMN sandbox_template_id;

-- +goose StatementEnd
