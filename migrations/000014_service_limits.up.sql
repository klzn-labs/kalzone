ALTER TABLE services
    ADD COLUMN limit_response_ms INT NOT NULL DEFAULT 0,
    ADD COLUMN limit_cpu_percent INT NOT NULL DEFAULT 0,
    ADD COLUMN limit_db_cpu_percent INT NOT NULL DEFAULT 0;
