import { useEffect, useState } from "react";
import { ApiError, ProjectDetail, getProject, isSessionExpired } from "../../lib/apiClient";
import Overview from "./Overview";
import "./ProjectWorkspace.css";

interface ProjectWorkspaceProps {
  serverUrl: string;
  token: string;
  projectId: string;
  onBackToServerHome: () => void;
  onSessionExpired: () => void;
}

// Sidebar navigation per docs/UX.md "Project Workspace". Only Overview is
// implemented this checkpoint; the rest are real Project Workspace sections
// from the same UX spec, shown but disabled rather than omitted, the same
// treatment as the disabled GitHub login option.
const SIDEBAR_ITEMS = ["Overview", "Chat", "Tasks", "Git", "Files", "Members", "Developer Tools"];

function ProjectWorkspace({ serverUrl, token, projectId, onBackToServerHome, onSessionExpired }: ProjectWorkspaceProps) {
  const [detail, setDetail] = useState<ProjectDetail | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setDetail(null);
    setError(null);

    getProject(serverUrl, token, projectId)
      .then((d) => {
        if (!cancelled) {
          setDetail(d);
        }
      })
      .catch((err) => {
        if (cancelled) {
          return;
        }
        if (isSessionExpired(err)) {
          onSessionExpired();
          return;
        }
        setError(err instanceof ApiError ? err.message : "Could not load this project.");
      });

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverUrl, token, projectId]);

  return (
    <div className="project-workspace">
      <aside className="project-workspace__sidebar">
        <button type="button" className="project-workspace__back" onClick={onBackToServerHome}>
          ← Server Home
        </button>

        <h1 className="project-workspace__title">{detail ? detail.name : "Loading..."}</h1>

        <nav className="project-workspace__nav">
          {SIDEBAR_ITEMS.map((item) => (
            <span
              key={item}
              className={
                item === "Overview"
                  ? "project-workspace__nav-item project-workspace__nav-item--active"
                  : "project-workspace__nav-item project-workspace__nav-item--disabled"
              }
              title={item === "Overview" ? undefined : "Not available yet"}
            >
              {item}
            </span>
          ))}
        </nav>
      </aside>

      <main className="project-workspace__content">
        {error && (
          <p className="project-workspace__error" role="alert">
            {error}
          </p>
        )}
        {!error && !detail && <p className="project-workspace__loading">Loading project...</p>}
        {detail && <Overview detail={detail} />}
      </main>
    </div>
  );
}

export default ProjectWorkspace;
