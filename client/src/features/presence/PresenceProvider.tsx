import { createContext, ReactNode, useContext, useEffect, useState } from "react";
import {
  ConnectionStatus,
  PresenceSnapshotEvent,
  PresenceUpdatedEvent,
  RealtimeClient,
  toWebSocketUrl,
} from "../../lib/realtimeClient";

interface PresenceState {
  connectionStatus: ConnectionStatus;
  // projectId -> userId -> online
  projects: Record<string, Record<string, boolean>>;
}

const initialState: PresenceState = { connectionStatus: "closed", projects: {} };

const PresenceStateContext = createContext<PresenceState>(initialState);

interface PresenceProviderProps {
  /** null before the user is authenticated with a Server (no connection is made). */
  serverUrl: string | null;
  /** null before the user is authenticated (no connection is made). */
  token: string | null;
  /** Called when the Server rejects token as an invalid/expired/revoked
   * session — the same condition the rest of the Client already treats as
   * a sign-out (apiClient's isSessionExpired over HTTP). Optional so a
   * screen without anywhere sensible to route back to can omit it. */
  onSessionExpired?: () => void;
  children: ReactNode;
}

/**
 * Owns the single authenticated real-time connection for the whole
 * authenticated app session. Mounted once at the top of the component tree
 * (see App.tsx) so navigating between Server Home, Create Project, and a
 * Project Workspace does not tear down and reopen the connection — only an
 * actual login, logout, or Server change (a change to serverUrl/token)
 * does, matching docs/ARCHITECTURE.md's expectation that reconnection
 * happens for real connectivity changes, not routine navigation.
 */
export function PresenceProvider({ serverUrl, token, onSessionExpired, children }: PresenceProviderProps) {
  const [state, setState] = useState<PresenceState>(initialState);

  useEffect(() => {
    if (!serverUrl || !token) {
      setState(initialState);
      return;
    }

    setState(initialState);

    const client = new RealtimeClient(toWebSocketUrl(serverUrl), token, {
      onConnectionStatusChange: (connectionStatus) => {
        setState((prev) => ({ ...prev, connectionStatus }));
      },
      onSnapshot: (data: PresenceSnapshotEvent) => {
        setState((prev) => ({
          ...prev,
          projects: {
            ...prev.projects,
            [data.project_id]: Object.fromEntries(data.members.map((m) => [m.user_id, m.online])),
          },
        }));
      },
      onUpdated: (data: PresenceUpdatedEvent) => {
        setState((prev) => ({
          ...prev,
          projects: {
            ...prev.projects,
            [data.project_id]: { ...prev.projects[data.project_id], [data.user_id]: data.online },
          },
        }));
      },
      onAuthError: () => {
        onSessionExpired?.();
      },
    });
    client.connect();

    return () => client.close();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverUrl, token]);

  return <PresenceStateContext.Provider value={state}>{children}</PresenceStateContext.Provider>;
}

export interface ProjectPresence {
  /** "open" when live presence is currently known; otherwise unknown/stale. */
  status: ConnectionStatus;
  /** Returns undefined when this member's presence is not currently known
   * (no snapshot received yet, or the real-time connection is not open) —
   * callers must not treat undefined as Offline. */
  isOnline: (userId: string) => boolean | undefined;
}

/** Reads real-time presence for one Project from the nearest PresenceProvider. */
export function useProjectPresence(projectId: string): ProjectPresence {
  const state = useContext(PresenceStateContext);
  return {
    status: state.connectionStatus,
    isOnline: (userId: string) =>
      state.connectionStatus === "open" ? state.projects[projectId]?.[userId] : undefined,
  };
}
