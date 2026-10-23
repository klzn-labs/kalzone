CREATE TABLE crons (
    id CHAR(36) NOT NULL PRIMARY KEY,
    name VARCHAR(63) NOT NULL,
    environment VARCHAR(32) NOT NULL,
    team_id CHAR(36) NOT NULL,
    service_id CHAR(36) NOT NULL,
    schedule VARCHAR(128) NOT NULL,
    path VARCHAR(255) NOT NULL,
    payload BLOB NULL,
    timeout_ms INT NOT NULL,
    paused BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(3) NOT NULL,
    updated_at DATETIME(3) NOT NULL,
    UNIQUE KEY crons_name (environment, name),
    KEY crons_team (team_id),
    KEY crons_service (service_id)
);

CREATE TABLE cron_runs (
    id VARCHAR(64) NOT NULL PRIMARY KEY,
    cron_id CHAR(36) NOT NULL,
    on_demand BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(16) NOT NULL,
    scheduled_at DATETIME(3) NOT NULL,
    started_at DATETIME(3) NOT NULL,
    finished_at DATETIME(3) NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    last_status INT NOT NULL DEFAULT 0,
    error VARCHAR(512) NOT NULL DEFAULT '',
    KEY cron_runs_cron (cron_id, scheduled_at)
);

CREATE TABLE cron_commands (
    id CHAR(36) NOT NULL PRIMARY KEY,
    cron_id CHAR(36) NOT NULL,
    kind VARCHAR(16) NOT NULL,
    created_at DATETIME(3) NOT NULL,
    done_at DATETIME(3) NULL,
    KEY cron_commands_pending (done_at, created_at),
    KEY cron_commands_cron (cron_id)
);
