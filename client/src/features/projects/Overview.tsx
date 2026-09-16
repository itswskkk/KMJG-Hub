import { ProjectDetail } from "../../lib/apiClient";
import "./Overview.css";

interface OverviewProps {
  detail: ProjectDetail;
}

/**
 * Project Overview, per docs/UX.md "Project Overview". Members are real
 * data (docs/UX.md "Members Experience" fields we do have: display name,
 * role). Online presence, Tasks, Git activity, and Project activity are
 * separate features not built in this checkpoint (see PROGRESS.md) — those
 * sections are shown honestly as not-yet-available rather than backed by
 * fabricated data, the same pattern used for the disabled GitHub login
 * option.
 */
function Overview({ detail }: OverviewProps) {
  return (
    <div className="overview">
      {detail.description && <p className="overview__description">{detail.description}</p>}

      <section className="overview__section">
        <h2>Members</h2>
        <ul className="overview__members">
          {detail.members.map((member) => (
            <li key={member.id}>
              <span className="overview__member-name">{member.username}</span>
              <span className="overview__member-role">{member.role}</span>
            </li>
          ))}
        </ul>
        <p className="overview__note">Online presence is not implemented yet.</p>
      </section>

      <section className="overview__section">
        <h2>Repository</h2>
        <p className="overview__note">No Repository Connected. Connecting a repository is not implemented yet.</p>
      </section>

      <section className="overview__section">
        <h2>Current Tasks</h2>
        <p className="overview__note">Tasks are not implemented yet.</p>
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
