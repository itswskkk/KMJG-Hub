import { useEffect, useState } from "react";
import { ApiError, FriendRequest, acceptRequest, declineRequest, isSessionExpired, listIncomingRequests } from "../../lib/apiClient";
import { useFriendEvents } from "../presence/PresenceProvider";
import "./Friends.css";

interface FriendRequestsProps {
  serverUrl: string;
  token: string;
  onClose: () => void;
  /** Called after a request is accepted or declined, so the caller can refresh its own lists. */
  onChanged: () => void;
  onSessionExpired: () => void;
}

/** Overlay listing incoming friend requests with Accept / Decline actions. */
function FriendRequests({ serverUrl, token, onClose, onChanged, onSessionExpired }: FriendRequestsProps) {
  const [requests, setRequests] = useState<FriendRequest[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const friendEvents = useFriendEvents();
  const latestFriendEvent = friendEvents[friendEvents.length - 1];

  useEffect(() => {
    let cancelled = false;
    listIncomingRequests(serverUrl, token)
      .then((items) => { if (!cancelled) setRequests(items); })
      .catch((err) => {
        if (cancelled) return;
        if (isSessionExpired(err)) { onSessionExpired(); return; }
        setError(err instanceof ApiError ? err.message : "Could not load friend requests.");
      });
    return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverUrl, token, latestFriendEvent]);

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  async function respond(item: FriendRequest, accept: boolean) {
    setBusyId(item.id);
    setError(null);
    try {
      if (accept) {
        await acceptRequest(serverUrl, token, item.id);
      } else {
        await declineRequest(serverUrl, token, item.id);
      }
      setRequests((current) => (current ?? []).filter((entry) => entry.id !== item.id));
      onChanged();
    } catch (err) {
      if (isSessionExpired(err)) { onSessionExpired(); return; }
      setError(err instanceof ApiError ? err.message : "Could not respond to this friend request.");
    } finally {
      setBusyId(null);
    }
  }

  return (
    <div className="friends-overlay" role="dialog" aria-modal="true" aria-labelledby="friend-requests-title" onClick={onClose}>
      <div className="friends-overlay__panel" onClick={(event) => event.stopPropagation()}>
        <div className="friends-overlay__header">
          <h2 id="friend-requests-title">Friend Requests</h2>
          <button type="button" onClick={onClose} aria-label="Close friend requests">Close</button>
        </div>
        {error && <p className="server-home__error" role="alert">{error}</p>}
        {requests === null && !error && <p className="server-home__loading">Loading requests...</p>}
        {requests !== null && requests.length === 0 && <p className="server-home__loading">No pending friend requests.</p>}
        {requests !== null && requests.length > 0 && (
          <ul className="server-home__project-list">
            {requests.map((item) => (
              <li key={item.id} className="server-home__project-card">
                <div>
                  <h3>{item.sender_username}</h3>
                  <p className="server-home__project-meta">Sent {new Date(item.created_at).toLocaleString()}</p>
                </div>
                <div className="server-home__actions">
                  <button type="button" onClick={() => respond(item, false)} disabled={busyId === item.id}>Decline</button>
                  <button type="button" onClick={() => respond(item, true)} disabled={busyId === item.id}>Accept</button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

export default FriendRequests;
