import { FormEvent, useState } from "react";
import { ApiError, ProjectDetail, deleteProject, isSessionExpired, leaveProject } from "../../lib/apiClient";

interface ProjectDangerZoneProps {
  detail: ProjectDetail;
  serverUrl: string;
  token: string;
  /** Called after the viewer left or deleted the Project. */
  onProjectClosed: () => void;
  onSessionExpired: () => void;
}

/**
 * Leave Project and Delete Project, per docs/PRD.md "Leaving a Project" and
 * "Project Deletion". Members and Admins may leave; the Owner must transfer
 * ownership first (from Members), and only the Owner may delete. Deletion
 * requires typing the Project name and is restorable for 30 days from
 * "Recently Deleted Projects" on Server Home.
 */
function ProjectDangerZone({ detail, serverUrl, token, onProjectClosed, onSessionExpired }: ProjectDangerZoneProps) {
  const [confirming, setConfirming] = useState<"leave" | "delete" | null>(null);
  const [typedName, setTypedName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const isOwner = detail.role === "owner";

  async function run(action: () => Promise<void>, fallback: string) {
    setBusy(true);
    setError(null);
    try {
      await action();
    } catch (err) {
      if (isSessionExpired(err)) {
        onSessionExpired();
        return;
      }
      setError(err instanceof ApiError ? err.message : fallback);
      setBusy(false);
      return;
    }
    onProjectClosed();
  }

  function submitDelete(event: FormEvent) {
    event.preventDefault();
    if (typedName !== detail.name) return;
    void run(() => deleteProject(serverUrl, token, detail.id), "Could not delete this project.");
  }

  function cancel() {
    setConfirming(null);
    setTypedName("");
    setError(null);
  }

  return (
    <section className="overview__section overview__danger-zone">
      <h2>Danger Zone</h2>

      {isOwner ? (
        <p className="overview__note">
          As the Owner you can&apos;t leave this Project. Transfer ownership to another member from Members first.
        </p>
      ) : (
        <button type="button" onClick={() => setConfirming("leave")} disabled={busy}>
          Leave Project
        </button>
      )}

      {isOwner && (
        <button type="button" className="overview__danger-button" onClick={() => setConfirming("delete")} disabled={busy}>
          Delete Project
        </button>
      )}

      {confirming === "leave" && (
        <div className="overview__confirm" role="alertdialog" aria-labelledby="leave-project-title">
          <h3 id="leave-project-title">Leave {detail.name}?</h3>
          <p>You will lose access to this Project until someone invites you again.</p>
          {error && <p className="overview__error" role="alert">{error}</p>}
          <button type="button" onClick={cancel} disabled={busy}>Cancel</button>{" "}
          <button
            type="button"
            className="overview__danger-button"
            onClick={() => run(() => leaveProject(serverUrl, token, detail.id), "Could not leave this project.")}
            disabled={busy}
          >
            {busy ? "Leaving…" : "Leave Project"}
          </button>
        </div>
      )}

      {confirming === "delete" && (
        <form className="overview__confirm" role="alertdialog" aria-labelledby="delete-project-title" onSubmit={submitDelete}>
          <h3 id="delete-project-title">Delete {detail.name}?</h3>
          <p>
            The Project becomes unavailable to all members. You can restore it from Recently Deleted Projects for 30
            days; after that it is permanently deleted. Any connected Git repository is not deleted.
          </p>
          <label>
            Type <strong>{detail.name}</strong> to confirm
            <input
              aria-label="Project name confirmation"
              value={typedName}
              onChange={(event) => setTypedName(event.target.value)}
              autoComplete="off"
            />
          </label>
          {error && <p className="overview__error" role="alert">{error}</p>}
          <button type="button" onClick={cancel} disabled={busy}>Cancel</button>{" "}
          <button type="submit" className="overview__danger-button" disabled={busy || typedName !== detail.name}>
            {busy ? "Deleting…" : "Delete Project"}
          </button>
        </form>
      )}
    </section>
  );
}

export default ProjectDangerZone;
