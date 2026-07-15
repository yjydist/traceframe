import { useQuery } from "@tanstack/react-query";
import { Activity, FolderKanban } from "lucide-react";
import { Navigate, Route, Routes } from "react-router-dom";

type Health = {
  status: "ok" | "unavailable";
  database: "ok" | "unavailable";
};

async function getHealth(): Promise<Health> {
  const response = await fetch("/api/v1/health");
  if (!response.ok) {
    throw new Error("Service unavailable");
  }
  return response.json() as Promise<Health>;
}

function ProjectList() {
  const health = useQuery({ queryKey: ["health"], queryFn: getHealth });
  const connected = health.data?.status === "ok";

  return (
    <div className="app-shell">
      <header className="topbar">
        <a className="brand" href="/projects" aria-label="Traceframe projects">
          <span className="brand-mark" aria-hidden="true">T</span>
          <span>Traceframe</span>
        </a>
        <div className="service-status" role="status">
          <span className={connected ? "status-dot connected" : "status-dot"} />
          {health.isPending ? "Connecting" : connected ? "Local service ready" : "Service unavailable"}
        </div>
      </header>

      <aside className="navigation" aria-label="Primary navigation">
        <a className="nav-item active" href="/projects" aria-current="page">
          <FolderKanban size={18} aria-hidden="true" />
          <span>Projects</span>
        </a>
      </aside>

      <main className="workspace">
        <div className="workspace-heading">
          <div>
            <p className="eyebrow">Workspace</p>
            <h1>Projects</h1>
          </div>
          <div className="connection-detail">
            <Activity size={17} aria-hidden="true" />
            <span>Local workspace</span>
          </div>
        </div>

        <section className="empty-state" aria-labelledby="empty-title">
          <div className="empty-icon" aria-hidden="true">
            <FolderKanban size={28} />
          </div>
          <h2 id="empty-title">No projects yet</h2>
          <p>Your design projects will appear here.</p>
        </section>
      </main>
    </div>
  );
}

export function App() {
  return (
    <Routes>
      <Route path="/projects" element={<ProjectList />} />
      <Route path="*" element={<Navigate to="/projects" replace />} />
    </Routes>
  );
}
