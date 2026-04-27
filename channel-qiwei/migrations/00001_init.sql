-- +goose Up
-- +goose StatementBegin

-- Conventions:
--   * Business IDs:      VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin
--   * Short enums:       VARCHAR(32)
--   * External IDs:      VARCHAR(128)
--   * Business text:     TEXT / MEDIUMTEXT (utf8mb4)
--   * Timestamps:        BIGINT NOT NULL DEFAULT 0   (UTC epoch ms, 0 = unset)
--   * Soft delete:       deleted_at BIGINT NOT NULL DEFAULT 0
--   * No FOREIGN KEYs; integrity enforced at the repo layer.

-- ---------------------------------------------------------------
-- qiwei_accounts: one row per WeCom login session
-- ---------------------------------------------------------------
CREATE TABLE qiwei_accounts (
    id               VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    guid             VARCHAR(128) NOT NULL,
    -- TODO: encrypt (tracked separately as security debt)
    token            VARCHAR(512) NOT NULL,
    short_hash       VARCHAR(32)  NOT NULL,
    display_name     VARCHAR(256) NOT NULL DEFAULT '',
    agent_id         VARCHAR(64)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    enabled          TINYINT(1)   NOT NULL DEFAULT 1,
    self_user_id     VARCHAR(128) NOT NULL DEFAULT '',
    self_name        VARCHAR(256) NOT NULL DEFAULT '',
    self_alias       VARCHAR(256) NOT NULL DEFAULT '',
    self_avatar_url  VARCHAR(1024) NOT NULL DEFAULT '',
    self_corp_name   VARCHAR(256) NOT NULL DEFAULT '',
    self_synced_at   BIGINT       NOT NULL DEFAULT 0,
    meta_json        MEDIUMTEXT   NOT NULL DEFAULT ('{}'),
    notes            TEXT         NOT NULL DEFAULT (''),
    created_at       BIGINT       NOT NULL DEFAULT 0,
    updated_at       BIGINT       NOT NULL DEFAULT 0,
    deleted_at       BIGINT       NOT NULL DEFAULT 0,
    UNIQUE KEY uk_qiwei_accounts_guid (guid, deleted_at),
    UNIQUE KEY uk_qiwei_accounts_short_hash (short_hash, deleted_at),
    KEY idx_qiwei_accounts_agent (agent_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- qiwei_known_rooms: tracking which rooms we have ever seen
-- ---------------------------------------------------------------
CREATE TABLE qiwei_known_rooms (
    account_id  VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    room_id     VARCHAR(128) NOT NULL,
    created_at  BIGINT NOT NULL DEFAULT 0,
    deleted_at  BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, room_id),
    KEY idx_qiwei_known_rooms_account (account_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- qiwei_contacts: contact book per account. follow_user_json split
-- into qiwei_contact_followers below.
-- ---------------------------------------------------------------
CREATE TABLE qiwei_contacts (
    account_id        VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    user_id           VARCHAR(128) NOT NULL,
    external_user_id  VARCHAR(128) NOT NULL DEFAULT '',
    source            VARCHAR(32)  NOT NULL DEFAULT 'unknown',
    nickname          VARCHAR(256) NOT NULL DEFAULT '',
    real_name         VARCHAR(256) NOT NULL DEFAULT '',
    alias             VARCHAR(256) NOT NULL DEFAULT '',
    remark            VARCHAR(256) NOT NULL DEFAULT '',
    avatar_url        VARCHAR(1024) NOT NULL DEFAULT '',
    gender            VARCHAR(16)  NOT NULL DEFAULT '',
    corp_id           VARCHAR(128) NOT NULL DEFAULT '',
    corp_name         VARCHAR(256) NOT NULL DEFAULT '',
    raw_json          MEDIUMTEXT   NOT NULL DEFAULT ('{}'),
    first_seen_at     BIGINT       NOT NULL DEFAULT 0,
    last_synced_at    BIGINT       NOT NULL DEFAULT 0,
    updated_at        BIGINT       NOT NULL DEFAULT 0,
    deleted_at        BIGINT       NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, user_id),
    KEY idx_qiwei_contacts_external_user (account_id, external_user_id),
    KEY idx_contacts_nickname  (account_id, nickname),
    KEY idx_contacts_real_name (account_id, real_name),
    KEY idx_contacts_alias     (account_id, alias),
    KEY idx_contacts_remark    (account_id, remark)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE qiwei_contact_followers (
    id                BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    account_id        VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    user_id           VARCHAR(128) NOT NULL,
    follow_user_id    VARCHAR(128) NOT NULL,
    position          INT NOT NULL DEFAULT 0,
    created_at        BIGINT NOT NULL DEFAULT 0,
    UNIQUE KEY uk_contact_follower (account_id, user_id, follow_user_id),
    KEY idx_contact_follower_account_user (account_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- qiwei_rooms: group chat metadata
-- ---------------------------------------------------------------
CREATE TABLE qiwei_rooms (
    account_id      VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    room_id         VARCHAR(128) NOT NULL,
    name            VARCHAR(256) NOT NULL DEFAULT '',
    announcement    MEDIUMTEXT NOT NULL DEFAULT (''),
    notice          MEDIUMTEXT NOT NULL DEFAULT (''),
    owner_user_id   VARCHAR(128) NOT NULL DEFAULT '',
    member_count    INT NOT NULL DEFAULT 0,
    qr_code_url     VARCHAR(1024) NOT NULL DEFAULT '',
    raw_json        MEDIUMTEXT NOT NULL DEFAULT ('{}'),
    first_seen_at   BIGINT NOT NULL DEFAULT 0,
    last_synced_at  BIGINT NOT NULL DEFAULT 0,
    updated_at      BIGINT NOT NULL DEFAULT 0,
    deleted_at      BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, room_id),
    KEY idx_qiwei_rooms_owner (account_id, owner_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- qiwei_room_members: membership roster
-- ---------------------------------------------------------------
CREATE TABLE qiwei_room_members (
    account_id     VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    room_id        VARCHAR(128) NOT NULL,
    user_id        VARCHAR(128) NOT NULL,
    display_name   VARCHAR(256) NOT NULL DEFAULT '',
    role           VARCHAR(32) NOT NULL DEFAULT 'member',
    joined_at      BIGINT NOT NULL DEFAULT 0,
    last_seen_at   BIGINT NOT NULL DEFAULT 0,
    deleted_at     BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, room_id, user_id),
    KEY idx_qiwei_room_members_user (account_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- ---------------------------------------------------------------
-- qiwei_identity_links: bridge to downstream identity systems
-- ---------------------------------------------------------------
CREATE TABLE qiwei_identity_links (
    id                    BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    account_id            VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    user_id               VARCHAR(128) NOT NULL DEFAULT '',
    external_user_id      VARCHAR(128) NOT NULL DEFAULT '',
    downstream_system     VARCHAR(32)  NOT NULL,
    downstream_id         VARCHAR(128) NOT NULL,
    downstream_meta_json  MEDIUMTEXT   NOT NULL DEFAULT ('{}'),
    created_at            BIGINT NOT NULL DEFAULT 0,
    updated_at            BIGINT NOT NULL DEFAULT 0,
    deleted_at            BIGINT NOT NULL DEFAULT 0,
    UNIQUE KEY uk_qiwei_identity_links (downstream_system, downstream_id, deleted_at),
    KEY idx_qiwei_identity_links_external (account_id, external_user_id),
    KEY idx_qiwei_identity_links_user (account_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS qiwei_identity_links;
DROP TABLE IF EXISTS qiwei_room_members;
DROP TABLE IF EXISTS qiwei_rooms;
DROP TABLE IF EXISTS qiwei_contact_followers;
DROP TABLE IF EXISTS qiwei_contacts;
DROP TABLE IF EXISTS qiwei_known_rooms;
DROP TABLE IF EXISTS qiwei_accounts;
-- +goose StatementEnd
