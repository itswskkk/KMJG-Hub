/**
 * Thin typed client for the KMJG Hub Server HTTP API
 * (docs/ARCHITECTURE.md "HTTP API", versioned under /api/v1).
 */

export class ApiError extends Error {
  status: number;
  code: string;
  field?: string;

  constructor(status: number, code: string, message: string, field?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.field = field;
  }
}

export interface KmjgUser {
  id: string;
  username: string;
  email: string;
  created_at: string;
}

export interface AuthSession {
  token: string;
  expires_at: string;
}

export interface AuthResponse {
  user: KmjgUser;
  session: AuthSession;
}

interface ErrorResponseBody {
  error: {
    code: string;
    message: string;
    field?: string;
  };
}

async function request<T>(
  serverUrl: string,
  path: string,
  init?: RequestInit,
): Promise<T> {
  const url = new URL(path, serverUrl).toString();

  let response: Response;
  try {
    response = await fetch(url, {
      ...init,
      headers: {
		...(init?.body instanceof FormData ? {} : { "Content-Type": "application/json" }),
        ...init?.headers,
      },
    });
  } catch {
    throw new ApiError(0, "network_error", "Could not reach the server. Check the server address and your connection.");
  }

  if (response.status === 204) {
    return undefined as T;
  }

  let body: unknown;
  try {
    body = await response.json();
  } catch {
    body = undefined;
  }

  if (!response.ok) {
    const errorBody = body as ErrorResponseBody | undefined;
    const detail = errorBody?.error;
    throw new ApiError(
      response.status,
      detail?.code ?? "unknown_error",
      detail?.message ?? `Request failed with status ${response.status}`,
      detail?.field,
    );
  }

  return body as T;
}

/** Confirms the given address is reachable and serving KMJG Hub. */
export async function checkHealth(serverUrl: string): Promise<void> {
  await request<{ status: string }>(serverUrl, "/api/v1/health");
}

export function registerAccount(
  serverUrl: string,
  input: { username: string; email: string; password: string },
): Promise<AuthResponse> {
  return request<AuthResponse>(serverUrl, "/api/v1/auth/register", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function login(
  serverUrl: string,
  input: { identifier: string; password: string },
): Promise<AuthResponse> {
  return request<AuthResponse>(serverUrl, "/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function logout(serverUrl: string, token: string): Promise<void> {
  return request<void>(serverUrl, "/api/v1/auth/logout", {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
  });
}

export function validateSession(serverUrl:string,token:string):Promise<KmjgUser>{return request(serverUrl,"/api/v1/auth/session",{headers:{Authorization:`Bearer ${token}`}})}

/** True when the server rejected the request because the session token is
 * missing, expired, or revoked — the Client should send the user back to
 * Login rather than show this as an ordinary error. */
export function isSessionExpired(err: unknown): boolean {
  return err instanceof ApiError && (err.code === "invalid_session" || err.code === "unauthorized");
}

export interface ProjectSummary {
  id: string;
  name: string;
  description: string;
  created_at: string;
  member_count: number;
  role: string;
}

export interface ProjectMember {
  id: string;
  username: string;
  role: string;
  current_task_title: string | null;
}

export interface ProjectDetail {
  id: string;
  name: string;
  description: string;
  created_at: string;
  role: string;
  members: ProjectMember[];
}

export interface ProjectChatMessage {
  id: string;
  project_id: string;
  author_id: string;
  author_username: string;
  body: string;
  created_at: string;
	attachments: ProjectChatAttachment[];
}

export interface ProjectChatAttachment {
	id: string;
	message_id: string;
	project_id: string;
	filename: string;
	content_type: string;
	size_bytes: number;
}

export interface ProjectChatPage {
	messages: ProjectChatMessage[];
	next_cursor: string;
}

export interface ProjectWorkContext {
	project_id: string;
	user_id: string;
	working: boolean;
	status_mode: "automatic" | "manual";
	current_branch?: string;
}

export type TaskStatus = "todo" | "in_progress" | "done";

export interface ProjectTask {
  id: string;
  project_id: string;
  title: string;
  description: string;
  status: TaskStatus;
  creator_id: string;
  creator_username: string;
  assignee_id: string | null;
  assignee_username: string | null;
  due_date: string | null;
  created_at: string;
  updated_at: string;
}

export interface TaskComment {
  id: string;
  task_id: string;
  author_id: string;
  author_username: string;
  body: string;
  created_at: string;
}
export interface TaskAssignmentRequest { id: string; task_id: string; project_id: string; task_title: string; requester_username: string; created_at: string; }

export interface DirectInvitation {
  id: string;
  project_id: string;
  project_name: string;
  inviter_username: string;
  recipient_username: string;
  created_at: string;
  expires_at: string | null;
}

export interface InviteCredential {
  id: string;
  project_id: string;
  project_name: string;
  creator_username: string;
  code?: string;
  created_at: string;
  expires_at: string | null;
  max_uses: number | null;
  uses: number;
}

function authHeaders(token: string): HeadersInit {
  return { Authorization: `Bearer ${token}` };
}

export async function listProjects(serverUrl: string, token: string): Promise<ProjectSummary[]> {
  const body = await request<{ projects: ProjectSummary[] | null }>(serverUrl, "/api/v1/projects", {
    headers: authHeaders(token),
  });
  return body.projects ?? [];
}

export function createProject(
  serverUrl: string,
  token: string,
  input: { name: string; description: string },
): Promise<ProjectSummary> {
  return request<ProjectSummary>(serverUrl, "/api/v1/projects", {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify(input),
  });
}

export function getProject(serverUrl: string, token: string, projectId: string): Promise<ProjectDetail> {
  return request<ProjectDetail>(serverUrl, `/api/v1/projects/${encodeURIComponent(projectId)}`, {
    headers: authHeaders(token),
  });
}

export async function listProjectMessages(serverUrl: string, token: string, projectId: string, cursor = ""): Promise<ProjectChatPage> {
	const body = await request<{ messages: ProjectChatMessage[] | null; next_cursor?: string }>(
		serverUrl,
		`/api/v1/projects/${encodeURIComponent(projectId)}/chat/messages${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ""}`,
		{ headers: authHeaders(token) },
	);
	return { messages: body.messages ?? [], next_cursor: body.next_cursor ?? "" };
}

export function sendProjectMessage(serverUrl: string, token: string, projectId: string, body: string): Promise<ProjectChatMessage> {
  return request<ProjectChatMessage>(serverUrl, `/api/v1/projects/${encodeURIComponent(projectId)}/chat/messages`, {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify({ body }),
  });
}

export function deleteProjectMessage(serverUrl: string, token: string, projectId: string, messageId: string): Promise<void> {
  return request<void>(
    serverUrl,
    `/api/v1/projects/${encodeURIComponent(projectId)}/chat/messages/${encodeURIComponent(messageId)}`,
    { method: "DELETE", headers: authHeaders(token) },
  );
}

export function uploadProjectAttachment(serverUrl: string,token: string,projectId: string,file: File,body: string):Promise<ProjectChatMessage>{
	const form=new FormData();form.append("file",file);form.append("body",body);
	return request<ProjectChatMessage>(serverUrl,`/api/v1/projects/${encodeURIComponent(projectId)}/chat/attachments`,{method:"POST",headers:authHeaders(token),body:form});
}

export async function downloadProjectAttachment(serverUrl:string,token:string,projectId:string,attachment:ProjectChatAttachment):Promise<void>{
	const response=await fetch(new URL(`/api/v1/projects/${encodeURIComponent(projectId)}/chat/attachments/${encodeURIComponent(attachment.id)}`,serverUrl),{headers:authHeaders(token)});
	if(!response.ok){let detail:ErrorResponseBody|undefined;try{detail=await response.json() as ErrorResponseBody}catch{};throw new ApiError(response.status,detail?.error.code??"unknown_error",detail?.error.message??`Request failed with status ${response.status}`)}
	const url=URL.createObjectURL(await response.blob());const link=document.createElement("a");link.href=url;link.download=attachment.filename;link.click();URL.revokeObjectURL(url);
}

export async function listProjectWorkContexts(serverUrl:string,token:string,projectId:string):Promise<ProjectWorkContext[]>{
	const body=await request<{contexts:ProjectWorkContext[]|null}>(serverUrl,`/api/v1/projects/${encodeURIComponent(projectId)}/work-contexts`,{headers:authHeaders(token)});return body.contexts??[];
}

export function updateProjectWorkContext(serverUrl:string,token:string,projectId:string,input:{working:boolean;status_mode:"automatic"|"manual";current_branch:string}):Promise<ProjectWorkContext>{
	return request(serverUrl,`/api/v1/projects/${encodeURIComponent(projectId)}/work-context`,{method:"PUT",headers:authHeaders(token),body:JSON.stringify(input)});
}

export async function listProjectTasks(serverUrl: string, token: string, projectId: string): Promise<ProjectTask[]> {
  const body = await request<{ tasks: ProjectTask[] | null }>(serverUrl, `/api/v1/projects/${encodeURIComponent(projectId)}/tasks`, {
    headers: authHeaders(token),
  });
  return body.tasks ?? [];
}

export function createProjectTask(
  serverUrl: string,
  token: string,
  projectId: string,
  input: { title: string; description: string; due_date: string | null },
): Promise<ProjectTask> {
  return request<ProjectTask>(serverUrl, `/api/v1/projects/${encodeURIComponent(projectId)}/tasks`, {
    method: "POST", headers: authHeaders(token), body: JSON.stringify(input),
  });
}

export function setProjectTaskStatus(
  serverUrl: string, token: string, projectId: string, taskId: string, status: TaskStatus,
): Promise<ProjectTask> {
  return request<ProjectTask>(serverUrl, `/api/v1/projects/${encodeURIComponent(projectId)}/tasks/${encodeURIComponent(taskId)}/status`, {
    method: "PATCH", headers: authHeaders(token), body: JSON.stringify({ status }),
  });
}

export function assignProjectTask(serverUrl: string, token: string, projectId: string, taskId: string, assigneeId: string): Promise<ProjectTask | { assignment_request_id: string; status: string }> {
  return request<ProjectTask>(serverUrl, `/api/v1/projects/${encodeURIComponent(projectId)}/tasks/${encodeURIComponent(taskId)}/assign`, {
    method: "POST", headers: authHeaders(token), body: JSON.stringify({ assignee_id: assigneeId }),
  });
}

export function setCurrentProjectTask(serverUrl: string, token: string, projectId: string, taskId: string): Promise<ProjectTask> {
  return request<ProjectTask>(serverUrl, `/api/v1/projects/${encodeURIComponent(projectId)}/tasks/${encodeURIComponent(taskId)}/current`, {
    method: "POST", headers: authHeaders(token),
  });
}

export async function listTaskComments(serverUrl: string, token: string, projectId: string, taskId: string): Promise<TaskComment[]> {
  const body = await request<{ comments: TaskComment[] | null }>(serverUrl, `/api/v1/projects/${encodeURIComponent(projectId)}/tasks/${encodeURIComponent(taskId)}/comments`, {
    headers: authHeaders(token),
  });
  return body.comments ?? [];
}

export function addTaskComment(serverUrl: string, token: string, projectId: string, taskId: string, body: string): Promise<TaskComment> {
  return request<TaskComment>(serverUrl, `/api/v1/projects/${encodeURIComponent(projectId)}/tasks/${encodeURIComponent(taskId)}/comments`, {
    method: "POST", headers: authHeaders(token), body: JSON.stringify({ body }),
  });
}
export async function listTaskAssignmentRequests(serverUrl: string, token: string): Promise<TaskAssignmentRequest[]> { const body = await request<{ requests: TaskAssignmentRequest[] | null }>(serverUrl, "/api/v1/task-assignment-requests", { headers: authHeaders(token) }); return body.requests ?? []; }
export function respondTaskAssignment(serverUrl: string, token: string, id: string, accept: boolean): Promise<ProjectTask> { return request<ProjectTask>(serverUrl, `/api/v1/task-assignment-requests/${encodeURIComponent(id)}/${accept ? "accept" : "decline"}`, { method: "POST", headers: authHeaders(token) }); }

/** Removes a different member when the authenticated Project role permits it. */
export function removeProjectMember(
  serverUrl: string,
  token: string,
  projectId: string,
  userId: string,
): Promise<void> {
  return request<void>(
    serverUrl,
    `/api/v1/projects/${encodeURIComponent(projectId)}/members/${encodeURIComponent(userId)}`,
    {
      method: "DELETE",
      headers: authHeaders(token),
    },
  );
}

export function createDirectInvitation(serverUrl: string, token: string, projectId: string, recipient: string, expiresIn: string): Promise<DirectInvitation> {
  return request(serverUrl, `/api/v1/projects/${encodeURIComponent(projectId)}/invitations`, { method: "POST", headers: authHeaders(token), body: JSON.stringify({ recipient, expires_in: expiresIn }) });
}

export async function listReceivedInvitations(serverUrl: string, token: string): Promise<DirectInvitation[]> {
  const body = await request<{ invitations: DirectInvitation[] | null }>(serverUrl, "/api/v1/invitations", { headers: authHeaders(token) });
  return body.invitations ?? [];
}

export async function listProjectInvitations(serverUrl: string, token: string, projectId: string): Promise<DirectInvitation[]> {
  const body = await request<{ invitations: DirectInvitation[] | null }>(serverUrl, `/api/v1/projects/${encodeURIComponent(projectId)}/invitations`, { headers: authHeaders(token) });
  return body.invitations ?? [];
}

export function acceptInvitation(serverUrl: string, token: string, id: string): Promise<{ project_id: string }> {
  return request(serverUrl, `/api/v1/invitations/${encodeURIComponent(id)}/accept`, { method: "POST", headers: authHeaders(token) });
}

export function declineInvitation(serverUrl: string, token: string, id: string): Promise<void> {
  return request(serverUrl, `/api/v1/invitations/${encodeURIComponent(id)}/decline`, { method: "POST", headers: authHeaders(token) });
}

export function cancelInvitation(serverUrl: string, token: string, projectId: string, id: string): Promise<void> {
  return request(serverUrl, `/api/v1/projects/${encodeURIComponent(projectId)}/invitations/${encodeURIComponent(id)}`, { method: "DELETE", headers: authHeaders(token) });
}

export function createInviteCredential(serverUrl: string, token: string, projectId: string, expiresIn: string, maxUses: number | null): Promise<InviteCredential> {
  return request(serverUrl, `/api/v1/projects/${encodeURIComponent(projectId)}/invite-credentials`, { method: "POST", headers: authHeaders(token), body: JSON.stringify({ expires_in: expiresIn, max_uses: maxUses }) });
}

export async function listInviteCredentials(serverUrl: string, token: string, projectId: string): Promise<InviteCredential[]> {
  const body = await request<{ credentials: InviteCredential[] | null }>(serverUrl, `/api/v1/projects/${encodeURIComponent(projectId)}/invite-credentials`, { headers: authHeaders(token) });
  return body.credentials ?? [];
}

export function revokeInviteCredential(serverUrl: string, token: string, projectId: string, id: string): Promise<void> {
  return request(serverUrl, `/api/v1/projects/${encodeURIComponent(projectId)}/invite-credentials/${encodeURIComponent(id)}`, { method: "DELETE", headers: authHeaders(token) });
}

export function joinProjectWithInvite(serverUrl: string, token: string, invite: string): Promise<{ project_id: string }> {
  return request(serverUrl, "/api/v1/invitations/join", { method: "POST", headers: authHeaders(token), body: JSON.stringify({ invite }) });
}

/** A profile field's visibility audience (docs/PRD.md § User Profiles). */
export type PrivacyAudience =
  | "everyone"
  | "friends"
  | "project_members"
  | "friends_and_project_members"
  | "nobody";

/** A profile field that can carry its own privacy audience. */
export type ProfileField =
  | "display_name"
  | "avatar"
  | "bio"
  | "current_project"
  | "current_task"
  | "current_branch"
  | "repositories";

export interface ProfilePrivacySetting {
  field: ProfileField;
  audience: PrivacyAudience;
}

/**
 * A user profile, as returned by the profile endpoints. `email` and
 * `privacy` are only ever present on the viewer's own profile (GET
 * /api/v1/users/me) — the Server omits them entirely from a public
 * profile's response rather than sending them empty, so their absence here
 * is exactly what to check for "is this my own profile".
 */
export interface UserProfile {
  user_id: string;
  username: string;
  email?: string;
  display_name?: string | null;
  avatar?: string | null;
  bio?: string | null;
  presence?: "online" | "offline" | null;
  current_project?: string | null;
  current_task?: string | null;
  current_branch?: string | null;
  repositories?: string[];
  privacy?: ProfilePrivacySetting[];
}

/** The authenticated user's own full profile, including email and privacy. */
export function getOwnProfile(serverUrl: string, token: string): Promise<UserProfile> {
  return request<UserProfile>(serverUrl, "/api/v1/users/me", { headers: authHeaders(token) });
}

/** Another user's profile, filtered by their privacy settings and their relationship to the viewer. */
export function getPublicProfile(serverUrl: string, token: string, userId: string): Promise<UserProfile> {
  return request<UserProfile>(serverUrl, `/api/v1/users/${encodeURIComponent(userId)}/profile`, {
    headers: authHeaders(token),
  });
}

/**
 * Updates the authenticated user's own profile fields. Omit a key to leave
 * it unchanged; pass "" to clear it. Returns the updated own profile.
 */
export function updateProfile(
  serverUrl: string,
  token: string,
  input: { display_name?: string; avatar?: string; bio?: string },
): Promise<UserProfile> {
  return request<UserProfile>(serverUrl, "/api/v1/users/profile", {
    method: "PUT",
    headers: authHeaders(token),
    body: JSON.stringify(input),
  });
}

/** Sets the visibility audience for one or more of the authenticated user's own profile fields. */
export function setProfilePrivacy(
  serverUrl: string,
  token: string,
  settings: ProfilePrivacySetting[],
): Promise<UserProfile> {
  return request<UserProfile>(serverUrl, "/api/v1/users/profile/privacy", {
    method: "PUT",
    headers: authHeaders(token),
    body: JSON.stringify({ settings }),
  });
}

/** A friend request (docs/PRD.md § Friend Requests). */
export interface FriendRequest {
  id: string;
  sender_id: string;
  sender_username: string;
  recipient_id: string;
  recipient_username: string;
  status: "pending" | "accepted" | "declined";
  created_at: string;
  responded_at?: string | null;
}

/** One entry in the viewer's friends list. */
export interface Friend {
  user_id: string;
  username: string;
  since: string;
}

/** A user the viewer has blocked. */
export interface BlockedUser {
  user_id: string;
  username: string;
  since: string;
}

/** Sends a friend request to a user named by username or email. */
export function sendFriendRequest(serverUrl: string, token: string, recipient: string): Promise<FriendRequest> {
  return request<FriendRequest>(serverUrl, "/api/v1/friends/requests", {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify({ recipient }),
  });
}

/** Sends a friend request to a user by ID. */
export function sendFriendRequestToUser(serverUrl: string, token: string, recipientId: string): Promise<FriendRequest> {
  return request<FriendRequest>(serverUrl, "/api/v1/friends/requests", {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify({ recipient_id: recipientId }),
  });
}

/** Pending friend requests the viewer has received. */
export async function listIncomingRequests(serverUrl: string, token: string): Promise<FriendRequest[]> {
  const body = await request<{ requests: FriendRequest[] | null }>(serverUrl, "/api/v1/friends/requests/incoming", { headers: authHeaders(token) });
  return body.requests ?? [];
}

/** Pending friend requests the viewer has sent. */
export async function listOutgoingRequests(serverUrl: string, token: string): Promise<FriendRequest[]> {
  const body = await request<{ requests: FriendRequest[] | null }>(serverUrl, "/api/v1/friends/requests/outgoing", { headers: authHeaders(token) });
  return body.requests ?? [];
}

export function acceptRequest(serverUrl: string, token: string, requestId: string): Promise<FriendRequest> {
  return request<FriendRequest>(serverUrl, `/api/v1/friends/requests/${encodeURIComponent(requestId)}/accept`, { method: "POST", headers: authHeaders(token) });
}

export function declineRequest(serverUrl: string, token: string, requestId: string): Promise<FriendRequest> {
  return request<FriendRequest>(serverUrl, `/api/v1/friends/requests/${encodeURIComponent(requestId)}/decline`, { method: "POST", headers: authHeaders(token) });
}

/** Withdraws a pending friend request the viewer sent. */
export function cancelRequest(serverUrl: string, token: string, requestId: string): Promise<void> {
  return request<void>(serverUrl, `/api/v1/friends/requests/${encodeURIComponent(requestId)}`, { method: "DELETE", headers: authHeaders(token) });
}

export async function listFriends(serverUrl: string, token: string): Promise<Friend[]> {
  const body = await request<{ friends: Friend[] | null }>(serverUrl, "/api/v1/friends", { headers: authHeaders(token) });
  return body.friends ?? [];
}

export function removeFriend(serverUrl: string, token: string, userId: string): Promise<void> {
  return request<void>(serverUrl, `/api/v1/friends/${encodeURIComponent(userId)}`, { method: "DELETE", headers: authHeaders(token) });
}

/** Blocks a user by ID. Removes any friendship and pending requests with them. */
export function block(serverUrl: string, token: string, userId: string): Promise<BlockedUser> {
  return request<BlockedUser>(serverUrl, "/api/v1/blocked", { method: "POST", headers: authHeaders(token), body: JSON.stringify({ user_id: userId }) });
}

/** Blocks a user named by username or email. */
export function blockByUsername(serverUrl: string, token: string, username: string): Promise<BlockedUser> {
  return request<BlockedUser>(serverUrl, "/api/v1/blocked", { method: "POST", headers: authHeaders(token), body: JSON.stringify({ username }) });
}

/** Unblocks a user. Does not restore any previous friendship. */
export function unblock(serverUrl: string, token: string, userId: string): Promise<void> {
  return request<void>(serverUrl, `/api/v1/blocked/${encodeURIComponent(userId)}`, { method: "DELETE", headers: authHeaders(token) });
}

export async function listBlocked(serverUrl: string, token: string): Promise<BlockedUser[]> {
  const body = await request<{ blocked: BlockedUser[] | null }>(serverUrl, "/api/v1/blocked", { headers: authHeaders(token) });
  return body.blocked ?? [];
}

/** One Direct Message between the viewer and another user
 * (docs/PRD.md § Friends and Direct Messages). */
export interface DirectMessage {
  id: string;
  sender_id: string;
  sender_username: string;
  recipient_id: string;
  recipient_username: string;
  body: string;
  created_at: string;
}

/** One DM thread in the viewer's conversation list, with its latest message. */
export interface Conversation {
  other_user_id: string;
  other_username: string;
  last_message_body: string;
  last_message_at: string;
  last_message_from_me: boolean;
}

export interface DirectMessagePage {
  messages: DirectMessage[];
  next_cursor: string;
}

export async function listConversations(serverUrl: string, token: string): Promise<Conversation[]> {
  const body = await request<{ conversations: Conversation[] | null }>(serverUrl, "/api/v1/direct-messages/conversations", { headers: authHeaders(token) });
  return body.conversations ?? [];
}

/** A page of the conversation with otherUserId, oldest first. Pass the
 * previous page's next_cursor to load older history. */
export async function listDMMessages(serverUrl: string, token: string, otherUserId: string, cursor = ""): Promise<DirectMessagePage> {
  const body = await request<{ messages: DirectMessage[] | null; next_cursor?: string }>(
    serverUrl,
    `/api/v1/direct-messages/${encodeURIComponent(otherUserId)}${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ""}`,
    { headers: authHeaders(token) },
  );
  return { messages: body.messages ?? [], next_cursor: body.next_cursor ?? "" };
}

/** Sends a Direct Message. The Server rejects it (403) unless the two users
 * are friends or share a Project, and neither has blocked the other. */
export function sendDM(serverUrl: string, token: string, recipientId: string, body: string): Promise<DirectMessage> {
  return request<DirectMessage>(serverUrl, `/api/v1/direct-messages/${encodeURIComponent(recipientId)}`, {
    method: "POST",
    headers: authHeaders(token),
    body: JSON.stringify({ body }),
  });
}

/** Deletes a Direct Message the viewer sent (sender only). */
export function deleteDM(serverUrl: string, token: string, messageId: string): Promise<void> {
  return request<void>(serverUrl, `/api/v1/direct-messages/${encodeURIComponent(messageId)}`, { method: "DELETE", headers: authHeaders(token) });
}
