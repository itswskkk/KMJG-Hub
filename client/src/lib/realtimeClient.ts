/**
 * Authenticated WebSocket client for the KMJG Hub Server's real-time layer
 * (docs/ARCHITECTURE.md "Real-Time Communication"). Framework-agnostic: no
 * React here, so it stays testable and reusable independent of how a
 * particular screen consumes it (see features/presence/PresenceProvider.tsx
 * for the React binding).
 */

/** Matches the Server's realtime.Envelope shape: {"v", "type", "data"}. */
interface RawEnvelope {
  v: number;
  type: string;
  data?: unknown;
}

export interface PresenceSnapshotEvent {
  project_id: string;
  members: { user_id: string; online: boolean }[];
}

export interface PresenceUpdatedEvent {
  project_id: string;
  user_id: string;
  online: boolean;
}

export interface ProjectMessageEvent {
  id: string;
  project_id: string;
  author_id: string;
  author_username: string;
  body: string;
  created_at: string;
	attachments: { id:string; message_id:string; project_id:string; filename:string; content_type:string; size_bytes:number }[];
}

export interface ProjectWorkContextEvent {
	project_id:string; user_id:string; working:boolean;
	status_mode:"automatic"|"manual"; current_branch?:string;
}

export interface ProjectMessageDeletedEvent {
  project_id: string;
  message_id: string;
}

export interface ProjectTaskChangedEvent {
  project_id: string;
  task_id: string;
}

export type FriendEventType =
  | "friend_request.sent"
  | "friend_request.accepted"
  | "friend_request.declined"
  | "friend_request.cancelled"
  | "friendship.removed"
  | "user.blocked"
  | "user.unblocked";

const FRIEND_EVENT_TYPES: ReadonlySet<string> = new Set<FriendEventType>([
  "friend_request.sent",
  "friend_request.accepted",
  "friend_request.declined",
  "friend_request.cancelled",
  "friendship.removed",
  "user.blocked",
  "user.unblocked",
]);

/** A friend/block relationship change. Only a reload hint: the HTTP
 * friends endpoints stay authoritative. */
export interface FriendRealtimeEvent {
  type: FriendEventType;
  data: Record<string, unknown>;
}

export interface DirectMessageEvent {
  id: string;
  sender_id: string;
  sender_username: string;
  recipient_id: string;
  recipient_username: string;
  body: string;
  created_at: string;
}

export interface DirectMessageDeletedEvent {
  message_id: string;
  sender_id: string;
  recipient_id: string;
}

/** A newly created notification for the connected user. Matches the HTTP
 * notification shape; the HTTP list stays authoritative (unread count etc.). */
export interface NotificationCreatedEvent {
  id: string;
  event_type: string;
  payload: Record<string, unknown>;
  created_at: string;
  read_at: string | null;
}

export type ConnectionStatus = "connecting" | "open" | "reconnecting" | "closed";

export interface RealtimeClientHandlers {
  onConnectionStatusChange: (status: ConnectionStatus) => void;
  onSnapshot: (data: PresenceSnapshotEvent) => void;
  onUpdated: (data: PresenceUpdatedEvent) => void;
  onProjectMessageCreated?: (data: ProjectMessageEvent) => void;
  onProjectMessageDeleted?: (data: ProjectMessageDeletedEvent) => void;
  onProjectTaskChanged?: (data: ProjectTaskChangedEvent) => void;
	onProjectWorkContextUpdated?: (data: ProjectWorkContextEvent) => void;
  onFriendEvent?: (event: FriendRealtimeEvent) => void;
  onDirectMessageCreated?: (data: DirectMessageEvent) => void;
  onDirectMessageDeleted?: (data: DirectMessageDeletedEvent) => void;
  onNotificationCreated?: (data: NotificationCreatedEvent) => void;
  /** The Server rejected the session (invalid/expired/revoked) — the same
   * condition the Client already treats as a sign-out over HTTP
   * (apiClient's isSessionExpired). Reconnecting with the same token would
   * only repeat the rejection, so the Client stops trying. */
  onAuthError: () => void;
}

/** Converts an HTTP(S) KMJG Hub Server address into its WebSocket endpoint,
 * per docs/ARCHITECTURE.md "Transport Security": "secure WebSocket
 * transport is used for real-time communication" — https -> wss,
 * http -> ws, using the Client's existing selected Server address rather
 * than a hardcoded host. */
export function toWebSocketUrl(serverUrl: string): string {
  const url = new URL("/api/v1/ws", serverUrl);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}

const INITIAL_BACKOFF_MS = 1000;
const MAX_BACKOFF_MS = 30000;

/**
 * Owns exactly one logical WebSocket connection at a time, including
 * authentication and reconnect-with-backoff. Every internal callback checks
 * `this.closed` and that the event belongs to the currently-tracked socket
 * before touching any state, so a socket superseded by `close()` (e.g. React
 * Strict Mode's mount -> cleanup -> mount, or the Provider reconnecting for
 * a new token) can never deliver a stale event after the fact.
 */
export class RealtimeClient {
  private ws: WebSocket | null = null;
  private closed = false;
  private backoffMs = INITIAL_BACKOFF_MS;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private everConnected = false;

  constructor(
    private readonly wsUrl: string,
    private readonly token: string,
    private readonly handlers: RealtimeClientHandlers,
  ) {}

  connect(): void {
    if (this.closed) {
      return;
    }
    this.handlers.onConnectionStatusChange(this.everConnected ? "reconnecting" : "connecting");

    const ws = new WebSocket(this.wsUrl);
    this.ws = ws;

    ws.onopen = () => {
      if (this.closed || this.ws !== ws) {
        return;
      }
      ws.send(JSON.stringify({ v: 1, type: "auth", data: { token: this.token } }));
    };

    ws.onmessage = (event) => {
      if (this.closed || this.ws !== ws) {
        return;
      }
      this.handleMessage(event.data);
    };

    ws.onclose = () => {
      if (this.ws !== ws) {
        return; // already superseded by a newer socket
      }
      this.ws = null;
      if (this.closed) {
        return;
      }
      this.handlers.onConnectionStatusChange("reconnecting");
      this.scheduleReconnect();
    };

    // No separate handling needed: a WebSocket error is always followed by
    // its close event, which already drives reconnect scheduling above.
    ws.onerror = () => {};
  }

  private handleMessage(raw: unknown): void {
    if (typeof raw !== "string") {
      return;
    }
    let envelope: RawEnvelope;
    try {
      envelope = JSON.parse(raw) as RawEnvelope;
    } catch {
      return;
    }

    switch (envelope.type) {
      case "connected":
        this.everConnected = true;
        this.backoffMs = INITIAL_BACKOFF_MS;
        this.handlers.onConnectionStatusChange("open");
        break;
      case "presence.snapshot":
        this.handlers.onSnapshot(envelope.data as PresenceSnapshotEvent);
        break;
      case "presence.updated":
        this.handlers.onUpdated(envelope.data as PresenceUpdatedEvent);
        break;
      case "project.message.created":
        this.handlers.onProjectMessageCreated?.(envelope.data as ProjectMessageEvent);
        break;
      case "project.message.deleted":
        this.handlers.onProjectMessageDeleted?.(envelope.data as ProjectMessageDeletedEvent);
        break;
      case "project.task.changed":
        this.handlers.onProjectTaskChanged?.(envelope.data as ProjectTaskChangedEvent);
        break;
	  case "project.work-context.updated":
		this.handlers.onProjectWorkContextUpdated?.(envelope.data as ProjectWorkContextEvent);
		break;
      case "direct_message.created":
        this.handlers.onDirectMessageCreated?.(envelope.data as DirectMessageEvent);
        break;
      case "direct_message.deleted":
        this.handlers.onDirectMessageDeleted?.(envelope.data as DirectMessageDeletedEvent);
        break;
      case "notification.created":
        this.handlers.onNotificationCreated?.(envelope.data as NotificationCreatedEvent);
        break;
      case "error":
        // The Server closes the connection right after; stop here instead
        // of letting the upcoming close event schedule a doomed retry with
        // the same rejected token.
        this.closed = true;
        if (this.reconnectTimer) {
          clearTimeout(this.reconnectTimer);
          this.reconnectTimer = null;
        }
        this.handlers.onConnectionStatusChange("closed");
        this.handlers.onAuthError();
        break;
      default:
        if (FRIEND_EVENT_TYPES.has(envelope.type)) {
          this.handlers.onFriendEvent?.({
            type: envelope.type as FriendEventType,
            data: (envelope.data ?? {}) as Record<string, unknown>,
          });
        }
        break;
    }
  }

  private scheduleReconnect(): void {
    if (this.closed) {
      return;
    }
    const jitter = Math.random() * 0.3 * this.backoffMs;
    const delay = Math.min(this.backoffMs, MAX_BACKOFF_MS) + jitter;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.backoffMs = Math.min(this.backoffMs * 2, MAX_BACKOFF_MS);
      this.connect();
    }, delay);
  }

  /** Ends this client permanently: no further reconnect attempts, and any
   * in-flight or future events from a superseded socket are ignored. Safe
   * to call more than once. */
  close(): void {
    this.closed = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      const ws = this.ws;
      this.ws = null;
      ws.onopen = null;
      ws.onmessage = null;
      ws.onclose = null;
      ws.onerror = null;
      ws.close();
    }
  }
}
