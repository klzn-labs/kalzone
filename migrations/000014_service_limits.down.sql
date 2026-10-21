ALTER TABLE services
    DROP COLUMN limit_response_ms,
    DROP COLUMN limit_cpu_percent,
    DROP COLUMN limit_db_cpu_percent;
