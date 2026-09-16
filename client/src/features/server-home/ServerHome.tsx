import { useEffect, useState } from "react";
import { ApiError, ProjectSummary, isSessionExpired, listProjects, logout } from "../../lib/apiClient";
import "./ServerHome.css";

interface ServerHomeProps {
  serverUrl: string;
  token: string;
  username: string;
  onOpenProject: (projectId: string) => void;
  onCreateProject: () => void;
  onSessionExpired: () => void;
}

function ServerHome({ serverUrl, token, username, onOpenProject, onCreateProject, onSessionExpired }: ServerHomeProps) {
  const [projects, setProjects] = useState<ProjectSummary[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loggingOut, setLoggingOut] = useState(false);

  useEffect(() => {
    let cancelled = false;

    listProjects(serverUrl, token)
      .then((list) => {
        if (!cancelled) {
          setProjects(list);
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
        setError(err instanceof ApiError ? err.message : "Could not load your projects.");
      });

    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverUrl, token]);

  async function handleLogout() {
    setLoggingOut(true);
    try {
      await logout(serverUrl, token);
    } catch {
      // Logging out best-effort: even if the request fails (already
      // expired, network hiccup), the Client still forgets the token.
    } finally {
      onSessionExpired();
    }
  }

  return (
    <main className="server-home">
      <div className="server-home__header">
        <div>
          <h1>{serverUrl}</h1>
          <p className="server-home__subtitle">Signed in as {username}</p>
        </div>
        <button type="button" className="server-home__logout" onClick={handleLogout} disabled={loggingOut}>
          {loggingOut ? "Logging Out..." : "Log Out"}
        </button>
      </div>

      <section className="server-home__projects">
        <h2>Your Projects</h2>

        {error && (
          <p className="server-home__error" role="alert">
            {error}
          </p>
        )}

        {projects === null && !error && <p className="server-home__loading">Loading projects...</p>}

        {projects !== null && projects.length === 0 && (
          <div className="server-home__empty">
            <p>You don&apos;t have any Projects yet.</p>
          </div>
        )}

        {projects !== null && projects.length > 0 && (
          <ul className="server-home__project-list">
            {projects.map((project) => (
              <li key={project.id} className="server-home__project-card">
                <div>
                  <h3>{project.name}</h3>
                  <p className="server-home__project-meta">
                    {project.member_count} {project.member_count === 1 ? "Member" : "Members"}
                  </p>
                </div>
                <button type="button" onClick={() => onOpenProject(project.id)}>
                  Open
                </button>
              </li>
            ))}
          </ul>
        )}

        <div className="server-home__actions">
          <button type="button" onClick={onCreateProject}>
            + Create Project
          </button>
          <button type="button" disabled title="Joining a Project by invite is not available yet">
            Join Project (Not Available Yet)
          </button>
        </div>
      </section>
    </main>
  );
}

export default ServerHome;
