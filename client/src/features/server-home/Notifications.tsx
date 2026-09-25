import { useEffect, useState } from "react";
import {
  ApiError,
  Notification,
  deleteNotification,
  isSessionExpired,
  listNotifications,
  markNotificationRead,
} from "../../lib/apiClient";
import { useNotificationEvents } from "../presence/PresenceProvider";
import "./Notifications.css";

interface NotificationsProps {
  serverUrl: string;
  token: string;
  onBack: () => void;
  onOpenProject: (projectId: string) => void;
  onOpenFriends: () => void;
  onOpenMessages: () => void;
  onSessionExpired: () => void;
}

function text(payload: Record<string, unknown>, key: string): string {
  const value = payload[key];
  return typeof value === "string" ? value : "";
}

/** Human-readable summary of a notification. Only uses payload fields as
 * display context; opening the target re-checks access on the Server. */
export function describeNotification(n: Notification): string {
  const p = n.payload ?? {};
  switch (n.event_type) {
    case "task_assigned":
      return `${text(p, "assigned_by_username") || "Someone"} asked you to take the task “${text(p, "task_title")}”`;
    case "task_comment":
      return `${text(p, "author_username") || "Someone"} commented on “${text(p, "task_title")}”: ${text(p, "body_preview")}`;
    case "direct_message":
      return `${text(p, "sender_username") || "Someone"} sent you a message: ${text(p, "body_preview")}`;
    case "friend_request":
      return `${text(p, "sender_username") || "Someone"} sent you a friend request`;
    case "project_invitation":
      return `${text(p, "inviter_username") || "Someone"} invited you to the Project “${text(p, "project_name")}”`;
    case "file_transfer_request":
      return "You have a new file transfer request";
    case "role_changed":
      return "Your Project role was changed";
    case "git_push":
      return "New commits were pushed to a Project repository";
    default:
      return "New notification";
  }
}

function Notifications({ serverUrl, token, onBack, onOpenProject, onOpenFriends, onOpenMessages, onSessionExpired }: NotificationsProps) {
  const [items, setItems] = useState<Notification[] | null>(null);
  const [nextCursor, setNextCursor] = useState("");
  const [unreadCount, setUnreadCount] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);
  const events = useNotificationEvents();
  const latestEvent = events[events.length - 1];

  function fail(err: unknown, fallback: string) {
    if (isSessionExpired(err)) { onSessionExpired(); return; }
    setError(err instanceof ApiError ? err.message : fallback);
  }

  useEffect(() => {
    let cancelled = false;
    listNotifications(serverUrl, token)
      .then((page) => {
        if (cancelled) return;
        setItems(page.notifications);
        setNextCursor(page.next_cursor);
        setUnreadCount(page.unread_count);
      })
      .catch((err) => { if (!cancelled) fail(err, "Could not load notifications."); });
    return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverUrl, token]);

  // Live delivery: prepend newly created notifications without refetching.
  useEffect(() => {
    // While the initial list is still loading, that fetch already covers it.
    if (!latestEvent || items === null || items.some((n) => n.id === latestEvent.id)) return;
    setItems([latestEvent as Notification, ...items]);
    if (!latestEvent.read_at) setUnreadCount((count) => count + 1);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [latestEvent]);

  async function loadMore() {
    if (!nextCursor) return;
    setLoadingMore(true);
    try {
      const page = await listNotifications(serverUrl, token, nextCursor);
      setItems((current) => {
        const known = new Set((current ?? []).map((n) => n.id));
        return [...(current ?? []), ...page.notifications.filter((n) => !known.has(n.id))];
      });
      setNextCursor(page.next_cursor);
      setUnreadCount(page.unread_count);
    } catch (err) {
      fail(err, "Could not load older notifications.");
    } finally {
      setLoadingMore(false);
    }
  }

  async function markRead(n: Notification) {
    if (n.read_at) return;
    setBusyId(n.id); setError(null);
    try {
      await markNotificationRead(serverUrl, token, n.id);
      const now = new Date().toISOString();
      setItems((current) => (current ?? []).map((entry) => (entry.id === n.id ? { ...entry, read_at: now } : entry)));
      setUnreadCount((count) => Math.max(0, count - 1));
    } catch (err) {
      fail(err, "Could not mark this notification as read.");
    } finally {
      setBusyId(null);
    }
  }

  async function dismiss(n: Notification) {
    setBusyId(n.id); setError(null);
    try {
      await deleteNotification(serverUrl, token, n.id);
      setItems((current) => (current ?? []).filter((entry) => entry.id !== n.id));
      if (!n.read_at) setUnreadCount((count) => Math.max(0, count - 1));
    } catch (err) {
      fail(err, "Could not dismiss this notification.");
    } finally {
      setBusyId(null);
    }
  }

  function openTarget(n: Notification): (() => void) | null {
    const projectId = text(n.payload ?? {}, "project_id");
    switch (n.event_type) {
      case "task_assigned":
      case "task_comment":
        return projectId ? () => onOpenProject(projectId) : null;
      case "direct_message":
        return onOpenMessages;
      case "friend_request":
        return onOpenFriends;
      case "project_invitation":
        // Pending invitations are accepted from Server Home.
        return onBack;
      default:
        return null;
    }
  }

  async function open(n: Notification, go: () => void) {
    await markRead(n);
    go();
  }

  return (
    <main className="server-home">
      <div className="server-home__header">
        <div>
          <h1>Notifications</h1>
          <p className="server-home__subtitle">{unreadCount > 0 ? `${unreadCount} unread` : "All caught up"}</p>
        </div>
        <div className="server-home__header-actions">
          <button type="button" onClick={onBack}>Back</button>
        </div>
      </div>

      {error && <p className="server-home__error" role="alert">{error}</p>}
      {items === null && !error && <p className="server-home__loading">Loading notifications...</p>}
      {items !== null && items.length === 0 && <p className="server-home__loading">You have no notifications.</p>}

      {items !== null && items.length > 0 && (
        <ul className="server-home__project-list">
          {items.map((n) => {
            const go = openTarget(n);
            return (
              <li key={n.id} className={`server-home__project-card notifications__item${n.read_at ? "" : " notifications__item--unread"}`}>
                <div>
                  <p className="notifications__summary">{describeNotification(n)}</p>
                  <p className="server-home__project-meta">{new Date(n.created_at).toLocaleString()}</p>
                </div>
                <div className="server-home__actions">
                  {go && <button type="button" disabled={busyId === n.id} onClick={() => void open(n, go)}>Open</button>}
                  {!n.read_at && <button type="button" disabled={busyId === n.id} onClick={() => void markRead(n)}>Mark Read</button>}
                  <button type="button" disabled={busyId === n.id} onClick={() => void dismiss(n)} aria-label="Dismiss notification">Dismiss</button>
                </div>
              </li>
            );
          })}
        </ul>
      )}

      {nextCursor && (
        <div className="server-home__actions">
          <button type="button" onClick={() => void loadMore()} disabled={loadingMore}>{loadingMore ? "Loading…" : "Load Older"}</button>
        </div>
      )}
    </main>
  );
}

export default Notifications;
