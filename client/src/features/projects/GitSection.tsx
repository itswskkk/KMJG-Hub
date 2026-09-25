import { useEffect, useState } from "react";
import { ApiError, ProjectRepositoryInfo, getProjectRepository, isSessionExpired } from "../../lib/apiClient";
import { openExternalUrl } from "../../lib/nativeClient";
import { useProjectWorkContexts } from "../work-context/useProjectWorkContexts";
import "./GitSection.css";

interface GitSectionProps {
  serverUrl: string;
  token: string;
  projectId: string;
  viewerUserId: string;
  viewerRole: string;
  onSessionExpired: () => void;
  /** Repository connection and push-notification settings live in Overview. */
  onOpenOverview: () => void;
  onOpenDeveloperTools: () => void;
}

/**
 * Git section, per docs/UX.md "Git Experience": repository awareness and
 * quick access to the user's own tools. It is read-only — connecting a
 * repository and the Owner-only push notification settings stay in
 * Overview's Repository section (ProjectRepositorySection) rather than
 * being duplicated here.
 */
function GitSection({
  serverUrl,
  token,
  projectId,
  viewerUserId,
  viewerRole,
  onSessionExpired,
  onOpenOverview,
  onOpenDeveloperTools,
}: GitSectionProps) {
  const [info, setInfo] = useState<ProjectRepositoryInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { contexts } = useProjectWorkContexts(serverUrl, token, projectId);
  const branch = contexts[viewerUserId]?.current_branch;
  const canManage = viewerRole === "owner" || viewerRole === "admin";
  const isOwner = viewerRole === "owner";

  useEffect(() => {
    let cancelled = false;
    setInfo(null);
    setError(null);
    getProjectRepository(serverUrl, token, projectId)
      .then((loaded) => {
        if (!cancelled) setInfo(loaded);
      })
      .catch((err) => {
        if (cancelled) return;
        if (isSessionExpired(err)) {
          onSessionExpired();
          return;
        }
        setError(err instanceof ApiError ? err.message : "Could not load the Project repository.");
      });
    return () => {
      cancelled = true;
    };
  }, [serverUrl, token, projectId, onSessionExpired]);

  const repo = info?.repository ?? null;

  return (
    <section className="git-section">
      <h1>Git</h1>

      {error && (
        <p className="git-section__error" role="alert">
          {error}
        </p>
      )}
      {!error && info === null && <p className="git-section__note">Loading repository...</p>}

      {info && !repo && (
        <div className="git-section__block">
          <h2>No Repository Connected</h2>
          <p>This Project is not connected to a Git repository yet.</p>
          {canManage ? (
            <button type="button" onClick={onOpenOverview}>
              Connect Repository in Overview
            </button>
          ) : (
            <p className="git-section__note">A Project Owner or Admin can connect a repository from Overview.</p>
          )}
        </div>
      )}

      {repo && (
        <>
          <div className="git-section__block">
            <h2>Repository</h2>
            <p className="git-section__repo">{repo.full_name}</p>
            <dl className="git-section__facts">
              <dt>Provider</dt>
              <dd>GitHub</dd>
              <dt>Default branch</dt>
              <dd>
                <code>{repo.default_branch}</code>
              </dd>
            </dl>
            <p className="git-section__note">
              Project membership does not grant repository access; access is managed on GitHub.
            </p>
          </div>

          <div className="git-section__block">
            <h2>Your Local Status</h2>
            <dl className="git-section__facts">
              <dt>Branch</dt>
              <dd>{branch ? <code>{branch}</code> : <span className="git-section__note">Not reported</span>}</dd>
            </dl>
            {!branch && (
              <p className="git-section__note">
                Your current branch appears here once you select a local repository in the work status bar (desktop app).
              </p>
            )}
          </div>

          <div className="git-section__block">
            <h2>Recent Activity</h2>
            <p className="git-section__note">
              {repo.post_pushes_to_chat
                ? "Pushes are posted to Project Chat as Git activity."
                : "Pushes are not posted to Project Chat for this Project."}{" "}
              {repo.notify_all_branches ? "All branches are included." : `Only pushes to ${repo.default_branch} are included.`}
            </p>
            {isOwner && (
              <button type="button" className="git-section__link" onClick={onOpenOverview}>
                Configure push notifications in Overview
              </button>
            )}
          </div>

          <div className="git-section__actions">
            <button type="button" onClick={() => void openExternalUrl(repo.html_url)}>
              Open Repository
            </button>
            <button type="button" onClick={onOpenDeveloperTools}>
              Open in Developer Tools
            </button>
          </div>
        </>
      )}
    </section>
  );
}

export default GitSection;
