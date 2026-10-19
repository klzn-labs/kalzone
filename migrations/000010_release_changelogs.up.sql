CREATE TABLE release_changelogs (
    release_id CHAR(36) NOT NULL PRIMARY KEY,
    base_release_id CHAR(36) NOT NULL DEFAULT '',
    base_sha CHAR(40) NOT NULL DEFAULT '',
    head_sha CHAR(40) NOT NULL DEFAULT '',
    state VARCHAR(16) NOT NULL,
    ahead_by INT NOT NULL DEFAULT 0,
    behind_by INT NOT NULL DEFAULT 0,
    truncated BOOL NOT NULL DEFAULT FALSE,
    compare_url VARCHAR(512) NOT NULL DEFAULT '',
    commits JSON NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    last_error VARCHAR(512) NOT NULL DEFAULT '',
    updated_at DATETIME(3) NOT NULL,
    KEY release_changelogs_state (state, updated_at),
    KEY release_changelogs_pair (base_sha, head_sha)
);

ALTER TABLE releases ADD KEY releases_finished (service_id, finished_at);
