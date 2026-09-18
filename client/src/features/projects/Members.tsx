import { useState } from "react";
import { ProjectDetail, ProjectMember } from "../../lib/apiClient";
import { ProjectPresence, useProjectPresence } from "../presence/PresenceProvider";
import "./Members.css";

interface MembersProps {
  detail: ProjectDetail;
}

/**
 * Project Members, per docs/UX.md "Members Experience". Each member entry
 * there may show Display Name, Presence, Work Status, Current Branch, and
 * Current Task. Display Name (we only have Username, not a separate
 * Display Name field yet), Role, and — as of this checkpoint — real
 * Presence (docs/ARCHITECTURE.md "Presence and Work Status Architecture")
 * are real data. Work Status/Current Branch/Current Task still depend on
 * features not built yet (Work Status needs its own activity-detection
 * design; Current Branch needs the Tauri native layer; Current Task needs
 * the Tasks feature) — shown as honest notes, not fabricated statuses.
 *
 * Presence is deliberately shown as a plain dot only once fresh Project
 * presence is known (a presence.snapshot has been received on the current
 * real-time connection) — not merely once the connection is open, since
 * "connected" arrives before that Project's own snapshot does. Until then,
 * no dot is drawn and a neutral note explains why, rather than presenting
 * stale, absent, or invented status as current (docs/ARCHITECTURE.md
 * "Offline State" / docs/UX.md "Offline Experience").
 *
 * "Member Details" (selecting a member) and its four quick actions (Chat,
 * Send File, View Branch, View Current Task) all depend on features this
 * checkpoint doesn't build (Direct Messages, Direct File Transfer, Git,
 * Tasks) and are rendered disabled rather than omitted or faked.
 */
function Members({ detail }: MembersProps) {
  const [selected, setSelected] = useState<ProjectMember | null>(null);
  const presence = useProjectPresence(detail.id);

  return (
    <div className="members">
      <h1>Members</h1>
      {!presence.ready && (
        <p className="members__note">Connecting to real-time presence…</p>
      )}

      <ul className="members__list">
        {detail.members.map((member) => (
          <li key={member.id}>
            <button type="button" className="members__row" onClick={() => setSelected(member)}>
              <span className="members__identity">
                <PresenceDot presence={presence} userId={member.id} />
                <span className="members__name">{member.username}</span>
              </span>
              <span className="members__role">{member.role}</span>
            </button>
          </li>
        ))}
      </ul>

      {selected && (
        <MemberDetail member={selected} presence={presence} onClose={() => setSelected(null)} />
      )}
    </div>
  );
}

interface PresenceDotProps {
  presence: ProjectPresence;
  userId: string;
}

function PresenceDot({ presence, userId }: PresenceDotProps) {
  if (!presence.ready) {
    return null;
  }
  const online = presence.isOnline(userId);
  if (online === undefined) {
    return null;
  }
  return (
    <span
      className={online ? "members__dot members__dot--online" : "members__dot members__dot--offline"}
      aria-label={online ? "Online" : "Offline"}
      title={online ? "Online" : "Offline"}
    >
      {online ? "●" : "○"}
    </span>
  );
}

interface MemberDetailProps {
  member: ProjectMember;
  presence: ProjectPresence;
  onClose: () => void;
}

function MemberDetail({ member, presence, onClose }: MemberDetailProps) {
  const online = presence.isOnline(member.id);

  return (
    <div className="member-detail__overlay" onClick={onClose}>
      <div className="member-detail" onClick={(event) => event.stopPropagation()}>
        <button type="button" className="member-detail__close" onClick={onClose} aria-label="Close">
          ×
        </button>

        <h2>{member.username}</h2>
        <p className="members__role">{member.role}</p>
        <p className="members__presence">
          {online === undefined ? "Presence unknown" : online ? "● Online" : "○ Offline"}
        </p>

        <p className="members__note">
          Work status, current branch, and current task are not implemented yet.
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
