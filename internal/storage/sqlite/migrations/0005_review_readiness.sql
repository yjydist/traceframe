CREATE TABLE review_findings (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    project_revision INTEGER NOT NULL CHECK (project_revision > 0),
    severity TEXT NOT NULL CHECK (severity IN ('info', 'low', 'medium', 'high', 'blocking')),
    category TEXT NOT NULL,
    affected_entity_ids_json TEXT NOT NULL,
    claim TEXT NOT NULL,
    evidence TEXT NOT NULL,
    recommended_resolution TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('open', 'resolved', 'dismissed', 'risk_accepted')),
    resolution_rationale TEXT NOT NULL DEFAULT '',
    counter_evidence_refs_json TEXT NOT NULL DEFAULT '[]',
    resolved_by TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    resolved_at TEXT,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (run_id) REFERENCES agent_runs(id) ON DELETE CASCADE
);

CREATE TABLE baselines (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    project_revision INTEGER NOT NULL CHECK (project_revision > 0),
    checksum TEXT NOT NULL,
    snapshot_json TEXT NOT NULL,
    approval_actor TEXT NOT NULL,
    approval_rationale TEXT NOT NULL,
    approved_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (project_id, project_revision),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE INDEX idx_review_findings_project_status ON review_findings(project_id, status, severity, created_at);
CREATE INDEX idx_baselines_project_revision ON baselines(project_id, project_revision DESC);
