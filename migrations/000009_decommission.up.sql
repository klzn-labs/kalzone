ALTER TABLE services
    ADD COLUMN status VARCHAR(16) NOT NULL DEFAULT 'active',
    ADD COLUMN live_slug VARCHAR(40) GENERATED ALWAYS AS (IF(deleted_at IS NULL, slug, NULL)) STORED,
    DROP INDEX services_slug,
    ADD UNIQUE KEY services_live_slug (live_slug),
    ADD KEY services_slug (slug);
