import { useState } from "react";
import { ProjectDetail, ProjectMember } from "../../lib/apiClient";
import "./Members.css";

interface MembersProps {
  detail: ProjectDetail;
}

/**
 * Project Members, per docs/UX.md "Members Experience". Each member entry
 * there may show Display Name, Presence, Work Status, Current Branch, and
 * Current Task — of those, only a name (we only have Username, not a
 * separate Display Name field yet) and Role are real data this checkpoint.
 * Presence/Work Status/Current Branch/Current Task all depend on features
 * not built yet (Presence architecture needs the WebSocket layer this
 * codebase doesn't have; Current Branch needs the Tauri native layer;
 * Current Task needs the Tasks feature) — shown as honest notes, not
 * fabricated statuses, per the same pattern used throughout Overview.
 *
 * "Member Details" (selecting a member) and its four quick actions (Chat,
 * Send File, View Branch, View Current Task) all depend on features this
 * checkpoint doesn't build (Direct Messages, Direct File Transfer, Git,
 * Tasks) and are rendered disabled rather than omitted or faked.
 */
function Members({ detail }: MembersProps) {
  const [selected, setSelected] = useState<ProjectMember | null>(null);

  return (
    <div className="members">
      <h1>Members</h1>
      <p className="members__note">Online presence and work status are not implemented yet.</p>

      <ul className="members__list">
        {detail.members.map((member) => (
          <li key={member.id}>
            <button type="button" className="members__row" onClick={() => setSelected(member)}>
              <span className="members__name">{member.username}</span>
              <span className="members__role">{member.role}</span>
            </button>
          </li>
        ))}
      </ul>

      {selected && <MemberDetail member={selected} onClose={() => setSelected(null)} />}
    </div>
  );
}

interface MemberDetailProps {
  member: ProjectMember;
  onClose: () => void;
}

function MemberDetail({ member, onClose }: MemberDetailProps) {
  return (
    <div className="member-detail__overlay" onClick={onClose}>
      <div className="member-detail" onClick={(event) => event.stopPropagation()}>
        <button type="button" className="member-detail__close" onClick={onClose} aria-label="Close">
          ×
        </button>

        <h2>{member.username}</h2>
        <p className="members__role">{member.role}</p>

        <p className="members__note">
          Presence, work status, current branch, and current task are not implemented yet.
        </p>

        <div className="member-detail__actions">
          <button type="button" disabled title="Direct Messages are not implemented yet">
            Chat
          </button>
          <button type="button" disabled title="Direct File Transfer is not implemented yet">
            Send File
          </button>
          <button type="button" disabled title="Git integration is not implemented yet">
            View Branch
          </button>
          <button type="button" disabled title="Tasks are not implemented yet">
            View Current Task
          </button>
        </div>
      </div>
    </div>
  );
}

export default Members;
