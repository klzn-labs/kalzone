CREATE TABLE queues (
    id CHAR(36) NOT NULL PRIMARY KEY,
    name VARCHAR(63) NOT NULL,
    environment VARCHAR(32) NOT NULL,
    team_id CHAR(36) NOT NULL,
    consumer_id CHAR(36) NOT NULL,
    version INT NOT NULL,
    paused BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(3) NOT NULL,
    updated_at DATETIME(3) NOT NULL,
    UNIQUE KEY queues_name (environment, name),
    KEY queues_team (team_id)
);

CREATE TABLE queue_versions (
    queue_id CHAR(36) NOT NULL,
    number INT NOT NULL,
    settings JSON NOT NULL,
    author_id CHAR(36) NOT NULL,
    message VARCHAR(255) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (queue_id, number)
);

CREATE TABLE queue_publishers (
    queue_id CHAR(36) NOT NULL,
    service_id CHAR(36) NOT NULL,
    PRIMARY KEY (queue_id, service_id),
    KEY queue_publishers_service (service_id)
);

CREATE TABLE queue_credentials (
    service_id CHAR(36) NOT NULL,
    environment VARCHAR(32) NOT NULL,
    token_ciphertext VARBINARY(512) NOT NULL,
    token_nonce VARBINARY(32) NOT NULL,
    token_hash CHAR(64) NOT NULL,
    key_ciphertext VARBINARY(512) NOT NULL,
    key_nonce VARBINARY(32) NOT NULL,
    previous_key_ciphertext VARBINARY(512) NULL,
    previous_key_nonce VARBINARY(32) NULL,
    key_id VARCHAR(32) NOT NULL,
    rotated_at DATETIME(3) NULL,
    PRIMARY KEY (service_id, environment),
    UNIQUE KEY queue_credentials_token (token_hash)
);

CREATE TABLE queue_commands (
    id CHAR(36) NOT NULL PRIMARY KEY,
    queue_id CHAR(36) NOT NULL,
    kind VARCHAR(16) NOT NULL,
    argument VARCHAR(255) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL,
    done_at DATETIME(3) NULL,
    KEY queue_commands_pending (done_at, created_at)
);
