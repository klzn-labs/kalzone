DELETE FROM users WHERE github_id IS NULL;
ALTER TABLE users
    DROP COLUMN kind,
    MODIFY github_id BIGINT NOT NULL;
ALTER TABLE services DROP COLUMN simulated;
ALTER TABLE teams DROP COLUMN is_demo;
