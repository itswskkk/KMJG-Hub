import { createContext, ReactNode, useContext, useEffect, useMemo, useState } from "react";
import {
  ConnectionStatus,
  DirectMessageDeletedEvent,
  DirectMessageEvent,
  FriendRealtimeEvent,
  PresenceSnapshotEvent,
  PresenceUpdatedEvent,
  ProjectMessageDeletedEvent,
  ProjectMessageEvent,
  ProjectTaskChangedEvent,
	ProjectWorkContextEvent,
  RealtimeClient,
  toWebSocketUrl,
} from "../../lib/realtimeClient";

interface PresenceState {
  connectionStatus: ConnectionStatus;
  // projectId -> userId -> online. Only ever holds data delivered on the
  // *current* connection generation — cleared whenever the connection stops
  // being open (see the onConnectionStatusChange handler below), so a
  // previous connection's presence can never be read once a new one takes
  // over, even before the new connection's own snapshot arrives.
  projects: Record<string, Record<string, boolean>>;
  // projectId -> true once a presence.snapshot for that Project has been
  // received on the *current* connection generation. This is the sole
  // authority for "is this Project's presence known" — receiving
  // "connected", or a presence.updated event with no prior snapshot, must
  // never set this (see useProjectPresence's `ready`).
  readyProjects: Record<string, boolean>;
  chatEvents: ChatRealtimeEvent[];
  taskEvents: ProjectTaskChangedEvent[];
	workContextEvents: ProjectWorkContextEvent[];
  friendEvents: FriendRealtimeEvent[];
  dmEvents: DMRealtimeEvent[];
}

export type DMRealtimeEvent =
  | { type: "created"; data: DirectMessageEvent }
  | { type: "deleted"; data: DirectMessageDeletedEvent };

export type ChatRealtimeEvent =
  | { type: "created"; data: ProjectMessageEvent }
  | { type: "deleted"; data: ProjectMessageDeletedEvent };

const initialState: PresenceState = { connectionStatus: "closed", projects: {}, readyProjects: {}, chatEvents: [], taskEvents: [], workContextEvents: [], friendEvents: [], dmEvents: [] };

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
        setState((prev) =>
          connectionStatus === "open"
            ? { ...prev, connectionStatus }
            : // The connection just stopped being open (or has never been
              // open yet): any presence already known belongs to a
              // connection that is no longer live. Drop it rather than
              // letting it survive into whatever connection comes next —
              // that next connection's own "connected" must not make this
              // stale (or, for a brand-new project, absent) data look
              // current again.
              { connectionStatus, projects: {}, readyProjects: {}, chatEvents: [], taskEvents: [], workContextEvents: [], friendEvents: [], dmEvents: [] },
        );
      },
      onSnapshot: (data: PresenceSnapshotEvent) => {
        setState((prev) => ({
          ...prev,
          projects: {
            ...prev.projects,
            [data.project_id]: Object.fromEntries(data.members.map((m) => [m.user_id, m.online])),
          },
          readyProjects: { ...prev.readyProjects, [data.project_id]: true },
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
      onProjectMessageCreated: (data) => {
        setState((prev) => ({
          ...prev,
          chatEvents: [...prev.chatEvents.slice(-99), { type: "created", data }],
        }));
      },
      onProjectMessageDeleted: (data) => {
        setState((prev) => ({
          ...prev,
          chatEvents: [...prev.chatEvents.slice(-99), { type: "deleted", data }],
        }));
      },
      onProjectTaskChanged: (data) => {
        setState((prev) => ({ ...prev, taskEvents: [...prev.taskEvents.slice(-99), data] }));
      },
	  onProjectWorkContextUpdated: (data) => {
		setState((prev)=>({...prev,workContextEvents:[...prev.workContextEvents.slice(-99),data]}));
	  },
      onFriendEvent: (event) => {
        setState((prev) => ({ ...prev, friendEvents: [...prev.friendEvents.slice(-99), event] }));
      },
      onDirectMessageCreated: (data) => {
        setState((prev) => ({ ...prev, dmEvents: [...prev.dmEvents.slice(-99), { type: "created", data }] }));
      },
      onDirectMessageDeleted: (data) => {
        setState((prev) => ({ ...prev, dmEvents: [...prev.dmEvents.slice(-99), { type: "deleted", data }] }));
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

/** Returns the current connection generation's Direct Message events. HTTP
 * history remains authoritative and repairs any events missed offline. */
export function useDMEvents(): DMRealtimeEvent[] {
  return useContext(PresenceStateContext).dmEvents;
}

/** Returns the current connection generation's friend/block events. Each
 * is only a reload hint; the HTTP friends endpoints remain authoritative. */
export function useFriendEvents(): FriendRealtimeEvent[] {
  return useContext(PresenceStateContext).friendEvents;
}

export function useProjectWorkContextEvents(projectId:string):ProjectWorkContextEvent[]{
	const state=useContext(PresenceStateContext);
	return useMemo(()=>state.workContextEvents.filter((event)=>event.project_id===projectId),[projectId,state.workContextEvents]);
}

/** Returns the current connection generation's Project Chat events. HTTP
 * history remains authoritative and repairs any events missed offline. */
export function useProjectChatEvents(projectId: string): ChatRealtimeEvent[] {
  const state = useContext(PresenceStateContext);
  return useMemo(
    () => state.chatEvents.filter((event) => event.data.project_id === projectId),
    [projectId, state.chatEvents],
  );
}

/** Returns the current connection generation's task-change events for one
 * Project. Each event is only a reload hint; HTTP remains authoritative. */
export function useProjectTaskEvents(projectId: string): ProjectTaskChangedEvent[] {
  const state = useContext(PresenceStateContext);
  return useMemo(
    () => state.taskEvents.filter((event) => event.project_id === projectId),
    [projectId, state.taskEvents],
  );
}

export interface ProjectPresence {
  /** The underlying real-time connection's status. Note this alone does not
   * mean this Project's presence is known — see `ready`: "open" only means
   * *a* connection is live, not that it has delivered *this* Project's
   * presence.snapshot yet (e.g. right after "connected", or right after
   * reconnecting). */
  status: ConnectionStatus;
  /** True once a fresh presence.snapshot for this Project has been received
   * on the current connection. Members must be presented as Unknown (never
   * as Offline, and never using data from a previous connection) while this
   * is false. */
  ready: boolean;
  /** Returns undefined when this member's presence is not currently known
   * — whenever `ready` is false — callers must not treat undefined as
   * Offline. */
  isOnline: (userId: string) => boolean | undefined;
}

/** Reads real-time presence for one Project from the nearest PresenceProvider. */
export function useProjectPresence(projectId: string): ProjectPresence {
  const state = useContext(PresenceStateContext);
  const ready = state.connectionStatus === "open" && Boolean(state.readyProjects[projectId]);
  return {
    status: state.connectionStatus,
    ready,
    isOnline: (userId: string) => (ready ? state.projects[projectId]?.[userId] : undefined),
  };
}
