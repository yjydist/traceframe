export type ProjectMode = "greenfield" | "feature" | "refactor" | "spike";

export type Project = {
  id: string;
  name: string;
  raw_request: string;
  mode: ProjectMode;
  output_language: string;
  stage: string;
  status: "active" | "ready" | "archived";
  revision: number;
  created_at: string;
  updated_at: string;
};

export type Entity = {
  id: string;
  project_id: string;
  kind: string;
  title: string;
  body: Record<string, unknown>;
  status: string;
  origin: string;
  confidence: number;
  freshness: string;
  source_refs: string[];
  tags: string[];
  revision: number;
};

export type Relation = {
  id: string;
  from_id: string;
  type: string;
  to_id: string;
  rationale: string;
};

export type Snapshot = {
  schema_version: string;
  project: Project;
  entities: Entity[];
  relations: Relation[];
};

export type Traceability = {
  project_revision: number;
  nodes: Array<{ id: string; kind: string; title: string; incoming: number; outgoing: number }>;
  edges: Relation[];
  unlinked: string[];
};

export type QuestionItem = { entity: Entity; priority: number; reason: string; blocking: boolean };
export type WorkflowState = { stage: string; project_revision: number; gate_passed: boolean; blockers: string[]; checks: Array<{ code: string; passed: boolean; message: string }>; recommended_roles: string[]; concerns: Array<{ name: string; mandatory: boolean; triggers: string[] }>; assessment: { criticality: "low" | "medium" | "high"; active_concerns: string[]; corrected: boolean } };
export type AgentRun = { id: string; project_id: string; role: string; state: string; task: string; base_revision: number; usage: { model_turns: number; tool_calls: number; input_tokens: number; output_tokens: number }; error_code?: string; error_message?: string; created_at: string; updated_at: string };
export type Approval = { id: string; project_id: string; subject_id: string; subject_revision: number; project_revision: number; status: "pending" | "approved" | "rejected" | "invalidated"; requested_by: string; resolved_by?: string; rationale?: string; created_at: string; resolved_at?: string };
export type ReviewFinding = { id: string; project_id: string; run_id: string; project_revision: number; severity: "info" | "low" | "medium" | "high" | "blocking"; category: string; affected_entity_ids: string[]; claim: string; evidence: string; recommended_resolution: string; status: "open" | "resolved" | "dismissed" | "risk_accepted"; resolution_rationale?: string; counter_evidence_refs: string[] };
export type Readiness = { project_id: string; project_revision: number; ready: boolean; checks: Array<{ code: string; passed: boolean; message: string }>; blockers: string[] };
export type Baseline = { id: string; project_id: string; project_revision: number; checksum: string; approval_actor: string; approval_rationale: string; approved_at: string; created_at: string };
export type ArtifactVersion = { id: string; artifact_id: string; project_id: string; renderer_type: "html" | "markdown" | "json" | "mermaid"; renderer_version: string; source_revision: number; source_baseline_id?: string; included_entity_ids: string[]; content_type: string; content: string; checksum: string; generation_run_id: string; stale: boolean; created_at: string };
export type Artifact = { id: string; project_id: string; view_type: string; title: string; renderer_type: ArtifactVersion["renderer_type"]; mandatory: boolean; concern?: string; created_at: string; latest?: ArtifactVersion };
export type RepositoryGrant = { id: string; project_id: string; root_path: string; canonical_root: string; created_at: string; revoked_at?: string };
export type RepositoryEntry = { path: string; kind: string; size?: number; sha256?: string; start_line?: number; end_line?: number; content?: string; summary?: string };
export type RepositoryToolResult = { tool: string; grant_id: string; entries: RepositoryEntry[]; evidence_ids: string[]; truncated: boolean; untrusted: boolean; policy: string; model_revision?: number };
export type ImpactAnalysis = { project_id: string; project_revision: number; mode: string; subject_id?: string; repository_evidence_ids: string[]; directly_affected_ids: string[]; transitively_affected_ids: string[]; potentially_stale_ids: string[] };

type Problem = { message?: string };

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: init?.body ? { "Content-Type": "application/json", ...init.headers } : init?.headers,
  });
  if (!response.ok) {
    const problem = (await response.json().catch(() => ({}))) as Problem;
    throw new Error(problem.message ?? `Request failed with status ${response.status}`);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return response.json() as Promise<T>;
}

export function listProjects(includeArchived = false) {
  return request<{ projects: Project[] }>(`/api/v1/projects?include_archived=${includeArchived}`);
}

export function createProject(input: { name: string; raw_request: string; mode: ProjectMode }) {
  return request<Project>("/api/v1/projects", { method: "POST", body: JSON.stringify(input) });
}

export function getSnapshot(projectID: string) {
  return request<Snapshot>(`/api/v1/projects/${projectID}/snapshot`);
}

export function getTraceability(projectID: string) {
  return request<Traceability>(`/api/v1/projects/${projectID}/traceability`);
}

export function updateProject(projectID: string, input: { expected_revision: number; name: string }) {
  return request<Project>(`/api/v1/projects/${projectID}`, { method: "PATCH", body: JSON.stringify(input) });
}

export function archiveProject(projectID: string, expectedRevision: number) {
  return request<Project>(`/api/v1/projects/${projectID}/archive`, { method: "POST", body: JSON.stringify({ expected_revision: expectedRevision }) });
}

export function deleteProject(projectID: string, expectedRevision: number) {
  return request<void>(`/api/v1/projects/${projectID}`, { method: "DELETE", body: JSON.stringify({ expected_revision: expectedRevision, confirm_project_id: projectID }) });
}

export function applyCommands(projectID: string, expectedRevision: number, commands: unknown[]) {
  return request<Snapshot>(`/api/v1/projects/${projectID}/commands`, { method: "POST", body: JSON.stringify({ expected_revision: expectedRevision, commands }) });
}

export function listQuestions(projectID: string) { return request<{ questions: QuestionItem[] }>(`/api/v1/projects/${projectID}/questions`); }
export function respondToQuestion(projectID: string, questionID: string, expectedRevision: number, action: "answer" | "defer" | "reject", answer?: string) {
  return request<Snapshot>(`/api/v1/projects/${projectID}/questions/${questionID}/answer`, { method: "POST", body: JSON.stringify({ expected_revision: expectedRevision, action, ...(answer === undefined ? {} : { answer }) }) });
}
export function getWorkflow(projectID: string) { return request<WorkflowState>(`/api/v1/projects/${projectID}/workflow`); }
export function correctAssessment(projectID: string, expectedRevision: number, criticality: string, activeConcerns: string[]) {
  return request<WorkflowState["assessment"]>(`/api/v1/projects/${projectID}/workflow/assessment`, { method: "PUT", body: JSON.stringify({ expected_revision: expectedRevision, criticality, active_concerns: activeConcerns }) });
}
export function listRuns(projectID: string) { return request<{ runs: AgentRun[] }>(`/api/v1/projects/${projectID}/runs`); }
export function createRun(projectID: string, task: string, role: string) {
  return request<AgentRun>(`/api/v1/projects/${projectID}/runs`, { method: "POST", body: JSON.stringify({ role, task, idempotency_key: crypto.randomUUID() }) });
}
export function cancelRun(projectID: string, runID: string) { return request<AgentRun>(`/api/v1/projects/${projectID}/runs/${runID}/cancel`, { method: "POST" }); }
export function listApprovals(projectID: string) { return request<{ approvals: Approval[] }>(`/api/v1/projects/${projectID}/approvals`); }
export function resolveApproval(projectID: string, approvalID: string, expectedRevision: number, approve: boolean, rationale: string) {
  return request<Approval>(`/api/v1/projects/${projectID}/approvals/${approvalID}/${approve ? "approve" : "reject"}`, { method: "POST", body: JSON.stringify({ expected_revision: expectedRevision, rationale }) });
}
export function listReviewFindings(projectID: string) { return request<{ findings: ReviewFinding[] }>(`/api/v1/projects/${projectID}/reviews`); }
export function resolveReviewFinding(projectID: string, findingID: string, input: { expected_revision: number; status: string; rationale: string; counter_evidence_refs: string[] }) {
  return request<ReviewFinding>(`/api/v1/projects/${projectID}/reviews/${findingID}/resolve`, { method: "POST", body: JSON.stringify(input) });
}
export function getReadiness(projectID: string) { return request<Readiness>(`/api/v1/projects/${projectID}/readiness`); }
export function listBaselines(projectID: string) { return request<{ baselines: Baseline[] }>(`/api/v1/projects/${projectID}/baselines`); }
export function createBaseline(projectID: string, expectedRevision: number, rationale: string) {
  return request<Baseline>(`/api/v1/projects/${projectID}/baseline`, { method: "POST", body: JSON.stringify({ expected_revision: expectedRevision, approve: true, rationale }) });
}
export function listArtifacts(projectID: string) { return request<{ artifacts: Artifact[] }>(`/api/v1/projects/${projectID}/artifacts`); }
export function renderArtifacts(projectID: string, expectedRevision: number) {
  return request<{ source_revision: number; versions: ArtifactVersion[] }>(`/api/v1/projects/${projectID}/artifacts/render`, { method: "POST", body: JSON.stringify({ expected_revision: expectedRevision, renderers: ["html", "markdown", "json", "mermaid"] }) });
}
export function listRepositoryGrants(projectID: string, includeRevoked = true) { return request<{ grants: RepositoryGrant[] }>(`/api/v1/projects/${projectID}/repository/grants?include_revoked=${includeRevoked}`); }
export function createRepositoryGrant(projectID: string, rootPath: string) { return request<RepositoryGrant>(`/api/v1/projects/${projectID}/repository/grants`, { method: "POST", body: JSON.stringify({ root_path: rootPath }) }); }
export function revokeRepositoryGrant(projectID: string, grantID: string) { return request<RepositoryGrant>(`/api/v1/projects/${projectID}/repository/grants/${grantID}`, { method: "DELETE" }); }
export function executeRepositoryTool(projectID: string, input: { grant_id: string; tool: string; path?: string; query?: string; start_line?: number; end_line?: number; max_results?: number; record_evidence: boolean; expected_revision?: number; subject_id?: string }) {
  return request<RepositoryToolResult>(`/api/v1/projects/${projectID}/repository/tools`, { method: "POST", body: JSON.stringify(input) });
}
export function getImpactAnalysis(projectID: string, entityID = "") { return request<ImpactAnalysis>(`/api/v1/projects/${projectID}/impact${entityID ? `?entity_id=${encodeURIComponent(entityID)}` : ""}`); }
