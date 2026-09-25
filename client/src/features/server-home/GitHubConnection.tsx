import { useEffect, useRef, useState } from "react";
import { ApiError, GitHubStatus, disconnectGitHub, getGitHubStatus, isSessionExpired, startGitHubConnect } from "../../lib/apiClient";
import { openExternalUrl } from "../../lib/nativeClient";

interface GitHubConnectionProps {
  serverUrl: string;
  token: string;
  onSessionExpired: () => void;
}

// How often to re-check the connection while the user completes GitHub
// authorization in their browser, and for how long.
const POLL_INTERVAL_MS = 3000;
const POLL_LIMIT_MS = 10 * 60 * 1000;

/**
 * The viewer's GitHub account link, per docs/PRD.md "GitHub
 * Authentication": GitHub is linked to (never replaces) the KMJG Hub
 * account. Authorization happens in the system browser; GitHub redirects to
 * the Server, which links the account, so this view polls for completion.
 */
function GitHubConnection({ serverUrl, token, onSessionExpired }: GitHubConnectionProps) {
  const [status, setStatus] = useState<GitHubStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [waiting, setWaiting] = useState(false);
  const pollRef = useRef<number | null>(null);

  function handleError(err: unknown, fallback: string) {
    if (isSessionExpired(err)) {
      onSessionExpired();
      return;
    }
    setError(err instanceof ApiError ? err.message : fallback);
  }

  function stopPolling() {
    if (pollRef.current !== null) {
      window.clearInterval(pollRef.current);
      pollRef.current = null;
    }
    setWaiting(false);
  }

  useEffect(() => {
    let cancelled = false;
    getGitHubStatus(serverUrl, token)
      .then((loaded) => {
        if (!cancelled) setStatus(loaded);
      })
      .catch((err) => {
        if (!cancelled) handleError(err, "Could not load your GitHub connection.");
      });
    return () => {
      cancelled = true;
      if (pollRef.current !== null) window.clearInterval(pollRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverUrl, token]);

  async function handleConnect() {
    setError(null);
    setBusy(true);
    try {
      const url = await startGitHubConnect(serverUrl, token);
      await openExternalUrl(url);
      setWaiting(true);
      const startedAt = Date.now();
      pollRef.current = window.setInterval(() => {
        if (Date.now() - startedAt > POLL_LIMIT_MS) {
          stopPolling();
          return;
        }
        getGitHubStatus(serverUrl, token)
          .then((latest) => {
            if (latest.connected) {
              setStatus(latest);
              stopPolling();
            }
          })
          .catch(() => {});
      }, POLL_INTERVAL_MS);
    } catch (err) {
      handleError(err, "Could not start connecting GitHub.");
    } finally {
      setBusy(false);
    }
  }

  async function handleDisconnect() {
    setError(null);
    setBusy(true);
    try {
      await disconnectGitHub(serverUrl, token);
      setStatus((current) => (current ? { configured: current.configured, connected: false } : current));
    } catch (err) {
      handleError(err, "Could not disconnect GitHub.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="profile__section">
      <h2>GitHub</h2>
      {status === null && !error && <p className="profile__meta">Loading...</p>}
      {status && !status.configured && !status.connected && (
        <p className="profile__meta">GitHub integration is not configured on this server.</p>
      )}
      {status?.connected && (
        <>
          <p className="profile__meta">
            Connected as <strong>{status.login}</strong>
            {status.connected_at ? ` since ${new Date(status.connected_at).toLocaleDateString()}` : ""}.
          </p>
          <div className="profile__buttons">
            <button type="button" onClick={() => void handleDisconnect()} disabled={busy}>
              {busy ? "Disconnecting..." : "Disconnect GitHub"}
            </button>
          </div>
        </>
      )}
      {status && status.configured && !status.connected && (
        <>
          <p className="profile__meta">
            Link a GitHub account to your KMJG Hub account to connect repositories to your Projects.
          </p>
          {waiting && <p className="profile__meta">Finish authorizing in your browser. This page updates automatically.</p>}
          <div className="profile__buttons">
            <button type="button" onClick={() => void handleConnect()} disabled={busy || waiting}>
              {busy ? "Opening GitHub..." : "Connect GitHub"}
            </button>
            {waiting && (
              <button type="button" onClick={stopPolling}>
                Cancel
              </button>
            )}
          </div>
        </>
      )}
      {error && (
        <p className="profile__error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}

export default GitHubConnection;
