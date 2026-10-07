ALTER TABLE services
    ADD COLUMN auto_build BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN build_workflow VARCHAR(100) NOT NULL DEFAULT 'platform-build.yml';

CREATE TABLE releases (
    id CHAR(36) NOT NULL PRIMARY KEY,
    service_id CHAR(36) NOT NULL,
    number INT NOT NULL,
    source VARCHAR(16) NOT NULL,
    git_ref VARCHAR(255) NOT NULL DEFAULT '',
    commit_sha CHAR(40) NOT NULL DEFAULT '',
    image VARCHAR(255) NOT NULL,
    tag VARCHAR(128) NOT NULL DEFAULT '',
    digest VARCHAR(80) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL,
    `trigger` VARCHAR(16) NOT NULL,
    requested_by CHAR(36) NOT NULL DEFAULT '',
    run_url VARCHAR(512) NOT NULL DEFAULT '',
    failure VARCHAR(512) NOT NULL DEFAULT '',
    created_at DATETIME(3) NOT NULL,
    finished_at DATETIME(3) NULL,
    UNIQUE KEY releases_number (service_id, number),
    KEY releases_status (status, created_at)
);
