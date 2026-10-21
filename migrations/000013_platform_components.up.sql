CREATE TABLE platform_components (
    name VARCHAR(64) NOT NULL PRIMARY KEY,
    payload JSON NOT NULL,
    agent VARCHAR(64) NOT NULL,
    observed_at DATETIME(3) NOT NULL
);
