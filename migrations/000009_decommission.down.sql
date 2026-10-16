ALTER TABLE services
    DROP INDEX services_slug,
    DROP INDEX services_live_slug,
    DROP COLUMN live_slug,
    DROP COLUMN status,
    ADD UNIQUE KEY services_slug (slug);
