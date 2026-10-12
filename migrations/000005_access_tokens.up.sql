CREATE TABLE access_tokens (
    id CHAR(36) NOT NULL PRIMARY KEY,
    owner_id CHAR(36) NOT NULL,
    name VARCHAR(64) NOT NULL,
    token_hash CHAR(64) NOT NULL,
    display_prefix CHAR(8) NOT NULL,
    scopes JSON NOT NULL,
    team_ids JSON NOT NULL,
    bypass BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(3) NOT NULL,
    created_by_token_id CHAR(36) NOT NULL DEFAULT '',
    expires_at DATETIME(3) NOT NULL,
    last_used_at DATETIME(3) NULL,
    last_used_ip VARCHAR(45) NOT NULL DEFAULT '',
    revoked_at DATETIME(3) NULL,
    active_name VARCHAR(64) AS (IF(revoked_at IS NULL, name, NULL)) STORED,
    UNIQUE KEY access_tokens_name (owner_id, active_name),
    UNIQUE KEY access_tokens_hash (token_hash),
    KEY access_tokens_owner (owner_id)
);

ALTER TABLE audit_events
    ADD COLUMN token_id CHAR(36) NOT NULL DEFAULT '',
    ADD COLUMN token_name VARCHAR(64) NOT NULL DEFAULT '',
    ADD KEY audit_events_token (token_id);
