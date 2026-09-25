import { useEffect, useState } from "react";
import { ApiError, DeletedProject, DirectInvitation, ProjectSummary, acceptInvitation, declineInvitation, isSessionExpired, joinProjectWithInvite, listNotifications, listProjects, listReceivedInvitations, listDeletedProjects, logout, restoreProject } from "../../lib/apiClient";
import { useNotificationEvents } from "../presence/PresenceProvider";
import "./ServerHome.css";
import "./Notifications.css";

interface ServerHomeProps {
  serverUrl: string;
  token: string;
  username: string;
  onOpenProject: (projectId: string) => void;
  onCreateProject: () => void;
  onOpenProfile: () => void;
  onOpenFriends: () => void;
  onOpenMessages: () => void;
  onOpenNotifications: () => void;
  onOpenFileTransfers: () => void;
  onSessionExpired: () => void;
}

function ServerHome({ serverUrl, token, username, onOpenProject, onCreateProject, onOpenProfile, onOpenFriends, onOpenMessages, onOpenNotifications, onOpenFileTransfers, onSessionExpired }: ServerHomeProps) {
  const [projects, setProjects] = useState<ProjectSummary[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loggingOut, setLoggingOut] = useState(false);
  const [invitations, setInvitations] = useState<DirectInvitation[]>([]);
  const [invitationError, setInvitationError] = useState<string | null>(null);
  const [joinInput, setJoinInput] = useState("");
  const [joining, setJoining] = useState(false);
  const [unreadCount, setUnreadCount] = useState(0);
  const [deletedProjects, setDeletedProjects] = useState<DeletedProject[]>([]);
  const [restoringId, setRestoringId] = useState<string | null>(null);
  const [deletedError, setDeletedError] = useState<string | null>(null);
  const notificationEvents = useNotificationEvents();
  const latestNotification = notificationEvents[notificationEvents.length - 1];

  // Unread badge: the HTTP unread_count is authoritative; each live
  // notification.created event re-fetches it (and a new project_invitation
  // also refreshes the invitation list below).
  useEffect(() => {
    let cancelled = false;
    listNotifications(serverUrl, token)
      .then((page) => { if (!cancelled) setUnreadCount(page.unread_count); })
      .catch(() => { /* badge is best-effort; the list screen surfaces errors */ });
    return () => { cancelled = true; };
  }, [serverUrl, token, latestNotification]);

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

  // "Recently Deleted Projects" (docs/PRD.md "Project Recovery"): only the
  // Owner who deleted a Project sees it here, and the section is hidden
  // entirely when there is nothing to restore.
  useEffect(() => {
    let cancelled = false;
    listDeletedProjects(serverUrl, token)
      .then((items) => { if (!cancelled) setDeletedProjects(items); })
      .catch(() => { /* recovery list is best-effort; the main project list surfaces errors */ });
    return () => { cancelled = true; };
  }, [serverUrl, token]);

  async function restore(item: DeletedProject) {
    setRestoringId(item.id);
    setDeletedError(null);
    try {
      await restoreProject(serverUrl, token, item.id);
      setDeletedProjects((current) => current.filter((entry) => entry.id !== item.id));
      setProjects(await listProjects(serverUrl, token));
    } catch (err) {
      if (isSessionExpired(err)) { onSessionExpired(); return; }
      setDeletedError(err instanceof ApiError ? err.message : "Could not restore this project.");
    } finally {
      setRestoringId(null);
    }
  }

  const invitationTrigger = latestNotification?.event_type === "project_invitation" ? latestNotification.id : null;
  useEffect(() => {
    let cancelled = false;
    listReceivedInvitations(serverUrl, token)
      .then((items) => { if (!cancelled) setInvitations(items); })
      .catch((err) => { if (!cancelled) setInvitationError(err instanceof ApiError ? err.message : "Could not load invitations."); });
    return () => { cancelled = true; };
  }, [serverUrl, token, invitationTrigger]);

  async function respond(item: DirectInvitation, accept: boolean) {
    setInvitationError(null);
    try {
      if (accept) {
        const result = await acceptInvitation(serverUrl, token, item.id);
        onOpenProject(result.project_id);
      } else {
        await declineInvitation(serverUrl, token, item.id);
        setInvitations((current) => current.filter((entry) => entry.id !== item.id));
      }
    } catch (err) {
      if (isSessionExpired(err)) { onSessionExpired(); return; }
      setInvitationError(err instanceof ApiError ? err.message : "Could not respond to invitation.");
    }
  }

  async function joinProject() {
    setJoining(true); setInvitationError(null);
    try {
      const result = await joinProjectWithInvite(serverUrl, token, joinInput.trim());
      onOpenProject(result.project_id);
    } catch (err) {
      if (isSessionExpired(err)) { onSessionExpired(); return; }
      setInvitationError(err instanceof ApiError ? err.message : "Could not use this invite.");
    } finally { setJoining(false); }
  }

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
        <div className="server-home__header-actions">
          <button type="button" onClick={onOpenNotifications} aria-label={unreadCount > 0 ? `Notifications, ${unreadCount} unread` : "Notifications"}>
            Notifications
            {unreadCount > 0 && <span className="notifications__badge" aria-hidden="true">{unreadCount > 99 ? "99+" : unreadCount}</span>}
          </button>
          <button type="button" onClick={onOpenMessages}>
            Messages
          </button>
          <button type="button" onClick={onOpenFriends}>
            Friends
          </button>
          <button type="button" onClick={onOpenFileTransfers}>
            Files
          </button>
          <button type="button" onClick={onOpenProfile}>
            Your Profile
          </button>
          <button type="button" className="server-home__logout" onClick={handleLogout} disabled={loggingOut}>
            {loggingOut ? "Logging Out..." : "Log Out"}
          </button>
        </div>
      </div>

      <section className="server-home__projects">
        <h2>Project Invitations</h2>
        {invitationError && <p className="server-home__error" role="alert">{invitationError}</p>}
        {invitations.length === 0 ? <p className="server-home__loading">No pending invitations.</p> : (
          <ul className="server-home__project-list">
            {invitations.map((item) => (
              <li key={item.id} className="server-home__project-card">
                <div><h3>{item.project_name}</h3><p className="server-home__project-meta">Invited by {item.inviter_username}</p></div>
                <div className="server-home__actions"><button type="button" onClick={() => respond(item, false)}>Decline</button><button type="button" onClick={() => respond(item, true)}>Accept</button></div>
              </li>
            ))}
          </ul>
        )}

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
          <input aria-label="Invite link or code" placeholder="Invite link or code" value={joinInput} onChange={(event) => setJoinInput(event.target.value)} />
          <button type="button" onClick={joinProject} disabled={joining || joinInput.trim() === ""}>{joining ? "Joining…" : "Join Project"}</button>
        </div>

        {deletedProjects.length > 0 && (
          <>
            <h2>Recently Deleted Projects</h2>
            {deletedError && <p className="server-home__error" role="alert">{deletedError}</p>}
            <ul className="server-home__project-list">
              {deletedProjects.map((item) => (
                <li key={item.id} className="server-home__project-card">
                  <div>
                    <h3>{item.name}</h3>
                    <p className="server-home__project-meta">{restoreTimeLeft(item.restore_deadline)}</p>
                  </div>
                  <button type="button" onClick={() => restore(item)} disabled={restoringId !== null}>
                    {restoringId === item.id ? "Restoring…" : "Restore"}
                  </button>
                </li>
              ))}
            </ul>
          </>
        )}
      </section>
    </main>
  );
}

function restoreTimeLeft(deadline: string): string {
  const days = Math.ceil((new Date(deadline).getTime() - Date.now()) / (24 * 60 * 60 * 1000));
  if (days <= 1) return "Restorable for less than a day";
  return `Restorable for ${days} more days`;
}

export default ServerHome;
