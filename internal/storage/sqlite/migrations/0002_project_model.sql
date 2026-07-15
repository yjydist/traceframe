CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    raw_request TEXT NOT NULL,
    mode TEXT NOT NULL,
    output_language TEXT NOT NULL,
    stage TEXT NOT NULL,
    status TEXT NOT NULL,
    appetite_json TEXT,
    revision INTEGER NOT NULL CHECK (revision > 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE project_revisions (
    project_id TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0),
    checksum TEXT NOT NULL,
    snapshot_json TEXT NOT NULL,
    actor TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (project_id, revision),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE entities (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    body_json TEXT NOT NULL,
    status TEXT NOT NULL,
    origin TEXT NOT NULL,
    confidence REAL NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    freshness TEXT NOT NULL,
    source_refs_json TEXT NOT NULL,
    tags_json TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (project_id, id),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE entity_versions (
    entity_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    entity_revision INTEGER NOT NULL CHECK (entity_revision > 0),
    project_revision INTEGER NOT NULL CHECK (project_revision > 0),
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    body_json TEXT NOT NULL,
    status TEXT NOT NULL,
    origin TEXT NOT NULL,
    confidence REAL NOT NULL,
    freshness TEXT NOT NULL,
    source_refs_json TEXT NOT NULL,
    tags_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (entity_id, entity_revision),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, entity_id) REFERENCES entities(project_id, id) ON DELETE CASCADE
);

CREATE TABLE relations (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    from_id TEXT NOT NULL,
    type TEXT NOT NULL,
    to_id TEXT NOT NULL,
    rationale TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    UNIQUE (project_id, from_id, type, to_id),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, from_id) REFERENCES entities(project_id, id),
    FOREIGN KEY (project_id, to_id) REFERENCES entities(project_id, id)
);

CREATE TABLE events (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT NOT NULL,
    type TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    occurred_at TEXT NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE INDEX idx_projects_status_updated ON projects(status, updated_at DESC);
CREATE INDEX idx_entities_project_kind ON entities(project_id, kind, id);
CREATE INDEX idx_relations_project_from ON relations(project_id, from_id);
CREATE INDEX idx_relations_project_to ON relations(project_id, to_id);
CREATE INDEX idx_events_project_sequence ON events(project_id, sequence);
