import { useEffect, useState } from "react";
import { ApiError, ProjectDetail, getProject, isSessionExpired } from "../../lib/apiClient";
import Overview from "./Overview";
import Members from "./Members";
import ProjectChat from "./ProjectChat";
import ProjectTasks from "./ProjectTasks";
import DeveloperTools from "./DeveloperTools";
import GitSection from "./GitSection";
import FilesSection from "./FilesSection";
import { useProjectTaskEvents } from "../presence/PresenceProvider";
import WorkContextPanel from "../work-context/WorkContextPanel";
import "./ProjectWorkspace.css";

interface ProjectWorkspaceProps {
  serverUrl: string;
  token: string;
  projectId: string;
  viewerUserId: string;
  onBackToServerHome: () => void;
  onSessionExpired: () => void;
}

type Section = "overview" | "chat" | "tasks" | "members" | "git" | "files" | "developerTools";

// Sidebar navigation per docs/UX.md "Project Workspace".
const SIDEBAR_ITEMS: { section: Section; label: string }[] = [
  { section: "overview", label: "Overview" },
  { section: "chat", label: "Chat" },
  { section: "tasks", label: "Tasks" },
  { section: "members", label: "Members" },
  { section: "git", label: "Git" },
  { section: "files", label: "Files" },
  { section: "developerTools", label: "Developer Tools" },
];

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

  // Role changes and ownership transfer change Project detail (including
  // the viewer's own role), so reload it from the Server afterwards.
  function reloadDetail() {
    getProject(serverUrl, token, projectId)
      .then(setDetail)
      .catch((err) => {
        if (isSessionExpired(err)) {
          onSessionExpired();
          return;
        }
        setError(err instanceof ApiError ? err.message : "Could not load this project.");
      });
  }

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
          {SIDEBAR_ITEMS.map((item) => (
            <button
              key={item.section}
              type="button"
              className={
                section === item.section
                  ? "project-workspace__nav-item project-workspace__nav-item--active"
                  : "project-workspace__nav-item"
              }
              aria-current={section === item.section ? "page" : undefined}
              onClick={() => setSection(item.section)}
            >
              {item.label}
            </button>
          ))}
        </nav>
      </aside>

      <main className="project-workspace__content">
		{detail&&<WorkContextPanel serverUrl={serverUrl} token={token} projectId={projectId} viewerUserId={viewerUserId}/>}
        {error && (
          <p className="project-workspace__error" role="alert">
            {error}
          </p>
        )}
        {!error && !detail && <p className="project-workspace__loading">Loading project...</p>}
        {detail && section === "overview" && <Overview detail={detail} serverUrl={serverUrl} token={token} onViewMembers={() => setSection("members")} onSessionExpired={onSessionExpired} onProjectClosed={onBackToServerHome} />}
        {detail && section === "chat" && (
          <ProjectChat detail={detail} serverUrl={serverUrl} token={token} viewerUserId={viewerUserId} onSessionExpired={onSessionExpired} />
        )}
        {detail && section === "tasks" && <ProjectTasks detail={detail} serverUrl={serverUrl} token={token} viewerUserId={viewerUserId} onSessionExpired={onSessionExpired} />}
        {detail && section === "git" && (
          <GitSection
            serverUrl={serverUrl}
            token={token}
            projectId={projectId}
            viewerUserId={viewerUserId}
            viewerRole={detail.role}
            onSessionExpired={onSessionExpired}
            onOpenOverview={() => setSection("overview")}
            onOpenDeveloperTools={() => setSection("developerTools")}
          />
        )}
        {detail && section === "files" && (
          <FilesSection
            serverUrl={serverUrl}
            token={token}
            projectId={projectId}
            onSessionExpired={onSessionExpired}
            onOpenChat={() => setSection("chat")}
          />
        )}
        {detail && section === "developerTools" && <DeveloperTools projectId={projectId} />}
        {detail && section === "members" && (
          <Members
            detail={detail}
            serverUrl={serverUrl}
            token={token}
            viewerUserId={viewerUserId}
            onMembershipChanged={reloadDetail}
            onSessionExpired={onSessionExpired}
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
