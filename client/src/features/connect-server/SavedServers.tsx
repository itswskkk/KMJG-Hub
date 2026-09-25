import { useEffect, useState } from "react";
import { AuthResponse, validateSession } from "../../lib/apiClient";
import { deleteSession, loadSessions, SavedSession } from "../../lib/nativeClient";
import "./SavedServers.css";

interface SavedServersProps {
  /** Session is still valid: go straight to that Server's Home. */
  onConnected: (serverUrl: string, auth: AuthResponse) => void;
  /** Session is missing/expired: go to that Server's Login screen. */
  onNeedsLogin: (serverUrl: string) => void;
  /** "+ Add Server": go to the Connect Server screen. */
  onAddServer: () => void;
}

/**
 * "Your Servers" (docs/UX.md "Saved Servers"): lists every Server the user
 * has previously connected to, letting them continue into one, remove one,
 * or add another — without ever auto-selecting a Server for them (docs/UX.md
 * "KMJG Hub does not automatically open the user's most recently used
 * Project" — the same principle applies to Servers).
 */
function SavedServers({ onConnected, onNeedsLogin, onAddServer }: SavedServersProps) {
  const [sessions, setSessions] = useState<SavedSession[] | null>(null);
  const [connectingUrl, setConnectingUrl] = useState<string | null>(null);
  const [removingUrl, setRemovingUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    loadSessions()
      .then((list) => { if (!cancelled) setSessions(list); })
      .catch(() => { if (!cancelled) setSessions([]); });
    return () => { cancelled = true; };
  }, []);

  async function handleConnect(session: SavedSession) {
    setError(null);
    setConnectingUrl(session.serverUrl);
    try {
      await validateSession(session.serverUrl, session.auth.session.token);
      onConnected(session.serverUrl, session.auth);
    } catch {
      // The saved session is no longer valid (docs/PRD.md "If no valid
      // session exists, the user must authenticate with that Server."):
      // forget it locally and send the user to that Server's Login screen.
      await deleteSession(session.serverUrl).catch(() => {});
      setSessions((current) => current?.filter((entry) => entry.serverUrl !== session.serverUrl) ?? current);
      onNeedsLogin(session.serverUrl);
    } finally {
      setConnectingUrl(null);
    }
  }

  async function handleRemove(serverUrl: string) {
    setError(null);
    setRemovingUrl(serverUrl);
    try {
      // Local-only action (docs/PRD.md "Removing a saved Server ... removes
      // only the local saved Server connection"): no Server API call.
      await deleteSession(serverUrl);
      setSessions((current) => current?.filter((entry) => entry.serverUrl !== serverUrl) ?? current);
    } catch {
      setError("Could not remove this Server.");
    } finally {
      setRemovingUrl(null);
    }
  }

  return (
    <main className="saved-servers">
      <div className="saved-servers__card">
        <h1>Your Servers</h1>

        {error && <p className="saved-servers__error" role="alert">{error}</p>}

        {sessions === null && <p className="saved-servers__loading">Loading your Servers…</p>}

        {sessions !== null && sessions.length === 0 && (
          <p className="saved-servers__empty">You haven&apos;t saved any Servers yet.</p>
        )}

        {sessions !== null && sessions.length > 0 && (
          <ul className="saved-servers__list">
            {sessions.map((session) => {
              const label = serverLabel(session.serverUrl);
              const busy = connectingUrl === session.serverUrl || removingUrl === session.serverUrl;
              return (
                <li key={session.serverUrl} className="saved-servers__item">
                  <div className="saved-servers__item-info">
                    <h2>{label}</h2>
                    <p className="saved-servers__item-url">{session.serverUrl}</p>
                  </div>
                  <div className="saved-servers__item-actions">
                    <button
                      type="button"
                      onClick={() => handleRemove(session.serverUrl)}
                      disabled={busy}
                      className="saved-servers__remove"
                      aria-label={`Remove ${label}`}
                    >
                      Remove
                    </button>
                    <button type="button" onClick={() => handleConnect(session)} disabled={busy}>
                      {connectingUrl === session.serverUrl ? "Connecting…" : "Connect"}
                    </button>
                  </div>
                </li>
              );
            })}
          </ul>
        )}

        <button type="button" className="saved-servers__add" onClick={onAddServer}>
          + Add Server
        </button>
      </div>
    </main>
  );
}

/** Friendly-ish label for a saved Server: no Server display name is stored
 * anywhere yet (see docs/PRD.md "Multi-Server Client Support"), so the
 * hostname of the saved address stands in for one. */
function serverLabel(serverUrl: string): string {
  try {
    return new URL(serverUrl).hostname;
  } catch {
    return serverUrl;
  }
}

export default SavedServers;
