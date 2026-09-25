import { FormEvent, useCallback, useEffect, useState } from "react";
import {
  ApiError,
  BlockedUser,
  Friend,
  FriendRequest,
  block,
  blockByUsername,
  cancelRequest,
  isSessionExpired,
  listBlocked,
  listFriends,
  listIncomingRequests,
  listOutgoingRequests,
  removeFriend,
  sendFriendRequest,
  unblock,
} from "../../lib/apiClient";
import { useFriendEvents } from "../presence/PresenceProvider";
import FriendRequests from "./FriendRequests";
import SendFileButton from "./SendFileButton";
import "./ServerHome.css";
import "./Friends.css";

interface FriendsProps {
  serverUrl: string;
  token: string;
  onBack: () => void;
  onSessionExpired: () => void;
}

interface FriendsData {
  friends: Friend[];
  incoming: FriendRequest[];
  outgoing: FriendRequest[];
  blocked: BlockedUser[];
}

/** Server Home's Friends section: friends list, requests, and block management. */
function Friends({ serverUrl, token, onBack, onSessionExpired }: FriendsProps) {
  const [data, setData] = useState<FriendsData | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [showRequests, setShowRequests] = useState(false);
  const [recipient, setRecipient] = useState("");
  const [sending, setSending] = useState(false);
  const [blockTarget, setBlockTarget] = useState("");
  const [reloadKey, setReloadKey] = useState(0);
  const friendEvents = useFriendEvents();
  const latestFriendEvent = friendEvents[friendEvents.length - 1];

  const reload = useCallback(() => setReloadKey((k) => k + 1), []);

  useEffect(() => {
    let cancelled = false;
    Promise.all([
      listFriends(serverUrl, token),
      listIncomingRequests(serverUrl, token),
      listOutgoingRequests(serverUrl, token),
      listBlocked(serverUrl, token),
    ])
      .then(([friends, incoming, outgoing, blocked]) => {
        if (!cancelled) setData({ friends, incoming, outgoing, blocked });
      })
      .catch((err) => {
        if (cancelled) return;
        if (isSessionExpired(err)) { onSessionExpired(); return; }
        setError(err instanceof ApiError ? err.message : "Could not load your friends.");
      });
    return () => { cancelled = true; };
    // Real-time friend events are reload hints: refetch whenever one arrives.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverUrl, token, reloadKey, latestFriendEvent]);

  async function run(action: () => Promise<unknown>, failure: string, success?: string) {
    setError(null);
    setNotice(null);
    try {
      await action();
      if (success) setNotice(success);
      reload();
      return true;
    } catch (err) {
      if (isSessionExpired(err)) { onSessionExpired(); return false; }
      setError(err instanceof ApiError ? err.message : failure);
      return false;
    }
  }

  async function handleSend(event: FormEvent) {
    event.preventDefault();
    const target = recipient.trim();
    if (!target) return;
    setSending(true);
    const ok = await run(() => sendFriendRequest(serverUrl, token, target), "Could not send the friend request.", `Friend request sent to ${target}.`);
    if (ok) setRecipient("");
    setSending(false);
  }

  function handleRemove(friend: Friend) {
    if (!window.confirm(`Remove ${friend.username} from your friends?`)) return;
    void run(() => removeFriend(serverUrl, token, friend.user_id), "Could not remove this friend.");
  }

  function handleBlock(userId: string, username: string) {
    if (!window.confirm(`Block ${username}? This removes your friendship and prevents friend requests between you.`)) return;
    void run(() => block(serverUrl, token, userId), "Could not block this user.", `${username} is blocked.`);
  }

  async function handleBlockByName(event: FormEvent) {
    event.preventDefault();
    const target = blockTarget.trim();
    if (!target) return;
    if (!window.confirm(`Block ${target}? This removes any friendship and prevents friend requests between you.`)) return;
    const ok = await run(() => blockByUsername(serverUrl, token, target), "Could not block this user.", `${target} is blocked.`);
    if (ok) setBlockTarget("");
  }

  const incomingCount = data?.incoming.length ?? 0;

  return (
    <main className="server-home">
      <div className="server-home__header">
        <div>
          <h1>Friends</h1>
          <p className="server-home__subtitle">{serverUrl}</p>
        </div>
        <div className="server-home__header-actions">
          <button type="button" onClick={() => setShowRequests(true)}>
            Requests{incomingCount > 0 ? ` (${incomingCount})` : ""}
          </button>
          <button type="button" onClick={onBack}>Back</button>
        </div>
      </div>

      {error && <p className="server-home__error" role="alert">{error}</p>}
      {notice && <p className="friends__notice" role="status">{notice}</p>}

      <section className="server-home__projects">
        <h2>Add a Friend</h2>
        <form className="server-home__actions" onSubmit={handleSend}>
          <input aria-label="Username or email" placeholder="Username or email" value={recipient} onChange={(event) => setRecipient(event.target.value)} />
          <button type="submit" disabled={sending || recipient.trim() === ""}>{sending ? "Sending…" : "Send Request"}</button>
        </form>

        <h2>Your Friends</h2>
        {data === null && !error && <p className="server-home__loading">Loading friends...</p>}
        {data !== null && data.friends.length === 0 && <p className="server-home__loading">You haven&apos;t added any friends yet.</p>}
        {data !== null && data.friends.length > 0 && (
          <ul className="server-home__project-list">
            {data.friends.map((friend) => (
              <li key={friend.user_id} className="server-home__project-card">
                <div>
                  <h3>{friend.username}</h3>
                  <p className="server-home__project-meta">Friends since {new Date(friend.since).toLocaleDateString()}</p>
                </div>
                <div className="server-home__actions">
                  <SendFileButton
                    serverUrl={serverUrl}
                    token={token}
                    recipientId={friend.user_id}
                    recipientUsername={friend.username}
                    onSent={(message) => { setError(null); setNotice(message); }}
                    onError={(message) => { setNotice(null); setError(message); }}
                    onSessionExpired={onSessionExpired}
                  />
                  <button type="button" className="friends__link" onClick={() => handleBlock(friend.user_id, friend.username)}>Block</button>
                  <button type="button" onClick={() => handleRemove(friend)}>Remove</button>
                </div>
              </li>
            ))}
          </ul>
        )}

        {data !== null && data.outgoing.length > 0 && (
          <>
            <h2>Sent Requests</h2>
            <ul className="server-home__project-list">
              {data.outgoing.map((item) => (
                <li key={item.id} className="server-home__project-card">
                  <div>
                    <h3>{item.recipient_username}</h3>
                    <p className="server-home__project-meta">Pending</p>
                  </div>
                  <button type="button" onClick={() => void run(() => cancelRequest(serverUrl, token, item.id), "Could not cancel this request.")}>Cancel</button>
                </li>
              ))}
            </ul>
          </>
        )}

        <h2>Blocked Users</h2>
        {data !== null && data.blocked.length === 0 && <p className="server-home__loading">You haven&apos;t blocked anyone.</p>}
        {data !== null && data.blocked.length > 0 && (
          <ul className="server-home__project-list">
            {data.blocked.map((entry) => (
              <li key={entry.user_id} className="server-home__project-card">
                <div>
                  <h3>{entry.username}</h3>
                  <p className="server-home__project-meta">Blocked {new Date(entry.since).toLocaleDateString()}</p>
                </div>
                <button type="button" onClick={() => void run(() => unblock(serverUrl, token, entry.user_id), "Could not unblock this user.", `${entry.username} is unblocked.`)}>Unblock</button>
              </li>
            ))}
          </ul>
        )}
        <form className="server-home__actions" onSubmit={handleBlockByName}>
          <input aria-label="Username or email to block" placeholder="Username or email to block" value={blockTarget} onChange={(event) => setBlockTarget(event.target.value)} />
          <button type="submit" disabled={blockTarget.trim() === ""}>Block User</button>
        </form>
      </section>

      {showRequests && (
        <FriendRequests
          serverUrl={serverUrl}
          token={token}
          onClose={() => setShowRequests(false)}
          onChanged={reload}
          onSessionExpired={onSessionExpired}
        />
      )}
    </main>
  );
}

export default Friends;
