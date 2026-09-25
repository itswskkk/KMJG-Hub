import { ProjectDetail } from "../../lib/apiClient";
import { useProjectPresence } from "../presence/PresenceProvider";
import "./Overview.css";

interface OverviewProps {
  detail: ProjectDetail;
  onViewMembers: () => void;
}

/**
 * Project Overview, per docs/UX.md "Project Overview". Member count and
 * online count are real data (the latter derived from the authenticated
 * real-time connection, per docs/ARCHITECTURE.md "Presence"); the full
 * member list with roles lives in the dedicated Members section
 * (docs/UX.md "Members Experience") rather than being duplicated here,
 * matching the UX mockup's own "Members Online 3/4" summary rather than a
 * full roster. Tasks, Git activity, and Project activity are separate
 * features not built in this checkpoint (see PROGRESS.md) — those sections
 * are shown honestly as not-yet-available rather than backed by fabricated
 * data, the same pattern used for the disabled GitHub login option.
 */
function Overview({ detail, onViewMembers }: OverviewProps) {
  const presence = useProjectPresence(detail.id);
  const onlineCount = presence.ready
    ? detail.members.filter((m) => presence.isOnline(m.id) === true).length
    : null;

  return (
    <div className="overview">
      {detail.description && <p className="overview__description">{detail.description}</p>}

      <section className="overview__section">
        <h2>Members</h2>
        {onlineCount !== null ? (
          <p>
            Members Online: {onlineCount} / {detail.members.length}
          </p>
        ) : (
          <>
            <p>
              {detail.members.length} {detail.members.length === 1 ? "Member" : "Members"}
            </p>
            <p className="overview__note">Connecting to real-time presence…</p>
          </>
        )}
        <button type="button" onClick={onViewMembers}>
          View Members
        </button>
      </section>

      <section className="overview__section">
        <h2>Repository</h2>
        <p className="overview__note">No Repository Connected. Connecting a repository is not implemented yet.</p>
      </section>

      <section className="overview__section">
        <h2>Current Tasks</h2>
        {detail.members.filter((member) => member.current_task_title).map((member) => <p key={member.id}>{member.username}: {member.current_task_title}</p>)}
        {detail.members.every((member) => !member.current_task_title) && <p className="overview__note">No member has selected a current task.</p>}
      </section>

      <section className="overview__section">
        <h2>Recent Git Activity</h2>
        <p className="overview__note">Git activity is not implemented yet.</p>
      </section>

      <section className="overview__section">
        <h2>Recent Project Activity</h2>
        <p className="overview__note">Project activity is not implemented yet.</p>
      </section>
    </div>
  );
}

export default Overview;
