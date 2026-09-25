import { FormEvent, useEffect, useState } from "react";
import { ApiError, DirectInvitation, InviteCredential, ProjectDetail, ProjectMember, cancelInvitation, createDirectInvitation, createInviteCredential, listInviteCredentials, listProjectInvitations, removeProjectMember, revokeInviteCredential } from "../../lib/apiClient";
import { ProjectPresence, useProjectPresence } from "../presence/PresenceProvider";
import { useProjectWorkContexts } from "../work-context/useProjectWorkContexts";
import "./Members.css";

interface MembersProps {
  detail: ProjectDetail;
  serverUrl: string;
  token: string;
  viewerUserId: string;
  onMemberRemoved: (userId: string) => void;
}

/**
 * Project Members, per docs/UX.md "Members Experience". Each member entry
 * there may show Display Name, Presence, Work Status, Current Branch, and
 * Current Task. Display Name (we only have Username, not a separate
 * Display Name field yet), Role, and — as of this checkpoint — real
 * Presence (docs/ARCHITECTURE.md "Presence and Work Status Architecture")
 * and Project-scoped Current Task are real data. Work Status and Current
 * Branch still depend on features not built yet (Work Status needs its own
 * activity-detection design; Current Branch needs the Tauri native layer)
 * — shown as honest notes, not fabricated statuses.
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
 * Send File, View Branch) depend on features this checkpoint doesn't build
 * (Direct Messages, Direct File Transfer, Git) and are rendered disabled
 * rather than omitted or faked. Current Task is shown as member context.
 */
function Members({ detail, serverUrl, token, viewerUserId, onMemberRemoved }: MembersProps) {
  const [selected, setSelected] = useState<ProjectMember | null>(null);
  const presence = useProjectPresence(detail.id);
	const {contexts:workContexts}=useProjectWorkContexts(serverUrl,token,detail.id);
  const [recipient, setRecipient] = useState("");
  const [expiresIn, setExpiresIn] = useState("7d");
  const [pending, setPending] = useState<DirectInvitation[]>([]);
  const [invitationError, setInvitationError] = useState<string | null>(null);
  const [credentials, setCredentials] = useState<InviteCredential[]>([]);
  const [credentialExpiration, setCredentialExpiration] = useState("7d");
  const [maxUses, setMaxUses] = useState("1");
  const [unlimitedUses, setUnlimitedUses] = useState(false);
  const [newCode, setNewCode] = useState<string | null>(null);
  const canInvite = detail.role === "owner" || detail.role === "admin";

  useEffect(() => {
    if (!canInvite) return;
    Promise.all([listProjectInvitations(serverUrl, token, detail.id), listInviteCredentials(serverUrl, token, detail.id)])
      .then(([direct, shareable]) => { setPending(direct); setCredentials(shareable); })
      .catch((err) => setInvitationError(err instanceof ApiError ? err.message : "Could not load invitations."));
  }, [canInvite, detail.id, serverUrl, token]);

  async function invite(event: FormEvent) {
    event.preventDefault(); setInvitationError(null);
    try {
      const item = await createDirectInvitation(serverUrl, token, detail.id, recipient.trim(), expiresIn);
      setPending((current) => [item, ...current]); setRecipient("");
    } catch (err) { setInvitationError(err instanceof ApiError ? err.message : "Could not send invitation."); }
  }

  async function cancel(item: DirectInvitation) {
    try { await cancelInvitation(serverUrl, token, detail.id, item.id); setPending((current) => current.filter((entry) => entry.id !== item.id)); }
    catch (err) { setInvitationError(err instanceof ApiError ? err.message : "Could not cancel invitation."); }
  }

  async function createShareableInvite(event: FormEvent) {
    event.preventDefault(); setInvitationError(null); setNewCode(null);
    try {
      const item = await createInviteCredential(serverUrl, token, detail.id, credentialExpiration, unlimitedUses ? null : Number(maxUses));
      setCredentials((current) => [item, ...current]); setNewCode(item.code ?? null);
    } catch (err) { setInvitationError(err instanceof ApiError ? err.message : "Could not create invite code."); }
  }

  async function revoke(item: InviteCredential) {
    try { await revokeInviteCredential(serverUrl, token, detail.id, item.id); setCredentials((current) => current.filter((entry) => entry.id !== item.id)); }
    catch (err) { setInvitationError(err instanceof ApiError ? err.message : "Could not revoke invite."); }
  }

  return (
    <div className="members">
      <h1>Members</h1>
      {canInvite && (
        <section className="members__invitations">
          <h2>Invite a Member</h2>
          <form onSubmit={invite} className="members__invite-form">
            <input aria-label="Username or email" placeholder="Username or email" value={recipient} onChange={(event) => setRecipient(event.target.value)} required />
            <select aria-label="Invitation expiration" value={expiresIn} onChange={(event) => setExpiresIn(event.target.value)}>
              <option value="1h">1 hour</option><option value="1d">1 day</option><option value="7d">7 days</option><option value="30d">30 days</option><option value="never">Never</option>
            </select>
            <button type="submit">Send Invitation</button>
          </form>
          {invitationError && <p className="member-detail__error" role="alert">{invitationError}</p>}
          {pending.map((item) => <div className="members__pending" key={item.id}><span>{item.recipient_username}</span><button type="button" onClick={() => cancel(item)}>Cancel</button></div>)}
          <h2>Invite Link or Code</h2>
          <form onSubmit={createShareableInvite} className="members__invite-form">
            <select aria-label="Link expiration" value={credentialExpiration} onChange={(event) => setCredentialExpiration(event.target.value)}><option value="1h">1 hour</option><option value="1d">1 day</option><option value="7d">7 days</option><option value="30d">30 days</option><option value="never">Never</option></select>
            <input aria-label="Maximum uses" type="number" min="1" value={maxUses} onChange={(event) => setMaxUses(event.target.value)} disabled={unlimitedUses} required={!unlimitedUses} />
            <label><input type="checkbox" checked={unlimitedUses} onChange={(event) => setUnlimitedUses(event.target.checked)} /> Unlimited uses</label>
            <button type="submit">Create Invite</button>
          </form>
          {newCode && <div className="members__invite-created"><strong>Code:</strong> <code>{newCode}</code><br /><strong>Link:</strong> <code>{new URL(`/invite/${newCode}`, serverUrl).toString()}</code><p className="members__note">Copy it now; the Server stores only its hash.</p></div>}
          {credentials.map((item) => <div className="members__pending" key={item.id}><span>{item.uses} / {item.max_uses ?? "∞"} uses</span><button type="button" onClick={() => revoke(item)}>Revoke</button></div>)}
        </section>
      )}
      {!presence.ready && (
        <p className="members__note">Connecting to real-time presence…</p>
      )}

      <ul className="members__list">
        {detail.members.map((member) => (
          <li key={member.id}>
            <button type="button" className="members__row" onClick={() => setSelected(member)}>
              <span className="members__identity">
                <PresenceDot presence={presence} userId={member.id} />
				<span><span className="members__name">{member.username}</span>{workContexts[member.id]&&<small className="members__current-task">{workContexts[member.id].working?"Working":"Not working"}{workContexts[member.id].current_branch?` · ${workContexts[member.id].current_branch}`:""}</small>}{member.current_task_title && <small className="members__current-task">Current task: {member.current_task_title}</small>}</span>
              </span>
              <span className="members__role">{member.role}</span>
            </button>
          </li>
        ))}
      </ul>

      {selected && (
        <MemberDetail
          member={selected}
          presence={presence}
		  workContext={workContexts[selected.id]}
          canRemove={canRemoveMember(detail.role, viewerUserId, selected)}
          serverUrl={serverUrl}
          token={token}
          projectId={detail.id}
          onRemoved={onMemberRemoved}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  );
}

function canRemoveMember(viewerRole: string, viewerUserId: string, member: ProjectMember): boolean {
  if (member.id === viewerUserId || member.role === "owner") {
    return false;
  }
  return viewerRole === "owner" || (viewerRole === "admin" && member.role === "member");
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
  canRemove: boolean;
  serverUrl: string;
  token: string;
  projectId: string;
  onRemoved: (userId: string) => void;
  onClose: () => void;
	workContext?: import("../../lib/apiClient").ProjectWorkContext;
}

function MemberDetail({ member, presence, workContext, canRemove, serverUrl, token, projectId, onRemoved, onClose }: MemberDetailProps) {
  const online = presence.isOnline(member.id);
  const [confirmingRemoval, setConfirmingRemoval] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [removalError, setRemovalError] = useState<string | null>(null);

  async function removeMember() {
    setRemoving(true);
    setRemovalError(null);
    try {
      await removeProjectMember(serverUrl, token, projectId, member.id);
    } catch (error) {
      setRemovalError(error instanceof ApiError ? error.message : "Could not remove this member.");
      setRemoving(false);
      return;
    }
    onRemoved(member.id);
    onClose();
  }

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

		{online&&<p className="members__note">Work status: {workContext?.working?"Working":"Not working"}<br/>Current branch: {workContext?.current_branch??"Not reported"}</p>}

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
          {member.current_task_title && <p className="members__note">Current task: {member.current_task_title}</p>}
          {canRemove && (
            <button type="button" className="member-detail__remove" onClick={() => setConfirmingRemoval(true)}>
              Remove Member
            </button>
          )}
        </div>

        {confirmingRemoval && (
          <section className="member-detail__confirm" role="alertdialog" aria-labelledby="remove-member-title">
            <h3 id="remove-member-title">Remove {member.username} from this Project?</h3>
            <p>
              Repository access is not configured for this Project, so this only removes KMJG Hub membership.
            </p>
            {removalError && <p className="member-detail__error" role="alert">{removalError}</p>}
            <div className="member-detail__actions">
              <button type="button" onClick={() => setConfirmingRemoval(false)} disabled={removing}>
                Cancel
              </button>
              <button type="button" className="member-detail__remove" onClick={removeMember} disabled={removing}>
                {removing ? "Removing…" : "Remove Member"}
              </button>
            </div>
          </section>
        )}
      </div>
    </div>
  );
}

export default Members;
