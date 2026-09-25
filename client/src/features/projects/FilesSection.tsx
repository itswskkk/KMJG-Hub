import { useEffect, useState } from "react";
import {
  ApiError,
  ProjectFile,
  downloadProjectAttachment,
  isSessionExpired,
  listProjectAttachments,
} from "../../lib/apiClient";
import { useProjectChatEvents } from "../presence/PresenceProvider";
import "./FilesSection.css";

interface FilesSectionProps {
  serverUrl: string;
  token: string;
  projectId: string;
  onSessionExpired: () => void;
  onOpenChat: () => void;
}

/**
 * Files section, per docs/UX.md "Files Experience": every file shared
 * through Project Chat, newest first. Files are downloaded only when the
 * member explicitly chooses to. Direct File Transfers are a separate
 * experience and never appear here.
 */
function FilesSection({ serverUrl, token, projectId, onSessionExpired, onOpenChat }: FilesSectionProps) {
  const [files, setFiles] = useState<ProjectFile[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [downloadingId, setDownloadingId] = useState<string | null>(null);
  // A chat message being created or deleted may add or remove files; the
  // event is only a hint, so reload the list from the Server.
  // (The event buffer is capped, so key on the latest event, not the count.)
  const chatEvents = useProjectChatEvents(projectId);
  const latestChatEvent = chatEvents[chatEvents.length - 1];

  function handleError(err: unknown, fallback: string) {
    if (isSessionExpired(err)) {
      onSessionExpired();
      return;
    }
    setError(err instanceof ApiError ? err.message : fallback);
  }

  useEffect(() => {
    let cancelled = false;
    listProjectAttachments(serverUrl, token, projectId)
      .then((loaded) => {
        if (!cancelled) {
          setFiles(loaded);
          setError(null);
        }
      })
      .catch((err) => {
        if (!cancelled) handleError(err, "Could not load Project files.");
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverUrl, token, projectId, latestChatEvent]);

  async function handleDownload(file: ProjectFile) {
    setDownloadingId(file.id);
    setError(null);
    try {
      await downloadProjectAttachment(serverUrl, token, projectId, file);
    } catch (err) {
      handleError(err, "Could not download the file.");
    } finally {
      setDownloadingId(null);
    }
  }

  return (
    <section className="files-section">
      <h1>Files</h1>
      <p className="files-section__intro">Files shared with this Project through Project Chat.</p>

      {error && (
        <p className="files-section__error" role="alert">
          {error}
        </p>
      )}
      {files === null && !error && <p className="files-section__note">Loading files...</p>}
      {files?.length === 0 && <p className="files-section__note">No files have been shared in this project yet.</p>}

      {files && files.length > 0 && (
        <ul className="files-section__list">
          {files.map((file) => (
            <li key={file.id} className="files-section__item">
              <div className="files-section__info">
                <strong title={file.filename}>{file.filename}</strong>
                <span>
                  {formatBytes(file.size_bytes)} · {file.author_username || "Unknown"} ·{" "}
                  <time dateTime={file.created_at}>{new Date(file.created_at).toLocaleDateString()}</time>
                </span>
              </div>
              <div className="files-section__actions">
                <button type="button" disabled={downloadingId === file.id} onClick={() => void handleDownload(file)}>
                  {downloadingId === file.id ? "Downloading..." : "Download"}
                </button>
                <button type="button" onClick={onOpenChat}>
                  View Chat
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

export default FilesSection;
