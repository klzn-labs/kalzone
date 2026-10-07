CREATE TABLE users (
    id CHAR(36) NOT NULL PRIMARY KEY,
    github_id BIGINT NOT NULL,
    login VARCHAR(39) NOT NULL,
    name VARCHAR(255) NOT NULL DEFAULT '',
    avatar_url VARCHAR(512) NOT NULL DEFAULT '',
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(3) NOT NULL,
    last_login_at DATETIME(3) NOT NULL,
    UNIQUE KEY users_github_id (github_id),
    UNIQUE KEY users_login (login)
);

CREATE TABLE sessions (
    token_hash CHAR(64) NOT NULL PRIMARY KEY,
    csrf_token CHAR(64) NOT NULL,
    user_id CHAR(36) NOT NULL,
    created_at DATETIME(3) NOT NULL,
    expires_at DATETIME(3) NOT NULL,
    KEY sessions_user_id (user_id),
    KEY sessions_expires_at (expires_at)
);

CREATE TABLE teams (
    id CHAR(36) NOT NULL PRIMARY KEY,
    slug VARCHAR(40) NOT NULL,
    name VARCHAR(120) NOT NULL,
    created_at DATETIME(3) NOT NULL,
    UNIQUE KEY teams_slug (slug)
);

CREATE TABLE memberships (
    team_id CHAR(36) NOT NULL,
    user_id CHAR(36) NOT NULL,
    role VARCHAR(16) NOT NULL,
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (team_id, user_id),
    KEY memberships_user_id (user_id)
);

CREATE TABLE environments (
    slug VARCHAR(32) NOT NULL PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    protected BOOLEAN NOT NULL,
    position INT NOT NULL
);

INSERT INTO environments (slug, name, protected, position) VALUES
    ('dev', 'Development', FALSE, 1),
    ('prod', 'Production', TRUE, 2);

CREATE TABLE services (
    id CHAR(36) NOT NULL PRIMARY KEY,
    slug VARCHAR(40) NOT NULL,
    name VARCHAR(120) NOT NULL,
    description TEXT NOT NULL,
    owner_team_id CHAR(36) NOT NULL,
    github_repo VARCHAR(140) NOT NULL DEFAULT '',
    image_repository VARCHAR(255) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL,
    deleted_at DATETIME(3) NULL,
    UNIQUE KEY services_slug (slug),
    KEY services_owner_team_id (owner_team_id)
);

CREATE TABLE config_revisions (
    id CHAR(36) NOT NULL PRIMARY KEY,
    service_id CHAR(36) NOT NULL,
    environment VARCHAR(32) NOT NULL,
    number INT NOT NULL,
    spec JSON NOT NULL,
    env JSON NOT NULL,
    secrets JSON NOT NULL,
    host VARCHAR(253) NOT NULL DEFAULT '',
    author_id CHAR(36) NOT NULL,
    message VARCHAR(255) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL,
    UNIQUE KEY config_revisions_number (service_id, environment, number),
    KEY config_revisions_host (host)
);

CREATE TABLE secret_versions (
    id CHAR(36) NOT NULL PRIMARY KEY,
    service_id CHAR(36) NOT NULL,
    environment VARCHAR(32) NOT NULL,
    name VARCHAR(128) NOT NULL,
    ciphertext VARBINARY(16384) NOT NULL,
    nonce VARBINARY(12) NOT NULL,
    key_id VARCHAR(32) NOT NULL,
    created_by CHAR(36) NOT NULL,
    created_at DATETIME(3) NOT NULL,
    KEY secret_versions_lookup (service_id, environment, name)
);

CREATE TABLE audit_events (
    id CHAR(36) NOT NULL PRIMARY KEY,
    at DATETIME(3) NOT NULL,
    actor_id CHAR(36) NOT NULL,
    actor_login VARCHAR(39) NOT NULL,
    action VARCHAR(64) NOT NULL,
    target_kind VARCHAR(32) NOT NULL,
    target_id CHAR(36) NOT NULL,
    target_name VARCHAR(140) NOT NULL DEFAULT '',
    team_id CHAR(36) NOT NULL DEFAULT '',
    environment VARCHAR(32) NOT NULL DEFAULT '',
    details JSON NOT NULL,
    KEY audit_events_at (at),
    KEY audit_events_target (target_kind, target_id),
    KEY audit_events_actor (actor_id),
    KEY audit_events_team (team_id)
);
