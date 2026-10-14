CREATE TABLE runtime_statuses (
    service_id CHAR(36) NOT NULL,
    environment VARCHAR(32) NOT NULL,
    container VARCHAR(120) NOT NULL DEFAULT '',
    found BOOLEAN NOT NULL,
    state VARCHAR(32) NOT NULL DEFAULT '',
    health VARCHAR(32) NOT NULL DEFAULT '',
    health_log JSON NOT NULL,
    exit_code INT NOT NULL DEFAULT 0,
    oom_killed BOOLEAN NOT NULL DEFAULT FALSE,
    restarts INT NOT NULL DEFAULT 0,
    image VARCHAR(255) NOT NULL DEFAULT '',
    memory_limit_mb INT NOT NULL DEFAULT 0,
    cpu_limit_millis INT NOT NULL DEFAULT 0,
    started_at DATETIME(3) NULL,
    observed_at DATETIME(3) NOT NULL,
    agent VARCHAR(64) NOT NULL,
    PRIMARY KEY (service_id, environment)
);

ALTER TABLE deployments ADD COLUMN failure JSON NULL;
