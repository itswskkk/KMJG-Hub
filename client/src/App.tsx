import { ReactElement, useEffect, useState } from "react";
import ConnectServer from "./features/connect-server/ConnectServer";
import SavedServers from "./features/connect-server/SavedServers";
import Login from "./features/auth/Login";
import Register from "./features/auth/Register";
import ServerHome from "./features/server-home/ServerHome";
import Profile from "./features/server-home/Profile";
import Friends from "./features/server-home/Friends";
import DirectMessages from "./features/server-home/DirectMessages";
import Notifications from "./features/server-home/Notifications";
import FileTransfers from "./features/server-home/FileTransfers";
import CreateProject from "./features/projects/CreateProject";
import ProjectWorkspace from "./features/projects/ProjectWorkspace";
import { PresenceProvider } from "./features/presence/PresenceProvider";
import { ApiError, AuthResponse, checkHealth } from "./lib/apiClient";
import { deleteSession, loadSessions, saveSession } from "./lib/nativeClient";
import "./App.css";

// `from` marks a screen reached via "+ Add Server" from "Your Servers"
// (docs/UX.md "Saved Servers"), so ConnectServer/Login/Register can offer a
// way back to that list instead of stranding the user mid-connection.
type ConnectOrigin = "saved-servers" | undefined;

type Screen =
	| { kind: "restoring" }
	| { kind: "connect"; from?: ConnectOrigin }
  | { kind: "saved-servers" }
  | { kind: "connecting"; serverUrl: string; from?: ConnectOrigin }
  | { kind: "connect-failed"; serverUrl: string; message: string; from?: ConnectOrigin }
  | { kind: "login"; serverUrl: string; from?: ConnectOrigin }
  | { kind: "register"; serverUrl: string; from?: ConnectOrigin }
  | { kind: "server-home"; serverUrl: string; auth: AuthResponse }
  | { kind: "create-project"; serverUrl: string; auth: AuthResponse }
  | { kind: "profile"; serverUrl: string; auth: AuthResponse }
  | { kind: "friends"; serverUrl: string; auth: AuthResponse }
  | { kind: "direct-messages"; serverUrl: string; auth: AuthResponse }
  | { kind: "notifications"; serverUrl: string; auth: AuthResponse }
  | { kind: "file-transfers"; serverUrl: string; auth: AuthResponse }
  | { kind: "project"; serverUrl: string; auth: AuthResponse; projectId: string };

function App() {
	const [screen, setScreen] = useState<Screen>({ kind: "restoring" });

	// On launch, never auto-pick a Server to sign into (docs/UX.md "KMJG Hub
	// does not automatically open the user's most recently used Project" —
	// the same principle applies here: don't guess which Server the user
	// wants). Zero saved Servers is still the unchanged first-launch path
	// straight to Connect Server; one or more saved Servers shows "Your
	// Servers" and lets the user choose (session validity is then checked
	// per-Server, in SavedServers, only when the user picks one).
	useEffect(()=>{let cancelled=false;(async()=>{const sessions=await loadSessions();if(!cancelled)setScreen(sessions.length>0?{kind:"saved-servers"}:{kind:"connect"})})().catch(()=>{if(!cancelled)setScreen({kind:"connect"})});return()=>{cancelled=true}},[]);

	const authenticated=(serverUrl:string,auth:AuthResponse)=>{void saveSession(serverUrl,auth).catch(()=>{});setScreen({kind:"server-home",serverUrl,auth})};
	const sessionInvalid=(serverUrl:string)=>{void deleteSession(serverUrl).catch(()=>{});setScreen({kind:"login",serverUrl})};

  // Verify the address the user entered on Connect Server actually reaches a
  // KMJG Hub Server before moving on to authentication, per docs/UX.md
  // "First Launch": "After a successful connection, the user continues to
  // authentication for that Server."
  useEffect(() => {
    if (screen.kind !== "connecting") {
      return;
    }
    const serverUrl = screen.serverUrl;
    const from = screen.from;
    let cancelled = false;

    checkHealth(serverUrl)
      .then(() => {
        if (!cancelled) {
          setScreen({ kind: "login", serverUrl, from });
        }
      })
      .catch((err) => {
        if (cancelled) {
          return;
        }
        const message = err instanceof ApiError ? err.message : "Could not reach the server.";
        setScreen({ kind: "connect-failed", serverUrl, message, from });
      });

    return () => {
      cancelled = true;
    };
  }, [screen]);

  let content: ReactElement;
  switch (screen.kind) {
	case "restoring":
	  content=<main className="container"><h1>KMJG Hub</h1><p>Restoring your saved session…</p></main>;
	  break;
    case "connect":
      content = (
        <ConnectServer
          onConnect={(serverUrl) => setScreen({ kind: "connecting", serverUrl, from: screen.from })}
          onBack={screen.from === "saved-servers" ? () => setScreen({ kind: "saved-servers" }) : undefined}
        />
      );
      break;

    case "saved-servers":
      content = (
        <SavedServers
          onConnected={(serverUrl, auth) => setScreen({ kind: "server-home", serverUrl, auth })}
          onNeedsLogin={(serverUrl) => setScreen({ kind: "login", serverUrl, from: "saved-servers" })}
          onAddServer={() => setScreen({ kind: "connect", from: "saved-servers" })}
        />
      );
      break;

    case "connecting":
      content = (
        <main className="container">
          <h1>Connecting...</h1>
          <p>{screen.serverUrl}</p>
        </main>
      );
      break;

    case "connect-failed":
      content = (
        <main className="container">
          <h1>Couldn&apos;t Connect</h1>
          <p>{screen.serverUrl}</p>
          <p>{screen.message}</p>
          <button onClick={() => setScreen({ kind: "connect", from: screen.from })}>Try a Different Server</button>
          <button onClick={() => setScreen({ kind: "connecting", serverUrl: screen.serverUrl, from: screen.from })}>Retry</button>
        </main>
      );
      break;

    case "login": {
      const serverUrl = screen.serverUrl;
      const from = screen.from;
      content = (
        <Login
          serverUrl={serverUrl}
		  onAuthenticated={(auth) => authenticated(serverUrl,auth)}
          onCreateAccount={() => setScreen({ kind: "register", serverUrl, from })}
          onChangeServer={() => setScreen({ kind: "connect", from })}
        />
      );
      break;
    }

    case "register": {
      const serverUrl = screen.serverUrl;
      const from = screen.from;
      content = (
        <Register
          serverUrl={serverUrl}
		  onAuthenticated={(auth) => authenticated(serverUrl,auth)}
          onBackToLogin={() => setScreen({ kind: "login", serverUrl, from })}
        />
      );
      break;
    }

    case "server-home": {
      const { serverUrl, auth } = screen;
      content = (
        <ServerHome
          serverUrl={serverUrl}
          token={auth.session.token}
          username={auth.user.username}
          onOpenProject={(projectId) => setScreen({ kind: "project", serverUrl, auth, projectId })}
          onCreateProject={() => setScreen({ kind: "create-project", serverUrl, auth })}
          onOpenProfile={() => setScreen({ kind: "profile", serverUrl, auth })}
          onOpenFriends={() => setScreen({ kind: "friends", serverUrl, auth })}
          onOpenMessages={() => setScreen({ kind: "direct-messages", serverUrl, auth })}
          onOpenNotifications={() => setScreen({ kind: "notifications", serverUrl, auth })}
          onOpenFileTransfers={() => setScreen({ kind: "file-transfers", serverUrl, auth })}
		  onSessionExpired={() => sessionInvalid(serverUrl)}
          onSwitchServer={() => setScreen({ kind: "saved-servers" })}
        />
      );
      break;
    }

    case "profile": {
      const { serverUrl, auth } = screen;
      content = (
        <Profile
          serverUrl={serverUrl}
          token={auth.session.token}
          onBack={() => setScreen({ kind: "server-home", serverUrl, auth })}
          onSessionExpired={() => sessionInvalid(serverUrl)}
        />
      );
      break;
    }

    case "friends": {
      const { serverUrl, auth } = screen;
      content = (
        <Friends
          serverUrl={serverUrl}
          token={auth.session.token}
          onBack={() => setScreen({ kind: "server-home", serverUrl, auth })}
          onSessionExpired={() => sessionInvalid(serverUrl)}
        />
      );
      break;
    }

    case "direct-messages": {
      const { serverUrl, auth } = screen;
      content = (
        <DirectMessages
          serverUrl={serverUrl}
          token={auth.session.token}
          viewerUserId={auth.user.id}
          onBack={() => setScreen({ kind: "server-home", serverUrl, auth })}
          onSessionExpired={() => sessionInvalid(serverUrl)}
        />
      );
      break;
    }

    case "notifications": {
      const { serverUrl, auth } = screen;
      content = (
        <Notifications
          serverUrl={serverUrl}
          token={auth.session.token}
          onBack={() => setScreen({ kind: "server-home", serverUrl, auth })}
          onOpenProject={(projectId) => setScreen({ kind: "project", serverUrl, auth, projectId })}
          onOpenFriends={() => setScreen({ kind: "friends", serverUrl, auth })}
          onOpenMessages={() => setScreen({ kind: "direct-messages", serverUrl, auth })}
          onOpenFileTransfers={() => setScreen({ kind: "file-transfers", serverUrl, auth })}
          onSessionExpired={() => sessionInvalid(serverUrl)}
        />
      );
      break;
    }

    case "file-transfers": {
      const { serverUrl, auth } = screen;
      content = (
        <FileTransfers
          serverUrl={serverUrl}
          token={auth.session.token}
          onBack={() => setScreen({ kind: "server-home", serverUrl, auth })}
          onSessionExpired={() => sessionInvalid(serverUrl)}
        />
      );
      break;
    }

    case "create-project": {
      const { serverUrl, auth } = screen;
      content = (
        <CreateProject
          serverUrl={serverUrl}
          token={auth.session.token}
          onCreated={(project) => setScreen({ kind: "project", serverUrl, auth, projectId: project.id })}
          onCancel={() => setScreen({ kind: "server-home", serverUrl, auth })}
        />
      );
      break;
    }

    case "project": {
      const { serverUrl, auth, projectId } = screen;
      content = (
        <ProjectWorkspace
          serverUrl={serverUrl}
          token={auth.session.token}
          projectId={projectId}
          viewerUserId={auth.user.id}
          onBackToServerHome={() => setScreen({ kind: "server-home", serverUrl, auth })}
		  onSessionExpired={() => sessionInvalid(serverUrl)}
        />
      );
      break;
    }
  }

  // The real-time connection is owned here, at a position stable across
  // every screen transition, so switching between Server Home, Create
  // Project, and a Project Workspace never tears the connection down (see
  // PresenceProvider). It only reconnects when serverUrl/token actually
  // change: login, logout, or a different Server.
  const presenceServerUrl = "serverUrl" in screen ? screen.serverUrl : null;
  const presenceToken = "auth" in screen ? screen.auth.session.token : null;

  // If the real-time layer rejects the current session (invalid, expired,
  // or revoked — e.g. it expired while this Client was already connected),
  // treat it the same way an HTTP 401 is already treated elsewhere: return
  // to that Server's Login screen rather than silently retrying with a
  // token the Server has already rejected.
	function handleSessionInvalid() {
	  if ("serverUrl" in screen) {
		sessionInvalid(screen.serverUrl);
    }
  }

  return (
    <PresenceProvider serverUrl={presenceServerUrl} token={presenceToken} onSessionExpired={handleSessionInvalid}>
      {content}
    </PresenceProvider>
  );
}

export default App;
