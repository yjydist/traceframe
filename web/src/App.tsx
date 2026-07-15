import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  Archive,
  ArrowLeft,
  CircleDot,
  FolderKanban,
  GitBranch,
  Plus,
  Pencil,
  Search,
  Trash2,
  X,
} from "lucide-react";
import { FormEvent, ReactNode, useMemo, useState } from "react";
import { Link, Navigate, Route, Routes, useNavigate, useParams } from "react-router-dom";
import {
  applyCommands,
  archiveProject,
  createProject,
  deleteProject,
  getSnapshot,
  getTraceability,
  listProjects,
  updateProject,
  type Entity,
  type ProjectMode,
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
  const [dialog, setDialog] = useState<"entity" | "relation" | "rename" | "delete" | null>(null);
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const snapshot = useQuery({ queryKey: ["snapshot", id], queryFn: () => getSnapshot(id), enabled: id !== "" });
  const trace = useQuery({ queryKey: ["traceability", id], queryFn: () => getTraceability(id), enabled: id !== "" });
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
          <div className="model-heading">
            <div><p className="eyebrow">{snapshot.data.project.mode} / {snapshot.data.project.stage}</p><h1>{snapshot.data.project.name}</h1><p>{snapshot.data.project.raw_request}</p></div>
            <div className="model-actions"><span className="revision-badge">Revision {snapshot.data.project.revision}</span><button className="icon-button" type="button" title="Rename project" onClick={() => setDialog("rename")}><Pencil size={17} /></button><button className="icon-button" type="button" title="Archive project" onClick={async () => { await archiveProject(id, snapshot.data.project.revision); await queryClient.invalidateQueries({ queryKey: ["projects"] }); navigate("/projects"); }}><Archive size={17} /></button><button className="icon-button danger" type="button" title="Delete project" onClick={() => setDialog("delete")}><Trash2 size={17} /></button></div>
          </div>
          <div className="model-stats" aria-label="Model summary">
            <div><CircleDot size={17} /><strong>{snapshot.data.entities.length}</strong><span>Entities</span></div>
            <div><GitBranch size={17} /><strong>{snapshot.data.relations.length}</strong><span>Relations</span></div>
            <div><Activity size={17} /><strong>{trace.data?.unlinked.length ?? 0}</strong><span>Unlinked</span></div>
          </div>
          <section className="model-section">
            <div className="section-heading"><div><h2>Entities</h2><p>Canonical typed knowledge in the current revision.</p></div><div className="model-filters"><label className="search-field"><Search size={16} /><span className="sr-only">Search entities</span><input placeholder="Search entities" value={query} onChange={(event) => setQuery(event.target.value)} /></label><select aria-label="Filter by kind" value={kind} onChange={(event) => setKind(event.target.value)}><option value="all">All kinds</option>{kinds.map((value) => <option value={value} key={value}>{value}</option>)}</select><button className="secondary-button" type="button" onClick={() => setDialog("entity")}><Plus size={16} />Add entity</button></div></div>
            {entities.length === 0 ? <div className="section-empty">No matching entities.</div> : <div className="entity-table" role="table"><div className="table-header" role="row"><span>Entity</span><span>Kind</span><span>Status</span><span>Revision</span></div>{entities.map((entity) => <div className="entity-row" role="row" key={entity.id}><div><strong>{entity.title}</strong><code>{entity.id}</code></div><span>{entity.kind}</span><span className="entity-status">{entity.status}</span><span>r{entity.revision}</span></div>)}</div>}
          </section>
          <section className="model-section"><div className="section-heading"><div><h2>Relations</h2><p>Typed edges preserving model traceability.</p></div><button className="secondary-button" type="button" disabled={snapshot.data.entities.length < 2} onClick={() => setDialog("relation")}><Plus size={16} />Add relation</button></div>{snapshot.data.relations.length === 0 ? <div className="section-empty">No relations in this revision.</div> : <div className="relation-list">{snapshot.data.relations.map((relation) => <div className="relation-row" key={relation.id}><code>{relation.from_id}</code><strong>{relation.type}</strong><code>{relation.to_id}</code><p>{relation.rationale}</p></div>)}</div>}</section>
          {dialog === "entity" && <EntityDialog projectID={id} revision={snapshot.data.project.revision} onClose={() => setDialog(null)} />}
          {dialog === "relation" && <RelationDialog projectID={id} revision={snapshot.data.project.revision} entities={snapshot.data.entities} onClose={() => setDialog(null)} />}
          {dialog === "rename" && <RenameDialog projectID={id} revision={snapshot.data.project.revision} name={snapshot.data.project.name} onClose={() => setDialog(null)} />}
          {dialog === "delete" && <DeleteDialog projectID={id} revision={snapshot.data.project.revision} onClose={() => setDialog(null)} />}
        </>
      )}
    </AppShell>
  );
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
  return <Routes><Route path="/projects" element={<ProjectList />} /><Route path="/projects/:id/model" element={<ModelView />} /><Route path="*" element={<Navigate to="/projects" replace />} /></Routes>;
}
