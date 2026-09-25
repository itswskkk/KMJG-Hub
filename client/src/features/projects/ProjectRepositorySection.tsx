import { useEffect, useState } from "react";
import {
  ApiError,
  GitHubRepository,
  ProjectRepositoryInfo,
  connectRepository,
  disconnectRepository,
  getProjectRepository,
  isSessionExpired,
  listAvailableRepos,
  updateGitHubNotificationConfig,
} from "../../lib/apiClient";
import { openExternalUrl } from "../../lib/nativeClient";

interface ProjectRepositorySectionProps {
  serverUrl: string;
  token: string;
  projectId: string;
  viewerRole: string;
  onSessionExpired: () => void;
}

/**
 * The Project's Git repository, per docs/PRD.md "Connecting a Repository
 * Later": the Owner or an Admin may connect one repository (chosen from
 * their own GitHub account's accessible repositories) or disconnect it;
 * every member sees the connected repository read-only. Push notification
 * settings are Owner-only ("The Project Owner may configure"). A fuller
 * Project Settings screen replaces this in the Project Lifecycle work.
 */
function ProjectRepositorySection({ serverUrl, token, projectId, viewerRole, onSessionExpired }: ProjectRepositorySectionProps) {
  const canManage = viewerRole === "owner" || viewerRole === "admin";
  const isOwner = viewerRole === "owner";

  const [info, setInfo] = useState<ProjectRepositoryInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [picking, setPicking] = useState(false);
  const [available, setAvailable] = useState<GitHubRepository[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);

  function handleError(err: unknown, fallback: string) {
    if (isSessionExpired(err)) {
      onSessionExpired();
      return;
    }
    if (err instanceof ApiError && err.code === "github_not_connected") {
      setError("Connect your GitHub account from your Profile first.");
      return;
    }
    setError(err instanceof ApiError ? err.message : fallback);
  }

  useEffect(() => {
    let cancelled = false;
    setInfo(null);
    setError(null);
    getProjectRepository(serverUrl, token, projectId)
      .then((loaded) => {
        if (!cancelled) setInfo(loaded);
      })
      .catch((err) => {
        if (!cancelled) handleError(err, "Could not load the Project repository.");
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverUrl, token, projectId]);

  async function openPicker() {
    setError(null);
    setPicking(true);
    setAvailable(null);
    try {
      setAvailable(await listAvailableRepos(serverUrl, token));
    } catch (err) {
      setPicking(false);
      handleError(err, "Could not list your GitHub repositories.");
    }
  }

  async function handleConnect(repo: GitHubRepository) {
    setError(null);
    setBusy(true);
    try {
      const connected = await connectRepository(serverUrl, token, projectId, repo.external_repo_id);
      setInfo((current) => ({ configured: current?.configured ?? true, repository: connected }));
      setPicking(false);
    } catch (err) {
      handleError(err, "Could not connect the repository.");
    } finally {
      setBusy(false);
    }
  }

  async function handleDisconnect() {
    if (!window.confirm("Disconnect this repository from the Project? The repository on GitHub is not changed.")) return;
    setError(null);
    setBusy(true);
    try {
      await disconnectRepository(serverUrl, token, projectId);
      setInfo((current) => ({ configured: current?.configured ?? true, repository: null }));
    } catch (err) {
      handleError(err, "Could not disconnect the repository.");
    } finally {
      setBusy(false);
    }
  }

  async function handleNotificationChange(change: Partial<{ post_to_chat: boolean; all_members: boolean; all_branches: boolean }>) {
    const repo = info?.repository;
    if (!repo) return;
    setError(null);
    setSaved(false);
    setBusy(true);
    try {
      const updated = await updateGitHubNotificationConfig(serverUrl, token, projectId, {
        post_to_chat: repo.post_pushes_to_chat,
        all_members: repo.notify_all_members,
        all_branches: repo.notify_all_branches,
        ...change,
      });
      setInfo((current) => ({ configured: current?.configured ?? true, repository: updated }));
      setSaved(true);
    } catch (err) {
      handleError(err, "Could not update notification settings.");
    } finally {
      setBusy(false);
    }
  }

  const repo = info?.repository ?? null;

  return (
    <section className="overview__section">
      <h2>Repository</h2>
      {info === null && !error && <p className="overview__note">Loading repository...</p>}

      {repo && (
        <>
          <p>
            <a
              href={repo.html_url}
              onClick={(event) => {
                event.preventDefault();
                void openExternalUrl(repo.html_url);
              }}
            >
              {repo.full_name}
            </a>{" "}
            · default branch <code>{repo.default_branch}</code>
          </p>
          <p className="overview__note">
            Project membership does not grant repository access; access is managed on GitHub.
          </p>
          {isOwner && (
            <fieldset className="overview__settings" disabled={busy}>
              <legend>Push notifications</legend>
              <label>
                <input type="checkbox" checked={repo.notify_all_members} onChange={(e) => void handleNotificationChange({ all_members: e.target.checked })} />{" "}
                Notify all Project members
              </label>
              <label>
                <input type="checkbox" checked={repo.notify_all_branches} onChange={(e) => void handleNotificationChange({ all_branches: e.target.checked })} />{" "}
                Pushes to all branches (otherwise only <code>{repo.default_branch}</code>)
              </label>
              <label>
                <input type="checkbox" checked={repo.post_pushes_to_chat} onChange={(e) => void handleNotificationChange({ post_to_chat: e.target.checked })} />{" "}
                Post pushes to Project Chat
              </label>
              {saved && <p className="overview__note">Saved.</p>}
            </fieldset>
          )}
          {canManage && (
            <button type="button" onClick={() => void handleDisconnect()} disabled={busy}>
              Disconnect Repository
            </button>
          )}
        </>
      )}

      {info && !repo && (
        <>
          <p className="overview__note">No Repository Connected.</p>
          {canManage && !info.configured && <p className="overview__note">GitHub integration is not configured on this server.</p>}
          {canManage && info.configured && !picking && (
            <button type="button" onClick={() => void openPicker()}>
              Connect GitHub Repository
            </button>
          )}
          {picking && (
            <div className="overview__picker">
              {available === null && <p className="overview__note">Loading your repositories...</p>}
              {available?.length === 0 && <p className="overview__note">Your GitHub account has no accessible repositories.</p>}
              {available && available.length > 0 && (
                <ul>
                  {available.map((candidate) => (
                    <li key={candidate.external_repo_id}>
                      {candidate.full_name} {candidate.private && <span className="overview__note">(private)</span>}{" "}
                      <button type="button" onClick={() => void handleConnect(candidate)} disabled={busy}>
                        Connect
                      </button>
                    </li>
                  ))}
                </ul>
              )}
              <button type="button" onClick={() => setPicking(false)} disabled={busy}>
                Cancel
              </button>
            </div>
          )}
        </>
      )}

      {error && (
        <p className="overview__error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}

export default ProjectRepositorySection;
