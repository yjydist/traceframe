import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  Archive,
  ArrowLeft,
  CircleDot,
  FolderKanban,
  GitBranch,
  FolderSearch,
  Plus,
  Pencil,
  Search,
  Trash2,
  HelpCircle,
  ListChecks,
  Play,
  Ban,
  CheckCircle2,
  ClipboardCheck,
  FileCheck2,
  Download,
  Layers3,
  RefreshCw,
  Scale,
  Settings2,
  ShieldCheck,
  SlidersHorizontal,
  XCircle,
  X,
} from "lucide-react";
import { FormEvent, ReactNode, useEffect, useMemo, useState } from "react";
import { Link, Navigate, Route, Routes, useNavigate, useParams } from "react-router-dom";
import {
  applyCommands,
  archiveProject,
  createProject,
  deleteProject,
  getSnapshot,
  getTraceability,
  getWorkflow,
  listProjects,
  listQuestions,
  listRuns,
  respondToQuestion,
  createRun,
  cancelRun,
  correctAssessment,
  createBaseline,
  listApprovals,
  listBaselines,
  listArtifacts,
  listReviewFindings,
  resolveApproval,
  resolveReviewFinding,
  getReadiness,
  getImpactAnalysis,
  renderArtifacts,
  listRepositoryGrants,
  createRepositoryGrant,
  revokeRepositoryGrant,
  executeRepositoryTool,
  updateProject,
  type Entity,
  type AgentRun,
  type Approval,
  type ReviewFinding,
  type Artifact,
  type ProjectMode,
  type RepositoryToolResult,
} from "./api";

type Health = { status: "ok" | "unavailable"; database: "ok" | "unavailable" };

async function getHealth(): Promise<Health> {
  const response = await fetch("/api/v1/health");
  if (!response.ok) throw new Error("Service unavailable");
  return response.json() as Promise<Health>;
}

function AppShell({ children }: { children: ReactNode }) {
  const health = useQuery({ queryKey: ["health"], queryFn: getHealth });
  const connected = health.data?.status === "ok";
  return (
    <div className="app-shell">
      <header className="topbar">
        <Link className="brand" to="/projects" aria-label="Traceframe projects">
          <span className="brand-mark" aria-hidden="true">T</span><span>Traceframe</span>
        </Link>
        <div className="service-status" role="status">
          <span className={connected ? "status-dot connected" : "status-dot"} />
          {health.isPending ? "Connecting" : connected ? "Local service ready" : "Service unavailable"}
        </div>
      </header>
      <aside className="navigation" aria-label="Primary navigation">
        <Link className="nav-item active" to="/projects" aria-current="page">
          <FolderKanban size={18} aria-hidden="true" /><span>Projects</span>
        </Link>
      </aside>
      <main className="workspace">{children}</main>
    </div>
  );
}

function ProjectTabs({ projectID, active }: { projectID: string; active: "model" | "questions" | "decisions" | "reviews" | "artifacts" | "runs" | "settings" }) {
  return <nav className="project-tabs" aria-label="Project workspace"><Link className={active === "model" ? "active" : ""} to={`/projects/${projectID}/model`}><CircleDot size={16} />Model</Link><Link className={active === "questions" ? "active" : ""} to={`/projects/${projectID}/questions`}><HelpCircle size={16} />Questions</Link><Link className={active === "decisions" ? "active" : ""} to={`/projects/${projectID}/decisions`}><Scale size={16} />Decisions</Link><Link className={active === "reviews" ? "active" : ""} to={`/projects/${projectID}/reviews`}><ClipboardCheck size={16} />Reviews</Link><Link className={active === "artifacts" ? "active" : ""} to={`/projects/${projectID}/artifacts`}><Layers3 size={16} />Artifacts</Link><Link className={active === "runs" ? "active" : ""} to={`/projects/${projectID}/runs`}><ListChecks size={16} />Runs</Link><Link className={active === "settings" ? "active" : ""} to={`/projects/${projectID}/settings`}><Settings2 size={16} />Settings</Link></nav>;
}

function ProjectList() {
  const [creating, setCreating] = useState(false);
  const projects = useQuery({ queryKey: ["projects"], queryFn: () => listProjects(false) });
  const projectList = projects.data?.projects ?? [];
  return (
    <AppShell>
      <div className="workspace-heading">
        <div><p className="eyebrow">Workspace</p><h1>Projects</h1></div>
        <button className="primary-button" type="button" onClick={() => setCreating(true)}>
          <Plus size={17} aria-hidden="true" />New project
        </button>
      </div>
      {projects.isError && <ErrorBanner message={projects.error.message} />}
      {projects.isPending ? (
        <div className="loading-row">Loading projects...</div>
      ) : projectList.length === 0 ? (
        <section className="empty-state" aria-labelledby="empty-title">
          <div className="empty-icon" aria-hidden="true"><FolderKanban size={28} /></div>
          <h2 id="empty-title">No projects yet</h2><p>Create a project from the request you want to shape.</p>
        </section>
      ) : (
        <div className="project-list">
          {projectList.map((project) => (
            <Link className="project-row" to={`/projects/${project.id}/model`} key={project.id}>
              <div className="project-row-main"><h2>{project.name}</h2><p>{project.raw_request}</p></div>
              <div className="project-meta"><span>{project.mode}</span><span>{project.stage}</span><span>r{project.revision}</span></div>
            </Link>
          ))}
        </div>
      )}
      {creating && <CreateProjectDialog onClose={() => setCreating(false)} />}
    </AppShell>
  );
}

function CreateProjectDialog({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [request, setRequest] = useState("");
  const [mode, setMode] = useState<ProjectMode>("greenfield");
  const create = useMutation({
    mutationFn: createProject,
    onSuccess: async (project) => {
      await queryClient.invalidateQueries({ queryKey: ["projects"] });
      navigate(`/projects/${project.id}/model`);
    },
  });
  function submit(event: FormEvent) {
    event.preventDefault();
    create.mutate({ name, raw_request: request, mode });
  }
  return (
    <div className="dialog-backdrop" role="presentation">
      <section className="dialog" role="dialog" aria-modal="true" aria-labelledby="create-title">
        <div className="dialog-heading"><h2 id="create-title">New project</h2><button className="icon-button" onClick={onClose} type="button" title="Close"><X size={18} /></button></div>
        <form onSubmit={submit}>
          <label>Project name<input value={name} onChange={(event) => setName(event.target.value)} required autoFocus /></label>
          <label>Initial request<textarea value={request} onChange={(event) => setRequest(event.target.value)} required rows={5} /></label>
          <label>Mode<select value={mode} onChange={(event) => setMode(event.target.value as ProjectMode)}><option value="greenfield">Greenfield</option><option value="feature">Feature</option><option value="refactor">Refactor</option><option value="spike">Spike</option></select></label>
          {create.isError && <ErrorBanner message={create.error.message} />}
          <div className="dialog-actions"><button className="secondary-button" type="button" onClick={onClose}>Cancel</button><button className="primary-button" disabled={create.isPending} type="submit">{create.isPending ? "Creating..." : "Create project"}</button></div>
        </form>
      </section>
    </div>
  );
}

function ModelView() {
  const { id = "" } = useParams();
  const [query, setQuery] = useState("");
  const [kind, setKind] = useState("all");
  const [dialog, setDialog] = useState<"entity" | "relation" | "rename" | "routing" | "delete" | null>(null);
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const snapshot = useQuery({ queryKey: ["snapshot", id], queryFn: () => getSnapshot(id), enabled: id !== "" });
  const trace = useQuery({ queryKey: ["traceability", id], queryFn: () => getTraceability(id), enabled: id !== "" });
  const workflow = useQuery({ queryKey: ["workflow", id], queryFn: () => getWorkflow(id), enabled: id !== "" });
  const entities = useMemo(() => {
    const source = snapshot.data?.entities ?? [];
    return source.filter((entity) => (kind === "all" || entity.kind === kind) && (entity.title.toLowerCase().includes(query.toLowerCase()) || entity.id.includes(query)));
  }, [snapshot.data, kind, query]);
  const kinds = [...new Set((snapshot.data?.entities ?? []).map((entity) => entity.kind))].sort();
  return (
    <AppShell>
      {snapshot.isPending ? <div className="loading-row">Loading project model...</div> : snapshot.isError ? <ErrorBanner message={snapshot.error.message} /> : (
        <>
          <Link className="back-link" to="/projects"><ArrowLeft size={16} />Projects</Link>
          <ProjectTabs projectID={id} active="model" />
          <div className="model-heading">
            <div><p className="eyebrow">{snapshot.data.project.mode} / {snapshot.data.project.stage}</p><h1>{snapshot.data.project.name}</h1><p>{snapshot.data.project.raw_request}</p></div>
            <div className="model-actions"><span className="revision-badge">Revision {snapshot.data.project.revision}</span><button className="icon-button" type="button" title="Rename project" onClick={() => setDialog("rename")}><Pencil size={17} /></button><button className="icon-button" type="button" title="Archive project" onClick={async () => { await archiveProject(id, snapshot.data.project.revision); await queryClient.invalidateQueries({ queryKey: ["projects"] }); navigate("/projects"); }}><Archive size={17} /></button><button className="icon-button danger" type="button" title="Delete project" onClick={() => setDialog("delete")}><Trash2 size={17} /></button></div>
          </div>
          <div className="model-stats" aria-label="Model summary">
            <div><CircleDot size={17} /><strong>{snapshot.data.entities.length}</strong><span>Entities</span></div>
            <div><GitBranch size={17} /><strong>{snapshot.data.relations.length}</strong><span>Relations</span></div>
            <div><Activity size={17} /><strong>{trace.data?.unlinked.length ?? 0}</strong><span>Unlinked</span></div>
          </div>
          {workflow.data && <div className="routing-strip"><div><strong>Engineering concerns</strong><span>{workflow.data.assessment.active_concerns.join(", ") || "No conditional concerns routed"}</span></div><div><span>{workflow.data.assessment.criticality} criticality{workflow.data.assessment.corrected ? " / corrected" : ""}</span><button className="icon-button" type="button" title="Edit concern routing" onClick={() => setDialog("routing")}><SlidersHorizontal size={17} /></button></div></div>}
          <section className="model-section">
            <div className="section-heading"><div><h2>Entities</h2><p>Canonical typed knowledge in the current revision.</p></div><div className="model-filters"><label className="search-field"><Search size={16} /><span className="sr-only">Search entities</span><input placeholder="Search entities" value={query} onChange={(event) => setQuery(event.target.value)} /></label><select aria-label="Filter by kind" value={kind} onChange={(event) => setKind(event.target.value)}><option value="all">All kinds</option>{kinds.map((value) => <option value={value} key={value}>{value}</option>)}</select><button className="secondary-button" type="button" onClick={() => setDialog("entity")}><Plus size={16} />Add entity</button></div></div>
            {entities.length === 0 ? <div className="section-empty">No matching entities.</div> : <div className="entity-table" role="table"><div className="table-header" role="row"><span>Entity</span><span>Kind</span><span>Status</span><span>Revision</span></div>{entities.map((entity) => <div className="entity-row" role="row" key={entity.id}><div><strong>{entity.title}</strong><code>{entity.id}</code></div><span>{entity.kind}</span><span className="entity-status">{entity.status}</span><span>r{entity.revision}</span></div>)}</div>}
          </section>
          <section className="model-section"><div className="section-heading"><div><h2>Relations</h2><p>Typed edges preserving model traceability.</p></div><button className="secondary-button" type="button" disabled={snapshot.data.entities.length < 2} onClick={() => setDialog("relation")}><Plus size={16} />Add relation</button></div>{snapshot.data.relations.length === 0 ? <div className="section-empty">No relations in this revision.</div> : <div className="relation-list">{snapshot.data.relations.map((relation) => <div className="relation-row" key={relation.id}><code>{relation.from_id}</code><strong>{relation.type}</strong><code>{relation.to_id}</code><p>{relation.rationale}</p></div>)}</div>}</section>
          {dialog === "entity" && <EntityDialog projectID={id} revision={snapshot.data.project.revision} onClose={() => setDialog(null)} />}
          {dialog === "relation" && <RelationDialog projectID={id} revision={snapshot.data.project.revision} entities={snapshot.data.entities} onClose={() => setDialog(null)} />}
          {dialog === "rename" && <RenameDialog projectID={id} revision={snapshot.data.project.revision} name={snapshot.data.project.name} onClose={() => setDialog(null)} />}
          {dialog === "routing" && workflow.data && <RoutingDialog projectID={id} revision={snapshot.data.project.revision} criticality={workflow.data.assessment.criticality} concerns={workflow.data.assessment.active_concerns} onClose={() => setDialog(null)} />}
          {dialog === "delete" && <DeleteDialog projectID={id} revision={snapshot.data.project.revision} onClose={() => setDialog(null)} />}
        </>
      )}
    </AppShell>
  );
}

function QuestionsView() {
  const { id = "" } = useParams();
  const queryClient = useQueryClient();
  const [answers, setAnswers] = useState<Record<string, string>>({});
  const snapshot = useQuery({ queryKey: ["snapshot", id], queryFn: () => getSnapshot(id), enabled: id !== "" });
  const questions = useQuery({ queryKey: ["questions", id], queryFn: () => listQuestions(id), enabled: id !== "" });
  const workflow = useQuery({ queryKey: ["workflow", id], queryFn: () => getWorkflow(id), enabled: id !== "" });
  const respond = useMutation({
    mutationFn: ({ questionID, action }: { questionID: string; action: "answer" | "defer" | "reject" }) => respondToQuestion(id, questionID, snapshot.data!.project.revision, action, action === "answer" ? answers[questionID] : undefined),
    onSuccess: async () => { await Promise.all([queryClient.invalidateQueries({ queryKey: ["snapshot", id] }), queryClient.invalidateQueries({ queryKey: ["questions", id] }), queryClient.invalidateQueries({ queryKey: ["workflow", id] })]); },
  });
  return <AppShell>{snapshot.isPending ? <div className="loading-row">Loading questions...</div> : snapshot.isError ? <ErrorBanner message={snapshot.error.message} /> : <><Link className="back-link" to="/projects"><ArrowLeft size={16} />Projects</Link><ProjectTabs projectID={id} active="questions" /><div className="model-heading"><div><p className="eyebrow">{snapshot.data.project.stage} / revision {snapshot.data.project.revision}</p><h1>Questions</h1><p>{snapshot.data.project.name}</p></div><span className="revision-badge">{workflow.data?.blockers.length ?? 0} blockers</span></div>{workflow.data && !workflow.data.gate_passed && <div className="gate-strip"><strong>Stage gate needs attention</strong><span>{workflow.data.blockers.join(", ")}</span></div>}{questions.isError && <ErrorBanner message={questions.error.message} />}{(questions.data?.questions.length ?? 0) === 0 ? <section className="empty-state"><div className="empty-icon"><HelpCircle size={28} /></div><h2>No pending questions</h2><p>The current question batch is clear.</p></section> : <div className="question-list">{questions.data?.questions.map((item) => <article className="question-item" key={item.entity.id}><div className="question-topline"><span>Priority {item.priority}</span>{item.blocking && <strong>Blocking</strong>}</div><h2>{String(item.entity.body.prompt)}</h2><p>{item.reason}</p><textarea rows={3} value={answers[item.entity.id] ?? ""} onChange={(event) => setAnswers((current) => ({ ...current, [item.entity.id]: event.target.value }))} placeholder="Answer" aria-label={`Answer ${item.entity.title}`} /><div className="question-actions"><button className="secondary-button" onClick={() => respond.mutate({ questionID: item.entity.id, action: "reject" })}>Reject</button><button className="secondary-button" onClick={() => respond.mutate({ questionID: item.entity.id, action: "defer" })}>Defer</button><button className="primary-button" disabled={!answers[item.entity.id]?.trim() || respond.isPending} onClick={() => respond.mutate({ questionID: item.entity.id, action: "answer" })}>Submit answer</button></div></article>)}</div>}{respond.isError && <ErrorBanner message={respond.error.message} />}</>}</AppShell>;
}

const terminalRunStates = new Set(["completed", "failed", "cancelled"]);

function DecisionsView() {
  const { id = "" } = useParams();
  const queryClient = useQueryClient();
  const [rationales, setRationales] = useState<Record<string, string>>({});
  const snapshot = useQuery({ queryKey: ["snapshot", id], queryFn: () => getSnapshot(id), enabled: id !== "" });
  const approvals = useQuery({ queryKey: ["approvals", id], queryFn: () => listApprovals(id), enabled: id !== "" });
  const resolve = useMutation({
    mutationFn: ({ approval, approve }: { approval: Approval; approve: boolean }) => resolveApproval(id, approval.id, snapshot.data!.project.revision, approve, rationales[approval.id] ?? ""),
    onSuccess: async () => { await Promise.all([queryClient.invalidateQueries({ queryKey: ["approvals", id] }), queryClient.invalidateQueries({ queryKey: ["workflow", id] })]); },
  });
  const decisions = snapshot.data?.entities.filter((entity) => entity.kind === "decision") ?? [];
  const options = snapshot.data?.entities.filter((entity) => entity.kind === "option") ?? [];
  const approvalFor = (subjectID: string) => approvals.data?.approvals.filter((approval) => approval.subject_id === subjectID).at(-1);
  return <AppShell>{snapshot.isPending ? <div className="loading-row">Loading decisions...</div> : snapshot.isError ? <ErrorBanner message={snapshot.error.message} /> : <><Link className="back-link" to="/projects"><ArrowLeft size={16} />Projects</Link><ProjectTabs projectID={id} active="decisions" /><div className="model-heading"><div><p className="eyebrow">{snapshot.data.project.stage} / revision {snapshot.data.project.revision}</p><h1>Decisions</h1><p>{snapshot.data.project.name}</p></div><span className="revision-badge">{approvals.data?.approvals.filter((approval) => approval.status === "pending").length ?? 0} pending</span></div>{approvals.isError && <ErrorBanner message={approvals.error.message} />}{options.length > 0 && <section className="model-section"><div className="section-heading"><div><h2>Options</h2><p>Visible alternatives and their trade-offs.</p></div></div><div className="option-grid">{options.map((option) => <article className="option-row" key={option.id}><div><h3>{option.title}</h3><code>{option.id}</code></div><p>{String(option.body.description ?? "")}</p><div><strong>Benefits</strong><span>{asList(option.body.benefits).join(", ") || "None recorded"}</span></div><div><strong>Risks</strong><span>{asList(option.body.risks).join(", ") || "None recorded"}</span></div></article>)}</div></section>}<section className="model-section"><div className="section-heading"><div><h2>Recorded decisions</h2><p>Selections remain tied to evidence, consequences, and exact revisions.</p></div></div>{decisions.length === 0 ? <div className="section-empty">No decisions recorded.</div> : <div className="decision-list">{decisions.map((decision) => { const approval = approvalFor(decision.id); return <article className="decision-row" key={decision.id}><div className="decision-summary"><div><h3>{decision.title}</h3><code>{decision.id} / entity r{decision.revision}</code></div><span className={`approval-status status-${approval?.status ?? "not-required"}`}>{approval?.status.replaceAll("_", " ") ?? "approval not required"}</span></div><p>{String(decision.body.rationale ?? "")}</p><dl><div><dt>Selected option</dt><dd>{String(decision.body.selected_option_id ?? "Not recorded")}</dd></div><div><dt>Significance</dt><dd>{String(decision.body.significance ?? "Not recorded")}</dd></div><div><dt>Consequences</dt><dd>{asList(decision.body.consequences).join(", ") || "None recorded"}</dd></div></dl>{approval?.status === "pending" && <div className="approval-actions"><label>Approval rationale<input value={rationales[approval.id] ?? ""} onChange={(event) => setRationales((current) => ({ ...current, [approval.id]: event.target.value }))} /></label><button className="secondary-button" disabled={resolve.isPending} onClick={() => resolve.mutate({ approval, approve: false })}><XCircle size={16} />Reject</button><button className="primary-button" disabled={resolve.isPending} onClick={() => resolve.mutate({ approval, approve: true })}><CheckCircle2 size={16} />Approve r{approval.subject_revision}</button></div>}</article>; })}</div>}{resolve.isError && <ErrorBanner message={resolve.error.message} />}</section></>}</AppShell>;
}

function asList(value: unknown): string[] { return Array.isArray(value) ? value.map(String) : []; }

function ReviewsView() {
  const { id = "" } = useParams();
  const queryClient = useQueryClient();
  const [resolutions, setResolutions] = useState<Record<string, { status: string; rationale: string; counter: string }>>({});
  const [baselineRationale, setBaselineRationale] = useState("");
  const [baselineApproved, setBaselineApproved] = useState(false);
  const snapshot = useQuery({ queryKey: ["snapshot", id], queryFn: () => getSnapshot(id), enabled: id !== "" });
  const findings = useQuery({ queryKey: ["reviews", id], queryFn: () => listReviewFindings(id), enabled: id !== "" });
  const readiness = useQuery({ queryKey: ["readiness", id], queryFn: () => getReadiness(id), enabled: id !== "" });
  const approvals = useQuery({ queryKey: ["approvals", id], queryFn: () => listApprovals(id), enabled: id !== "" });
  const baselines = useQuery({ queryKey: ["baselines", id], queryFn: () => listBaselines(id), enabled: id !== "" });
  const refresh = async () => { await Promise.all([queryClient.invalidateQueries({ queryKey: ["reviews", id] }), queryClient.invalidateQueries({ queryKey: ["readiness", id] }), queryClient.invalidateQueries({ queryKey: ["approvals", id] }), queryClient.invalidateQueries({ queryKey: ["baselines", id] }), queryClient.invalidateQueries({ queryKey: ["snapshot", id] })]); };
  const resolveFinding = useMutation({ mutationFn: (finding: ReviewFinding) => { const value = resolutions[finding.id] ?? { status: "resolved", rationale: "", counter: "" }; return resolveReviewFinding(id, finding.id, { expected_revision: snapshot.data!.project.revision, status: value.status, rationale: value.rationale, counter_evidence_refs: value.counter.split(",").map((item) => item.trim()).filter(Boolean) }); }, onSuccess: refresh });
  const resolveRisk = useMutation({ mutationFn: ({ approval, approve }: { approval: Approval; approve: boolean }) => resolveApproval(id, approval.id, snapshot.data!.project.revision, approve, approve ? "Residual risk explicitly accepted" : "Residual risk rejected"), onSuccess: refresh });
  const freeze = useMutation({ mutationFn: () => createBaseline(id, snapshot.data!.project.revision, baselineRationale), onSuccess: refresh });
  const riskIDs = new Set(snapshot.data?.entities.filter((entity) => entity.kind === "risk").map((entity) => entity.id) ?? []);
  const riskApprovals = approvals.data?.approvals.filter((approval) => riskIDs.has(approval.subject_id) && approval.status === "pending") ?? [];
  const updateResolution = (id: string, changes: Partial<{ status: string; rationale: string; counter: string }>) => setResolutions((current) => {
    const previous = current[id] ?? { status: "resolved", rationale: "", counter: "" };
    return { ...current, [id]: { ...previous, ...changes } };
  });
  const pageError = findings.error ?? readiness.error ?? approvals.error ?? baselines.error;
  return <AppShell>{snapshot.isPending ? <div className="loading-row">Loading review...</div> : snapshot.isError ? <ErrorBanner message={snapshot.error.message} /> : <><Link className="back-link" to="/projects"><ArrowLeft size={16} />Projects</Link><ProjectTabs projectID={id} active="reviews" /><div className="model-heading"><div><p className="eyebrow">{snapshot.data.project.stage} / revision {snapshot.data.project.revision}</p><h1>Review and readiness</h1><p>{snapshot.data.project.name}</p></div><span className={`readiness-badge ${readiness.data?.ready ? "ready" : "blocked"}`}>{readiness.data?.ready ? "Ready to baseline" : `${readiness.data?.blockers.length ?? 0} blockers`}</span></div>{pageError && <ErrorBanner message={pageError.message} />}<section className="model-section"><div className="section-heading"><div><h2>Readiness checks</h2><p>Deterministic evidence required before an immutable baseline.</p></div></div><div className="readiness-list">{readiness.data?.checks.map((check) => <div className={check.passed ? "passed" : "failed"} key={check.code}>{check.passed ? <CheckCircle2 size={17} /> : <XCircle size={17} />}<div><strong>{check.code.replaceAll("_", " ")}</strong><span>{check.message}</span></div></div>)}</div></section>{riskApprovals.length > 0 && <section className="model-section"><div className="section-heading"><div><h2>Residual risk approvals</h2><p>High residual risk requires an explicit user decision.</p></div></div>{riskApprovals.map((approval) => { const risk = snapshot.data.entities.find((entity) => entity.id === approval.subject_id); return <div className="risk-approval-row" key={approval.id}><div><strong>{risk?.title ?? approval.subject_id}</strong><span>entity r{approval.subject_revision}</span></div><button className="secondary-button" onClick={() => resolveRisk.mutate({ approval, approve: false })}><XCircle size={16} />Reject</button><button className="primary-button" onClick={() => resolveRisk.mutate({ approval, approve: true })}><CheckCircle2 size={16} />Accept risk</button></div>; })}</section>}<section className="model-section"><div className="section-heading"><div><h2>Review findings</h2><p>Blocking findings require resolution or dismissal with counter-evidence.</p></div></div>{(findings.data?.findings.length ?? 0) === 0 ? <div className="section-empty">No review findings recorded.</div> : <div className="finding-list">{findings.data?.findings.map((finding) => { const value = resolutions[finding.id] ?? { status: "resolved", rationale: "", counter: "" }; return <article className="finding-row" key={finding.id}><div className="finding-heading"><div><span className={`severity severity-${finding.severity}`}>{finding.severity}</span><strong>{finding.category}</strong></div><span>{finding.status.replaceAll("_", " ")}</span></div><h3>{finding.claim}</h3><p>{finding.evidence}</p><div className="finding-recommendation"><strong>Resolution</strong><span>{finding.recommended_resolution}</span></div>{finding.status === "open" && <div className="finding-resolution"><select aria-label={`Resolution for ${finding.id}`} value={value.status} onChange={(event) => updateResolution(finding.id, { status: event.target.value })}><option value="resolved">Resolved</option><option value="dismissed">Dismissed</option>{finding.severity !== "blocking" && <option value="risk_accepted">Risk accepted</option>}</select><input aria-label={`Rationale for ${finding.id}`} placeholder="Resolution rationale" value={value.rationale} onChange={(event) => updateResolution(finding.id, { rationale: event.target.value })} />{value.status === "dismissed" && <input aria-label={`Counter evidence for ${finding.id}`} placeholder="Counter-evidence IDs, comma separated" value={value.counter} onChange={(event) => updateResolution(finding.id, { counter: event.target.value })} />}<button className="primary-button" disabled={!value.rationale.trim() || resolveFinding.isPending} onClick={() => resolveFinding.mutate(finding)}>Resolve finding</button></div>}</article>; })}</div>}</section><section className="model-section"><div className="section-heading"><div><h2>Baselines</h2><p>Approved snapshots remain immutable as work continues.</p></div></div>{snapshot.data.project.stage === "REVIEW" && <div className="baseline-control"><label>Approval rationale<input value={baselineRationale} onChange={(event) => setBaselineRationale(event.target.value)} /></label><label className="approval-check"><input type="checkbox" checked={baselineApproved} onChange={(event) => setBaselineApproved(event.target.checked)} />Approve revision {snapshot.data.project.revision} as the implementation baseline</label><button className="primary-button" disabled={!readiness.data?.ready || !baselineApproved || !baselineRationale.trim() || freeze.isPending} onClick={() => freeze.mutate()}><FileCheck2 size={16} />Create baseline</button></div>}{freeze.isError && <ErrorBanner message={freeze.error.message} />}<div className="baseline-list">{baselines.data?.baselines.map((baseline) => <div key={baseline.id}><div><strong>Revision {baseline.project_revision}</strong><code>{baseline.checksum.slice(0, 16)}</code></div><span>{baseline.approval_rationale}</span><time>{new Date(baseline.approved_at).toLocaleString()}</time></div>)}</div>{(baselines.data?.baselines.length ?? 0) === 0 && <div className="section-empty">No baseline created.</div>}</section></>}</AppShell>;
}

function ArtifactsView() {
  const { id = "" } = useParams();
  const queryClient = useQueryClient();
  const [selectedID, setSelectedID] = useState("");
  const snapshot = useQuery({ queryKey: ["snapshot", id], queryFn: () => getSnapshot(id), enabled: id !== "" });
  const artifacts = useQuery({ queryKey: ["artifacts", id], queryFn: () => listArtifacts(id), enabled: id !== "" });
  const render = useMutation({ mutationFn: () => renderArtifacts(id, snapshot.data!.project.revision), onSuccess: async () => { await Promise.all([queryClient.invalidateQueries({ queryKey: ["artifacts", id] }), queryClient.invalidateQueries({ queryKey: ["readiness", id] })]); } });
  const artifactList = artifacts.data?.artifacts ?? [];
  const selected = artifactList.find((artifact) => artifact.id === selectedID) ?? artifactList[0];
  const grouped = artifactList.reduce<Record<string, Artifact[]>>((result, artifact) => {
    (result[artifact.view_type] ??= []).push(artifact);
    return result;
  }, {});
  return <AppShell>{snapshot.isPending ? <div className="loading-row">Loading artifacts...</div> : snapshot.isError ? <ErrorBanner message={snapshot.error.message} /> : <><Link className="back-link" to="/projects"><ArrowLeft size={16} />Projects</Link><ProjectTabs projectID={id} active="artifacts" /><div className="model-heading"><div><p className="eyebrow">{snapshot.data.project.stage} / revision {snapshot.data.project.revision}</p><h1>Artifacts</h1><p>{snapshot.data.project.name}</p></div><div className="artifact-actions"><a className="secondary-button" href={`/api/v1/projects/${id}/export?format=json`}><Download size={16} />JSON</a><a className="secondary-button" href={`/api/v1/projects/${id}/export?format=markdown`}><Download size={16} />Markdown</a><button className="primary-button" disabled={render.isPending} onClick={() => render.mutate()}><RefreshCw size={16} />{render.isPending ? "Rendering..." : "Render revision"}</button></div></div>{render.isError && <ErrorBanner message={render.error.message} />}{artifacts.isError && <ErrorBanner message={artifacts.error.message} />}{artifactList.length === 0 ? <section className="empty-state"><div className="empty-icon"><Layers3 size={28} /></div><h2>No artifacts rendered</h2><p>Render the current revision to create routed views.</p></section> : <div className="artifact-workspace"><nav className="artifact-index" aria-label="Artifact views">{Object.entries(grouped).map(([view, versions]) => <div key={view}><strong>{versions[0].title}</strong>{versions.map((artifact) => <button className={selected?.id === artifact.id ? "active" : ""} key={artifact.id} onClick={() => setSelectedID(artifact.id)}><span>{artifact.renderer_type}</span>{artifact.latest?.stale && <em>Stale</em>}</button>)}</div>)}</nav>{selected?.latest && <section className="artifact-preview"><header><div><h2>{selected.title}</h2><span>{selected.renderer_type} v{selected.latest.renderer_version} / source r{selected.latest.source_revision}</span></div><div><code>{selected.latest.checksum.slice(0, 16)}</code>{selected.latest.stale && <strong>Stale source</strong>}</div></header>{selected.renderer_type === "html" ? <iframe title={`${selected.title} preview`} sandbox="" src={`/api/v1/projects/${id}/artifacts/${selected.id}?raw=true`} /> : <pre>{selected.latest.content}</pre>}<footer><span>{selected.latest.included_entity_ids.length} source entities</span><a href={`/api/v1/projects/${id}/artifacts/${selected.id}?raw=true`}><Download size={15} />Download</a></footer></section>}</div>}</>}</AppShell>;
}

function RunsView() {
  const { id = "" } = useParams();
  const queryClient = useQueryClient();
  const [task, setTask] = useState("Identify the next bounded stage gap and propose only evidence-backed entities");
  const [role, setRole] = useState("discovery");
  const snapshot = useQuery({ queryKey: ["snapshot", id], queryFn: () => getSnapshot(id), enabled: id !== "" });
  const runs = useQuery({ queryKey: ["runs", id], queryFn: () => listRuns(id), enabled: id !== "" });
  const workflow = useQuery({ queryKey: ["workflow", id], queryFn: () => getWorkflow(id), enabled: id !== "" });
  const roles = workflow.data?.recommended_roles.length ? workflow.data.recommended_roles : ["discovery"];
  const selectedRole = roles.includes(role) ? role : roles[0];
  useEffect(() => {
    if (!id) return;
    const events = new EventSource(`/api/v1/projects/${id}/events`);
    const refresh = () => { void queryClient.invalidateQueries({ queryKey: ["runs", id] }); void queryClient.invalidateQueries({ queryKey: ["snapshot", id] }); };
    events.addEventListener("run.state_changed", refresh);
    events.addEventListener("run.cancelled", refresh);
    events.addEventListener("run.failed", refresh);
    events.addEventListener("run.completed", refresh);
    events.addEventListener("project.revision_created", refresh);
    return () => events.close();
  }, [id, queryClient]);
  const start = useMutation({ mutationFn: () => createRun(id, task, selectedRole), onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ["runs", id] }); } });
  const cancel = useMutation({ mutationFn: (runID: string) => cancelRun(id, runID), onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ["runs", id] }); } });
  return <AppShell>{snapshot.isPending ? <div className="loading-row">Loading runs...</div> : snapshot.isError ? <ErrorBanner message={snapshot.error.message} /> : <><Link className="back-link" to="/projects"><ArrowLeft size={16} />Projects</Link><ProjectTabs projectID={id} active="runs" /><div className="model-heading"><div><p className="eyebrow">{snapshot.data.project.stage} / revision {snapshot.data.project.revision}</p><h1>Agent runs</h1><p>{snapshot.data.project.name}</p></div></div><section className="run-launch"><label>Specialist<select value={selectedRole} onChange={(event) => setRole(event.target.value)}>{roles.map((value) => <option key={value} value={value}>{value.replaceAll("_", " ")}</option>)}</select></label><label>Bounded task<textarea rows={3} value={task} onChange={(event) => setTask(event.target.value)} /></label><button className="primary-button" disabled={!task.trim() || start.isPending} onClick={() => start.mutate()}><Play size={16} />{start.isPending ? "Queueing..." : "Run specialist"}</button></section>{start.isError && <ErrorBanner message={start.error.message} />}{runs.isError && <ErrorBanner message={runs.error.message} />}<div className="run-list">{runs.data?.runs.map((run: AgentRun) => <article className="run-row" key={run.id}><div className="run-state"><span className={`status-dot run-${run.state}`} /><strong>{run.state.replaceAll("_", " ")}</strong></div><div><h2>{run.task}</h2><code>{run.id}</code>{run.error_message && <p className="run-error">{run.error_message}</p>}</div><div className="run-usage"><span>{run.role}</span><span>{run.usage.model_turns} turns</span><span>{run.usage.input_tokens + run.usage.output_tokens} tokens</span></div>{!terminalRunStates.has(run.state) && <button className="icon-button danger" title="Cancel run" onClick={() => cancel.mutate(run.id)}><Ban size={17} /></button>}</article>)}</div>{(runs.data?.runs.length ?? 0) === 0 && <div className="section-empty">No runs yet.</div>}</>}</AppShell>;
}

const repositoryTools = ["list_files", "search_text", "read_file", "inspect_manifest", "inspect_git_metadata", "list_tests"];

function SettingsView() {
  const { id = "" } = useParams();
  const queryClient = useQueryClient();
  const snapshot = useQuery({ queryKey: ["snapshot", id], queryFn: () => getSnapshot(id), enabled: id !== "" });
  const applicable = snapshot.data?.project.mode === "feature" || snapshot.data?.project.mode === "refactor";
  const grants = useQuery({ queryKey: ["repository-grants", id], queryFn: () => listRepositoryGrants(id), enabled: id !== "" && applicable });
  const [rootPath, setRootPath] = useState("");
  const [selectedGrantID, setGrantID] = useState("");
  const [tool, setTool] = useState("list_files");
  const [repositoryPath, setRepositoryPath] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const [startLine, setStartLine] = useState(1);
  const [endLine, setEndLine] = useState(200);
  const [recordEvidence, setRecordEvidence] = useState(true);
  const [subjectID, setSubjectID] = useState("");
  const [result, setResult] = useState<RepositoryToolResult | null>(null);
  const activeGrants = grants.data?.grants.filter((grant) => !grant.revoked_at) ?? [];
  const grantID = activeGrants.some((grant) => grant.id === selectedGrantID) ? selectedGrantID : activeGrants[0]?.id ?? "";
  const impact = useQuery({ queryKey: ["repository-impact", id, subjectID], queryFn: () => getImpactAnalysis(id, subjectID), enabled: id !== "" && applicable });

  const createGrant = useMutation({ mutationFn: () => createRepositoryGrant(id, rootPath), onSuccess: async (grant) => { setRootPath(""); setGrantID(grant.id); await queryClient.invalidateQueries({ queryKey: ["repository-grants", id] }); } });
  const revokeGrant = useMutation({ mutationFn: (selected: string) => revokeRepositoryGrant(id, selected), onSuccess: async () => { setResult(null); await queryClient.invalidateQueries({ queryKey: ["repository-grants", id] }); } });
  const execute = useMutation({
    mutationFn: () => executeRepositoryTool(id, {
      grant_id: grantID, tool, path: repositoryPath || undefined, query: tool === "search_text" ? searchQuery : undefined,
      start_line: tool === "read_file" ? startLine : undefined, end_line: tool === "read_file" ? endLine : undefined,
      record_evidence: recordEvidence, expected_revision: recordEvidence ? snapshot.data!.project.revision : undefined, subject_id: recordEvidence && subjectID ? subjectID : undefined,
    }),
    onSuccess: async (value) => { setResult(value); await Promise.all([queryClient.invalidateQueries({ queryKey: ["snapshot", id] }), queryClient.invalidateQueries({ queryKey: ["repository-impact", id] })]); },
  });
  const pageError = snapshot.error ?? grants.error ?? createGrant.error ?? revokeGrant.error ?? execute.error ?? impact.error;

  if (snapshot.isPending) return <AppShell><div className="loading-row">Loading settings...</div></AppShell>;
  if (snapshot.isError) return <AppShell><ErrorBanner message={snapshot.error.message} /></AppShell>;
  return <AppShell><Link className="back-link" to="/projects"><ArrowLeft size={16} />Projects</Link><ProjectTabs projectID={id} active="settings" /><div className="model-heading"><div><p className="eyebrow">{snapshot.data.project.mode} / revision {snapshot.data.project.revision}</p><h1>Project settings</h1><p>{snapshot.data.project.name}</p></div><span className="revision-badge">{activeGrants.length} active roots</span></div>{pageError && <ErrorBanner message={pageError.message} />}{!applicable ? <section className="empty-state"><div className="empty-icon"><FolderSearch size={28} /></div><h2>No repository access for this mode</h2><p>Repository roots apply to feature and refactor projects.</p></section> : <><section className="model-section"><div className="section-heading"><div><h2>Repository grants</h2><p>Canonical read-only roots.</p></div></div><form className="grant-form" onSubmit={(event) => { event.preventDefault(); createGrant.mutate(); }}><label>Local root<input value={rootPath} onChange={(event) => setRootPath(event.target.value)} placeholder="/absolute/path/to/repository" /></label><button className="primary-button" disabled={!rootPath.trim() || createGrant.isPending}><FolderSearch size={16} />Grant access</button></form><div className="grant-list">{grants.data?.grants.map((grant) => <div key={grant.id}><div><strong>{grant.canonical_root}</strong><code>{grant.id}</code></div><span className={`approval-status ${grant.revoked_at ? "status-rejected" : "status-approved"}`}>{grant.revoked_at ? "revoked" : "active"}</span>{!grant.revoked_at && <button className="icon-button danger" title="Revoke repository access" onClick={() => revokeGrant.mutate(grant.id)}><XCircle size={17} /></button>}</div>)}</div>{grants.data?.grants.length === 0 && <div className="section-empty">No repository roots granted.</div>}</section><section className="model-section"><div className="section-heading"><div><h2>Repository inspection</h2><p>Bounded tool results and evidence locators.</p></div><span className="security-label"><ShieldCheck size={15} />Read only</span></div><div className="repository-tool-grid"><label>Root<select value={grantID} onChange={(event) => setGrantID(event.target.value)}>{activeGrants.map((grant) => <option key={grant.id} value={grant.id}>{grant.canonical_root}</option>)}</select></label><label>Tool<select value={tool} onChange={(event) => setTool(event.target.value)}>{repositoryTools.map((name) => <option key={name} value={name}>{name.replaceAll("_", " ")}</option>)}</select></label><label>Relative path<input value={repositoryPath} onChange={(event) => setRepositoryPath(event.target.value)} placeholder="." /></label>{tool === "search_text" && <label>Fixed text<input value={searchQuery} onChange={(event) => setSearchQuery(event.target.value)} /></label>}{tool === "read_file" && <><label>Start line<input type="number" min={1} value={startLine} onChange={(event) => setStartLine(Number(event.target.value))} /></label><label>End line<input type="number" min={1} value={endLine} onChange={(event) => setEndLine(Number(event.target.value))} /></label></>}<label>Evidence subject<select value={subjectID} onChange={(event) => setSubjectID(event.target.value)}><option value="">Current-state evidence</option>{snapshot.data.entities.filter((entity) => entity.kind !== "evidence").map((entity) => <option key={entity.id} value={entity.id}>{entity.title}</option>)}</select></label><label className="approval-check"><input type="checkbox" checked={recordEvidence} onChange={(event) => setRecordEvidence(event.target.checked)} />Record evidence</label><button className="primary-button" disabled={!grantID || execute.isPending || (tool === "search_text" && !searchQuery.trim())} onClick={() => execute.mutate()}><FolderSearch size={16} />{execute.isPending ? "Inspecting..." : "Run tool"}</button></div>{result && <div className="repository-result"><header><strong>{result.tool.replaceAll("_", " ")}</strong><span>{result.entries.length} results{result.truncated ? " / truncated" : ""}</span></header>{result.entries.map((entry, index) => <article key={`${entry.path}-${entry.start_line ?? 0}-${index}`}><div><strong>{entry.path}</strong><span>{entry.start_line ? `L${entry.start_line}-${entry.end_line}` : entry.kind}</span><code>{entry.sha256?.slice(0, 16)}</code></div>{entry.content ? <pre>{entry.content}</pre> : <p>{entry.summary}</p>}</article>)}{result.entries.length === 0 && <div className="section-empty">No matching repository content.</div>}{result.evidence_ids.length > 0 && <footer><span>Evidence</span>{result.evidence_ids.map((evidenceID) => <code key={evidenceID}>{evidenceID}</code>)}</footer>}</div>}</section><section className="model-section"><div className="section-heading"><div><h2>Current state and impact</h2><p>Evidence-backed model reachability.</p></div></div>{impact.data && <div className="impact-grid"><div><strong>Repository evidence</strong><span>{impact.data.repository_evidence_ids.length}</span></div><div><strong>Directly affected</strong><span>{impact.data.directly_affected_ids.length}</span></div><div><strong>Transitively affected</strong><span>{impact.data.transitively_affected_ids.length}</span></div><div><strong>Stale model entities</strong><span>{impact.data.potentially_stale_ids.length}</span></div></div>}<div className="impact-ids">{impact.data?.repository_evidence_ids.map((evidenceID) => <code key={evidenceID}>{evidenceID}</code>)}</div></section></>}</AppShell>;
}

const entityKinds = ["goal", "stakeholder", "context", "scope_item", "constraint", "assumption", "question", "term", "scenario", "requirement", "quality_scenario", "risk", "option", "decision", "system_element", "work_slice", "experiment", "evidence", "verification"];
const relationTypes = ["motivates", "affects", "constrains", "assumes", "answers", "derives_from", "satisfies", "verifies", "mitigates", "selects", "rejects", "depends_on", "conflicts_with", "supersedes", "implements", "decomposes", "evidenced_by"];

function useModelMutation(projectID: string, onClose: () => void) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ revision, commands }: { revision: number; commands: unknown[] }) => applyCommands(projectID, revision, commands),
    onSuccess: async () => {
      await Promise.all([queryClient.invalidateQueries({ queryKey: ["snapshot", projectID] }), queryClient.invalidateQueries({ queryKey: ["traceability", projectID] })]);
      onClose();
    },
  });
}

function EntityDialog({ projectID, revision, onClose }: { projectID: string; revision: number; onClose: () => void }) {
  const [kind, setKind] = useState("goal");
  const [title, setTitle] = useState("");
  const [body, setBody] = useState('{"outcome":"","success_signals":[],"priority":"must"}');
  const [parseError, setParseError] = useState("");
  const mutation = useModelMutation(projectID, onClose);
  function submit(event: FormEvent) {
    event.preventDefault();
    let parsed: unknown;
    try { parsed = JSON.parse(body); setParseError(""); } catch { setParseError("Body must be valid JSON."); return; }
    mutation.mutate({ revision, commands: [{ type: "create_entity", entity: { kind, title, body: parsed, status: "draft", origin: "user", confidence: 1 } }] });
  }
  return <Dialog title="Add entity" onClose={onClose}><form onSubmit={submit}><label>Kind<select value={kind} onChange={(event) => setKind(event.target.value)}>{entityKinds.map((value) => <option key={value}>{value}</option>)}</select></label><label>Title<input value={title} onChange={(event) => setTitle(event.target.value)} required autoFocus /></label><label>Body (JSON)<textarea className="code-input" value={body} onChange={(event) => setBody(event.target.value)} rows={8} required /></label>{parseError && <ErrorBanner message={parseError} />}{mutation.isError && <ErrorBanner message={mutation.error.message} />}<DialogActions onClose={onClose} pending={mutation.isPending} label="Add entity" /></form></Dialog>;
}

function RelationDialog({ projectID, revision, entities, onClose }: { projectID: string; revision: number; entities: Entity[]; onClose: () => void }) {
  const [from, setFrom] = useState(entities[0]?.id ?? "");
  const [to, setTo] = useState(entities[1]?.id ?? "");
  const [type, setType] = useState("satisfies");
  const [rationale, setRationale] = useState("");
  const mutation = useModelMutation(projectID, onClose);
  function submit(event: FormEvent) { event.preventDefault(); mutation.mutate({ revision, commands: [{ type: "create_relation", relation: { from_id: from, type, to_id: to, rationale } }] }); }
  const options = entities.map((entity) => <option value={entity.id} key={entity.id}>{entity.title} ({entity.kind})</option>);
  return <Dialog title="Add relation" onClose={onClose}><form onSubmit={submit}><label>From<select value={from} onChange={(event) => setFrom(event.target.value)}>{options}</select></label><label>Relation<select value={type} onChange={(event) => setType(event.target.value)}>{relationTypes.map((value) => <option key={value}>{value}</option>)}</select></label><label>To<select value={to} onChange={(event) => setTo(event.target.value)}>{options}</select></label><label>Rationale<textarea value={rationale} onChange={(event) => setRationale(event.target.value)} rows={3} required /></label>{mutation.isError && <ErrorBanner message={mutation.error.message} />}<DialogActions onClose={onClose} pending={mutation.isPending} label="Add relation" /></form></Dialog>;
}

function RenameDialog({ projectID, revision, name, onClose }: { projectID: string; revision: number; name: string; onClose: () => void }) {
  const [value, setValue] = useState(name);
  const queryClient = useQueryClient();
  const mutation = useMutation({ mutationFn: () => updateProject(projectID, { expected_revision: revision, name: value }), onSuccess: async () => { await Promise.all([queryClient.invalidateQueries({ queryKey: ["snapshot", projectID] }), queryClient.invalidateQueries({ queryKey: ["projects"] })]); onClose(); } });
  return <Dialog title="Rename project" onClose={onClose}><form onSubmit={(event) => { event.preventDefault(); mutation.mutate(); }}><label>Project name<input value={value} onChange={(event) => setValue(event.target.value)} required autoFocus /></label>{mutation.isError && <ErrorBanner message={mutation.error.message} />}<DialogActions onClose={onClose} pending={mutation.isPending} label="Rename" /></form></Dialog>;
}

const concernOptions = ["interaction", "api", "data", "migration", "runtime", "deployment", "security", "privacy", "reliability", "performance", "compatibility", "repository_change", "experiment"];

function RoutingDialog({ projectID, revision, criticality: initialCriticality, concerns: initialConcerns, onClose }: { projectID: string; revision: number; criticality: "low" | "medium" | "high"; concerns: string[]; onClose: () => void }) {
  const [criticality, setCriticality] = useState(initialCriticality);
  const [concerns, setConcerns] = useState(initialConcerns);
  const queryClient = useQueryClient();
  const mutation = useMutation({ mutationFn: () => correctAssessment(projectID, revision, criticality, concerns), onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ["workflow", projectID] }); onClose(); } });
  function toggle(concern: string) { setConcerns((current) => current.includes(concern) ? current.filter((value) => value !== concern) : [...current, concern]); }
  return <Dialog title="Edit concern routing" onClose={onClose}><form onSubmit={(event) => { event.preventDefault(); mutation.mutate(); }}><label>Criticality<select value={criticality} onChange={(event) => setCriticality(event.target.value as "low" | "medium" | "high")}><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option></select></label><fieldset className="concern-fieldset"><legend>Active concerns</legend><div>{concernOptions.map((concern) => <label key={concern}><input type="checkbox" checked={concerns.includes(concern)} onChange={() => toggle(concern)} />{concern.replaceAll("_", " ")}</label>)}</div></fieldset>{mutation.isError && <ErrorBanner message={mutation.error.message} />}<DialogActions onClose={onClose} pending={mutation.isPending} label="Save routing" /></form></Dialog>;
}

function DeleteDialog({ projectID, revision, onClose }: { projectID: string; revision: number; onClose: () => void }) {
  const [confirmation, setConfirmation] = useState("");
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const mutation = useMutation({ mutationFn: () => deleteProject(projectID, revision), onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ["projects"] }); navigate("/projects"); } });
  return <Dialog title="Delete project" onClose={onClose}><form onSubmit={(event) => { event.preventDefault(); mutation.mutate(); }}><p className="dialog-copy">This permanently removes the project and its revision history.</p><label>Type the project ID to confirm<code>{projectID}</code><input value={confirmation} onChange={(event) => setConfirmation(event.target.value)} required autoFocus /></label>{mutation.isError && <ErrorBanner message={mutation.error.message} />}<div className="dialog-actions"><button className="secondary-button" type="button" onClick={onClose}>Cancel</button><button className="danger-button" disabled={confirmation !== projectID || mutation.isPending} type="submit">Delete project</button></div></form></Dialog>;
}

function Dialog({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  return <div className="dialog-backdrop" role="presentation"><section className="dialog" role="dialog" aria-modal="true" aria-label={title}><div className="dialog-heading"><h2>{title}</h2><button className="icon-button" onClick={onClose} type="button" title="Close"><X size={18} /></button></div>{children}</section></div>;
}

function DialogActions({ onClose, pending, label }: { onClose: () => void; pending: boolean; label: string }) {
  return <div className="dialog-actions"><button className="secondary-button" type="button" onClick={onClose}>Cancel</button><button className="primary-button" disabled={pending} type="submit">{pending ? "Saving..." : label}</button></div>;
}

function ErrorBanner({ message }: { message: string }) { return <div className="error-banner" role="alert">{message}</div>; }

export function App() {
  return <Routes><Route path="/projects" element={<ProjectList />} /><Route path="/projects/:id/model" element={<ModelView />} /><Route path="/projects/:id/questions" element={<QuestionsView />} /><Route path="/projects/:id/decisions" element={<DecisionsView />} /><Route path="/projects/:id/reviews" element={<ReviewsView />} /><Route path="/projects/:id/artifacts" element={<ArtifactsView />} /><Route path="/projects/:id/runs" element={<RunsView />} /><Route path="/projects/:id/settings" element={<SettingsView />} /><Route path="*" element={<Navigate to="/projects" replace />} /></Routes>;
}
