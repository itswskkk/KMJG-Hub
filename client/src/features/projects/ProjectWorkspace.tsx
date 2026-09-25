import { useEffect, useState } from "react";
import { ApiError, ProjectDetail, getProject, isSessionExpired } from "../../lib/apiClient";
import Overview from "./Overview";
import Members from "./Members";
import ProjectChat from "./ProjectChat";
import ProjectTasks from "./ProjectTasks";
import { useProjectTaskEvents } from "../presence/PresenceProvider";
import "./ProjectWorkspace.css";

interface ProjectWorkspaceProps {
  serverUrl: string;
  token: string;
  projectId: string;
  viewerUserId: string;
  onBackToServerHome: () => void;
  onSessionExpired: () => void;
}

type Section = "overview" | "chat" | "tasks" | "members";

// Sidebar navigation per docs/UX.md "Project Workspace". Overview, Chat,
// and Members are implemented; the rest are real Project
// Workspace sections from the same UX spec, shown but disabled rather than
// omitted, the same treatment as the disabled GitHub login option.
const DISABLED_SIDEBAR_ITEMS = ["Git", "Files", "Developer Tools"];

function ProjectWorkspace({
  serverUrl,
  token,
  projectId,
  viewerUserId,
  onBackToServerHome,
  onSessionExpired,
}: ProjectWorkspaceProps) {
  const [detail, setDetail] = useState<ProjectDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [section, setSection] = useState<Section>("overview");
  const taskEvents = useProjectTaskEvents(projectId);

  useEffect(() => {
    let cancelled = false;
    setDetail(null);
    setError(null);
    setSection("overview");

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

  // Current Task is part of Project detail. A task event is only a hint;
  // reload the detail so Overview and Members reflect Server state.
  useEffect(() => {
    if (taskEvents.length === 0) return;
    let cancelled = false;
    getProject(serverUrl, token, projectId)
      .then((d) => { if (!cancelled) setDetail(d); })
      .catch((err) => {
        if (!cancelled && isSessionExpired(err)) onSessionExpired();
      });
    return () => { cancelled = true; };
  }, [taskEvents.length, serverUrl, token, projectId, onSessionExpired]);

  return (
    <div className="project-workspace">
      <aside className="project-workspace__sidebar">
        <button type="button" className="project-workspace__back" onClick={onBackToServerHome}>
          ← Server Home
        </button>

        <h1 className="project-workspace__title">{detail ? detail.name : "Loading..."}</h1>

        <nav className="project-workspace__nav">
          <button
            type="button"
            className={
              section === "overview"
                ? "project-workspace__nav-item project-workspace__nav-item--active"
                : "project-workspace__nav-item"
            }
            onClick={() => setSection("overview")}
          >
            Overview
          </button>
          <button
            type="button"
            className={
              section === "chat"
                ? "project-workspace__nav-item project-workspace__nav-item--active"
                : "project-workspace__nav-item"
            }
            onClick={() => setSection("chat")}
          >
            Chat
          </button>
          <button type="button" className={section === "tasks" ? "project-workspace__nav-item project-workspace__nav-item--active" : "project-workspace__nav-item"} onClick={() => setSection("tasks")}>
            Tasks
          </button>
          <button
            type="button"
            className={
              section === "members"
                ? "project-workspace__nav-item project-workspace__nav-item--active"
                : "project-workspace__nav-item"
            }
            onClick={() => setSection("members")}
          >
            Members
          </button>
          {DISABLED_SIDEBAR_ITEMS.map((item) => (
            <span
              key={item}
              className="project-workspace__nav-item project-workspace__nav-item--disabled"
              title="Not available yet"
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
        {detail && section === "overview" && <Overview detail={detail} onViewMembers={() => setSection("members")} />}
        {detail && section === "chat" && (
          <ProjectChat detail={detail} serverUrl={serverUrl} token={token} viewerUserId={viewerUserId} onSessionExpired={onSessionExpired} />
        )}
        {detail && section === "tasks" && <ProjectTasks detail={detail} serverUrl={serverUrl} token={token} viewerUserId={viewerUserId} onSessionExpired={onSessionExpired} />}
        {detail && section === "members" && (
          <Members
            detail={detail}
            serverUrl={serverUrl}
            token={token}
            viewerUserId={viewerUserId}
            onMemberRemoved={(userId) => {
              setDetail((current) =>
                current ? { ...current, members: current.members.filter((member) => member.id !== userId) } : current,
              );
            }}
          />
        )}
      </main>
    </div>
  );
}

export default ProjectWorkspace;
