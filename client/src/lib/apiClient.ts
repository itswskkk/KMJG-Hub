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
