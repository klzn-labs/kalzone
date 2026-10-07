CREATE TABLE deployments (
    id CHAR(36) NOT NULL PRIMARY KEY,
    service_id CHAR(36) NOT NULL,
    environment VARCHAR(32) NOT NULL,
    number INT NOT NULL,
    release_id CHAR(36) NOT NULL,
    release_number INT NOT NULL,
    image_ref VARCHAR(400) NOT NULL,
    config_revision INT NOT NULL,
    strategy VARCHAR(16) NOT NULL,
    `trigger` VARCHAR(16) NOT NULL,
    status VARCHAR(16) NOT NULL,
    requested_by CHAR(36) NOT NULL DEFAULT '',
    agent VARCHAR(64) NOT NULL DEFAULT '',
    message VARCHAR(512) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL,
    started_at DATETIME(3) NULL,
    finished_at DATETIME(3) NULL,
    UNIQUE KEY deployments_number (service_id, environment, number),
    KEY deployments_status (status, created_at)
);

CREATE TABLE deployment_events (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    deployment_id CHAR(36) NOT NULL,
    at DATETIME(3) NOT NULL,
    message VARCHAR(1000) NOT NULL,
    KEY deployment_events_deployment (deployment_id, id)
);

CREATE TABLE agents (
    name VARCHAR(64) NOT NULL PRIMARY KEY,
    version VARCHAR(64) NOT NULL DEFAULT '',
    last_seen_at DATETIME(3) NOT NULL
);
