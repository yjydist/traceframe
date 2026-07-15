CREATE TABLE approvals (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    subject_id TEXT NOT NULL,
    subject_revision INTEGER NOT NULL CHECK (subject_revision > 0),
    project_revision INTEGER NOT NULL CHECK (project_revision > 0),
    status TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'invalidated')),
    requested_by TEXT NOT NULL,
    resolved_by TEXT NOT NULL DEFAULT '',
    rationale TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    resolved_at TEXT,
    updated_at TEXT NOT NULL,
    UNIQUE (project_id, subject_id, subject_revision),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (project_id, subject_id) REFERENCES entities(project_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_approvals_project_status ON approvals(project_id, status, created_at);
