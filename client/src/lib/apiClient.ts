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
        "Content-Type": "application/json",
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
}

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

export async function listProjectMessages(serverUrl: string, token: string, projectId: string): Promise<ProjectChatMessage[]> {
  const body = await request<{ messages: ProjectChatMessage[] | null }>(
    serverUrl,
    `/api/v1/projects/${encodeURIComponent(projectId)}/chat/messages`,
    { headers: authHeaders(token) },
  );
  return body.messages ?? [];
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
