import { ChangeEvent, useEffect, useRef, useState } from "react";
import {
  ApiError,
  FileTransfer,
  acceptTransfer,
  cancelTransfer,
  declineTransfer,
  downloadTransferFile,
  isSessionExpired,
  listIncomingTransfers,
  listSentTransfers,
  uploadTransferFile,
} from "../../lib/apiClient";
import { forgetTransferFile, getTransferFile, rememberTransferFile } from "../../lib/pendingTransferFiles";
import { useFileTransferEvents } from "../presence/PresenceProvider";
import "./ServerHome.css";
import "./FileTransfers.css";

interface FileTransfersProps {
  serverUrl: string;
  token: string;
  onBack: () => void;
  onSessionExpired: () => void;
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

const STATUS_LABELS: Record<string, string> = {
  declined: "Declined",
  cancelled: "Cancelled",
  expired: "Expired",
};

/** Server Home's Direct File Transfers screen.
 *
 * Received: pending → Accept / Decline; accepted → waiting for the sender to
 * upload; uploaded → Download.
 * Sent: pending → waiting for the recipient (Cancel); accepted → Upload with
 * progress (Cancel aborts); uploaded → delivered. */
function FileTransfers({ serverUrl, token, onBack, onSessionExpired }: FileTransfersProps) {
  const [incoming, setIncoming] = useState<FileTransfer[] | null>(null);
  const [sent, setSent] = useState<FileTransfer[] | null>(null);
  const [maxUploadBytes, setMaxUploadBytes] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [progress, setProgress] = useState<Record<string, number>>({});
  const [reloadKey, setReloadKey] = useState(0);
  const aborters = useRef(new Map<string, AbortController>());
  const events = useFileTransferEvents();
  const latestEvent = events[events.length - 1];

  useEffect(() => {
    let cancelled = false;
    Promise.all([listIncomingTransfers(serverUrl, token), listSentTransfers(serverUrl, token)])
      .then(([inList, sentList]) => {
        if (cancelled) return;
        setIncoming(inList.transfers);
        setSent(sentList.transfers);
        setMaxUploadBytes(inList.max_upload_bytes);
      })
      .catch((err) => {
        if (cancelled) return;
        if (isSessionExpired(err)) { onSessionExpired(); return; }
        setError(err instanceof ApiError ? err.message : "Could not load file transfers.");
      });
    return () => { cancelled = true; };
    // Real-time file_transfer.* events are reload hints.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverUrl, token, reloadKey, latestEvent]);

  // Abort any in-flight upload when leaving the screen.
  useEffect(() => {
    const current = aborters.current;
    return () => { current.forEach((controller) => controller.abort()); };
  }, []);

  function fail(err: unknown, fallback: string) {
    if (isSessionExpired(err)) { onSessionExpired(); return; }
    setError(err instanceof ApiError ? err.message : fallback);
  }

  async function act(transfer: FileTransfer, action: () => Promise<unknown>, failure: string, success?: string) {
    setError(null); setNotice(null); setBusyId(transfer.id);
    try {
      await action();
      if (success) setNotice(success);
      setReloadKey((k) => k + 1);
    } catch (err) {
      fail(err, failure);
    } finally {
      setBusyId(null);
    }
  }

  async function upload(transfer: FileTransfer, file: File) {
    if (file.size !== transfer.file_size) {
      setError(`“${file.name}” is ${formatBytes(file.size)}, but the request was for ${formatBytes(transfer.file_size)}. Choose the original file.`);
      return;
    }
    setError(null); setNotice(null);
    rememberTransferFile(transfer.id, file);
    const controller = new AbortController();
    aborters.current.set(transfer.id, controller);
    setProgress((p) => ({ ...p, [transfer.id]: 0 }));
    try {
      await uploadTransferFile(serverUrl, token, transfer.id, file, (fraction) => setProgress((p) => ({ ...p, [transfer.id]: fraction })), controller.signal);
      forgetTransferFile(transfer.id);
      setNotice(`“${transfer.file_name}” was delivered to ${transfer.recipient_username}.`);
      setReloadKey((k) => k + 1);
    } catch (err) {
      if (err instanceof ApiError && err.code === "aborted") return;
      if (err instanceof ApiError && err.code === "network_error") {
        setError(`Uploading “${transfer.file_name}” was interrupted. You can try again.`);
        return;
      }
      fail(err, "Could not upload the file.");
    } finally {
      aborters.current.delete(transfer.id);
      setProgress((p) => { const next = { ...p }; delete next[transfer.id]; return next; });
    }
  }

  function chooseAndUpload(transfer: FileTransfer, event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (file) void upload(transfer, file);
  }

  function cancel(transfer: FileTransfer) {
    aborters.current.get(transfer.id)?.abort();
    void act(transfer, async () => {
      await cancelTransfer(serverUrl, token, transfer.id);
      forgetTransferFile(transfer.id);
    }, "Could not cancel this transfer.", `Transfer of “${transfer.file_name}” cancelled.`);
  }

  function renderIncoming(transfer: FileTransfer) {
    const busy = busyId === transfer.id;
    switch (transfer.status) {
      case "pending":
        return (
          <div className="server-home__actions">
            <button type="button" disabled={busy} onClick={() => void act(transfer, () => declineTransfer(serverUrl, token, transfer.id), "Could not decline this transfer.")}>Decline</button>
            <button type="button" disabled={busy} onClick={() => void act(transfer, () => acceptTransfer(serverUrl, token, transfer.id), "Could not accept this transfer.", `Accepted. Waiting for ${transfer.sender_username} to upload.`)}>Accept</button>
          </div>
        );
      case "accepted":
        return <span className="file-transfers__status">Waiting for upload…</span>;
      case "uploaded":
        return (
          <button type="button" disabled={busy} onClick={() => void act(transfer, () => downloadTransferFile(serverUrl, token, transfer), "Could not download the file.")}>Download</button>
        );
      default:
        return <span className="file-transfers__status">{STATUS_LABELS[transfer.status] ?? transfer.status}</span>;
    }
  }

  function renderSent(transfer: FileTransfer) {
    const busy = busyId === transfer.id;
    const fraction = progress[transfer.id];
    switch (transfer.status) {
      case "pending":
        return (
          <div className="server-home__actions">
            <span className="file-transfers__status">Waiting for {transfer.recipient_username}</span>
            <button type="button" disabled={busy} onClick={() => cancel(transfer)}>Cancel</button>
          </div>
        );
      case "accepted": {
        if (fraction !== undefined) {
          return (
            <div className="server-home__actions">
              <progress className="file-transfers__progress" max={1} value={fraction} aria-label={`Uploading ${transfer.file_name}`} />
              <span className="file-transfers__status">{Math.round(fraction * 100)}%</span>
              <button type="button" disabled={busy} onClick={() => cancel(transfer)}>Cancel</button>
            </div>
          );
        }
        const remembered = getTransferFile(transfer.id);
        return (
          <div className="server-home__actions">
            <span className="file-transfers__status">Accepted</span>
            {remembered ? (
              <button type="button" disabled={busy} onClick={() => void upload(transfer, remembered)}>Upload</button>
            ) : (
              <label className="file-transfers__choose">
                Choose file to upload
                <input type="file" hidden onChange={(event) => chooseAndUpload(transfer, event)} />
              </label>
            )}
            <button type="button" disabled={busy} onClick={() => cancel(transfer)}>Cancel</button>
          </div>
        );
      }
      case "uploaded":
        return <span className="file-transfers__status">Delivered</span>;
      default:
        return <span className="file-transfers__status">{STATUS_LABELS[transfer.status] ?? transfer.status}</span>;
    }
  }

  return (
    <main className="server-home">
      <div className="server-home__header">
        <div>
          <h1>File Transfers</h1>
          <p className="server-home__subtitle">
            {serverUrl}
            {maxUploadBytes > 0 && ` · Maximum file size ${formatBytes(maxUploadBytes)}`}
          </p>
        </div>
        <div className="server-home__header-actions">
          <button type="button" onClick={onBack}>Back</button>
        </div>
      </div>

      {error && <p className="server-home__error" role="alert">{error}</p>}
      {notice && <p className="file-transfers__notice" role="status">{notice}</p>}

      <section className="server-home__projects">
        <p className="server-home__loading">To send a file, open Friends and choose Send File next to a friend. Files are uploaded only after the recipient accepts.</p>

        <h2>Received</h2>
        {incoming === null && !error && <p className="server-home__loading">Loading…</p>}
        {incoming !== null && incoming.length === 0 && <p className="server-home__loading">No incoming file transfers.</p>}
        {incoming !== null && incoming.length > 0 && (
          <ul className="server-home__project-list">
            {incoming.map((transfer) => (
              <li key={transfer.id} className="server-home__project-card">
                <div>
                  <h3>{transfer.file_name}</h3>
                  <p className="server-home__project-meta">From {transfer.sender_username} · {formatBytes(transfer.file_size)} · {new Date(transfer.created_at).toLocaleString()}</p>
                </div>
                {renderIncoming(transfer)}
              </li>
            ))}
          </ul>
        )}

        <h2>Sent</h2>
        {sent !== null && sent.length === 0 && <p className="server-home__loading">No sent file transfers.</p>}
        {sent !== null && sent.length > 0 && (
          <ul className="server-home__project-list">
            {sent.map((transfer) => (
              <li key={transfer.id} className="server-home__project-card">
                <div>
                  <h3>{transfer.file_name}</h3>
                  <p className="server-home__project-meta">To {transfer.recipient_username} · {formatBytes(transfer.file_size)} · {new Date(transfer.created_at).toLocaleString()}</p>
                </div>
                {renderSent(transfer)}
              </li>
            ))}
          </ul>
        )}
      </section>
    </main>
  );
}

export default FileTransfers;
