CREATE TABLE assessments (
    project_id TEXT PRIMARY KEY,
    assessment_json TEXT NOT NULL,
    project_revision INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE workflow_states (
    project_id TEXT PRIMARY KEY,
    stage TEXT NOT NULL,
    gate_json TEXT NOT NULL,
    reason TEXT NOT NULL,
    project_revision INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE agent_runs (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    role TEXT NOT NULL,
    state TEXT NOT NULL,
    task TEXT NOT NULL,
    base_revision INTEGER NOT NULL,
    budget_json TEXT NOT NULL,
    usage_json TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_checksum TEXT NOT NULL,
    prompt_version TEXT NOT NULL,
    response_schema_version TEXT NOT NULL,
    model_identifier TEXT NOT NULL DEFAULT '',
    provider_request_id TEXT NOT NULL DEFAULT '',
    selected_context_ids_json TEXT NOT NULL,
    allowed_tools_json TEXT NOT NULL,
    proposal_checksum TEXT NOT NULL DEFAULT '',
    application_outcome TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    cancel_requested_at TEXT,
    created_at TEXT NOT NULL,
    started_at TEXT,
    completed_at TEXT,
    updated_at TEXT NOT NULL,
    UNIQUE (project_id, idempotency_key),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE agent_run_steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    kind TEXT NOT NULL,
    state TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    usage_json TEXT NOT NULL DEFAULT '{}',
    started_at TEXT NOT NULL,
    completed_at TEXT,
    UNIQUE (run_id, sequence),
    FOREIGN KEY (run_id) REFERENCES agent_runs(id) ON DELETE CASCADE
);

CREATE TABLE tool_calls (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    step_sequence INTEGER NOT NULL,
    tool_name TEXT NOT NULL,
    state TEXT NOT NULL,
    arguments_checksum TEXT NOT NULL,
    result_checksum TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    completed_at TEXT,
    FOREIGN KEY (run_id) REFERENCES agent_runs(id) ON DELETE CASCADE
);

CREATE TABLE jobs (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    run_id TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    state TEXT NOT NULL,
    attempts INTEGER NOT NULL,
    max_attempts INTEGER NOT NULL,
    available_at TEXT NOT NULL,
    lease_owner TEXT NOT NULL DEFAULT '',
    lease_expires_at TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (run_id) REFERENCES agent_runs(id) ON DELETE CASCADE
);

CREATE INDEX idx_agent_runs_project_created ON agent_runs(project_id, created_at DESC);
CREATE INDEX idx_jobs_claim ON jobs(state, available_at, created_at);
CREATE INDEX idx_jobs_lease ON jobs(state, lease_expires_at);
