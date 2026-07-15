CREATE TABLE repository_grants (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    root_path TEXT NOT NULL,
    canonical_root TEXT NOT NULL,
    created_at TEXT NOT NULL,
    revoked_at TEXT,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_repository_grants_active_root
    ON repository_grants(project_id, canonical_root) WHERE revoked_at IS NULL;
CREATE INDEX idx_repository_grants_project ON repository_grants(project_id, created_at DESC);

CREATE TABLE repository_tool_calls (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    grant_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    arguments_checksum TEXT NOT NULL,
    result_checksum TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('completed', 'failed')),
    error_code TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    completed_at TEXT,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (grant_id) REFERENCES repository_grants(id) ON DELETE CASCADE
);

CREATE INDEX idx_repository_tool_calls_project ON repository_tool_calls(project_id, started_at DESC);
