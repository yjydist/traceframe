CREATE TABLE application_metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

INSERT INTO application_metadata (key, value, updated_at)
VALUES ('schema_initialized', 'true', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
