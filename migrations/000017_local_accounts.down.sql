DROP TABLE account_links;

ALTER TABLE users
    DROP INDEX users_active_admins,
    DROP COLUMN disabled_at,
    DROP COLUMN password_hash;
