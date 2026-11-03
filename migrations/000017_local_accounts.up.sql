ALTER TABLE users
    ADD COLUMN password_hash VARCHAR(255) NULL,
    ADD COLUMN disabled_at DATETIME(3) NULL,
    ADD INDEX users_active_admins (is_admin, disabled_at);

CREATE TABLE account_links (
    id CHAR(36) NOT NULL PRIMARY KEY,
    kind VARCHAR(16) NOT NULL,
    token_hash CHAR(64) NOT NULL UNIQUE,
    team_id CHAR(36) NULL,
    role VARCHAR(16) NULL,
    user_id CHAR(36) NULL,
    created_by CHAR(36) NULL,
    created_at DATETIME(3) NOT NULL,
    expires_at DATETIME(3) NOT NULL,
    used_at DATETIME(3) NULL,
    revoked_at DATETIME(3) NULL,
    INDEX account_links_team (team_id),
    INDEX account_links_user (user_id),
    INDEX account_links_created_by (created_by, kind)
);
