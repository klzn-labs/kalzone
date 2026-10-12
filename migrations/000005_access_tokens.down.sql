ALTER TABLE audit_events
    DROP KEY audit_events_token,
    DROP COLUMN token_name,
    DROP COLUMN token_id;

DROP TABLE access_tokens;
