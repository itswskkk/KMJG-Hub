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
}

export interface ProjectMessageDeletedEvent {
  project_id: string;
  message_id: string;
}

export type ConnectionStatus = "connecting" | "open" | "reconnecting" | "closed";

export interface RealtimeClientHandlers {
  onConnectionStatusChange: (status: ConnectionStatus) => void;
  onSnapshot: (data: PresenceSnapshotEvent) => void;
  onUpdated: (data: PresenceUpdatedEvent) => void;
  onProjectMessageCreated?: (data: ProjectMessageEvent) => void;
  onProjectMessageDeleted?: (data: ProjectMessageDeletedEvent) => void;
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
