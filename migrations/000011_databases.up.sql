CREATE TABLE `databases` (
    id CHAR(36) NOT NULL PRIMARY KEY,
    name VARCHAR(20) NOT NULL,
    environment VARCHAR(32) NOT NULL,
    team_id CHAR(36) NOT NULL,
    schema_name VARCHAR(64) NOT NULL,
    username VARCHAR(32) NOT NULL,
    password_ciphertext VARBINARY(512) NOT NULL,
    password_nonce VARBINARY(32) NOT NULL,
    password_key_id VARCHAR(32) NOT NULL DEFAULT '',
    max_connections INT NOT NULL DEFAULT 10,
    adopted BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(3) NOT NULL,
    updated_at DATETIME(3) NOT NULL,
    UNIQUE KEY databases_name (environment, name),
    UNIQUE KEY databases_schema (schema_name),
    KEY databases_team (team_id)
);

CREATE TABLE database_attachments (
    database_id CHAR(36) NOT NULL,
    service_id CHAR(36) NOT NULL,
    environment VARCHAR(32) NOT NULL,
    attached_at DATETIME(3) NOT NULL,
    detached_at DATETIME(3) NULL,
    PRIMARY KEY (database_id, service_id),
    KEY database_attachments_service (service_id, environment)
);
