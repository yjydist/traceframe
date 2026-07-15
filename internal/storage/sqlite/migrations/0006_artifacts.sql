ALTER TABLE baselines ADD COLUMN routed_concerns_json TEXT NOT NULL DEFAULT '[]';

CREATE TABLE artifacts (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    view_type TEXT NOT NULL,
    title TEXT NOT NULL,
    renderer_type TEXT NOT NULL,
    mandatory INTEGER NOT NULL CHECK (mandatory IN (0, 1)),
    concern TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    UNIQUE (project_id, view_type, renderer_type),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE artifact_versions (
    id TEXT PRIMARY KEY,
    artifact_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    renderer_version TEXT NOT NULL,
    source_revision INTEGER NOT NULL CHECK (source_revision > 0),
    source_baseline_id TEXT,
    included_entity_ids_json TEXT NOT NULL,
    content_type TEXT NOT NULL,
    content TEXT NOT NULL,
    checksum TEXT NOT NULL,
    generation_run_id TEXT NOT NULL,
    stale INTEGER NOT NULL CHECK (stale IN (0, 1)),
    created_at TEXT NOT NULL,
    UNIQUE (artifact_id, source_revision, renderer_version, checksum),
    FOREIGN KEY (artifact_id) REFERENCES artifacts(id) ON DELETE CASCADE,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (source_baseline_id) REFERENCES baselines(id) ON DELETE SET NULL
);

CREATE INDEX idx_artifacts_project_view ON artifacts(project_id, view_type, renderer_type);
CREATE INDEX idx_artifact_versions_current ON artifact_versions(project_id, source_revision, stale, created_at);
