import { ReactElement, useEffect, useState } from "react";
import ConnectServer from "./features/connect-server/ConnectServer";
import Login from "./features/auth/Login";
import Register from "./features/auth/Register";
import ServerHome from "./features/server-home/ServerHome";
import Profile from "./features/server-home/Profile";
import CreateProject from "./features/projects/CreateProject";
import ProjectWorkspace from "./features/projects/ProjectWorkspace";
import { PresenceProvider } from "./features/presence/PresenceProvider";
import { ApiError, AuthResponse, checkHealth, validateSession } from "./lib/apiClient";
import { deleteSession, loadSessions, saveSession } from "./lib/nativeClient";
import "./App.css";

type Screen =
	| { kind: "restoring" }
	| { kind: "connect" }
  | { kind: "connecting"; serverUrl: string }
  | { kind: "connect-failed"; serverUrl: string; message: string }
  | { kind: "login"; serverUrl: string }
  | { kind: "register"; serverUrl: string }
  | { kind: "server-home"; serverUrl: string; auth: AuthResponse }
  | { kind: "create-project"; serverUrl: string; auth: AuthResponse }
  | { kind: "profile"; serverUrl: string; auth: AuthResponse }
  | { kind: "project"; serverUrl: string; auth: AuthResponse; projectId: string };

function App() {
	const [screen, setScreen] = useState<Screen>({ kind: "restoring" });

	useEffect(()=>{let cancelled=false;(async()=>{for(const saved of await loadSessions()){try{await validateSession(saved.serverUrl,saved.auth.session.token);if(!cancelled){setScreen({kind:"server-home",serverUrl:saved.serverUrl,auth:saved.auth});return}}catch{await deleteSession(saved.serverUrl)}}if(!cancelled)setScreen({kind:"connect"})})().catch(()=>{if(!cancelled)setScreen({kind:"connect"})});return()=>{cancelled=true}},[]);

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
    let cancelled = false;

    checkHealth(serverUrl)
      .then(() => {
        if (!cancelled) {
          setScreen({ kind: "login", serverUrl });
        }
      })
      .catch((err) => {
        if (cancelled) {
          return;
        }
        const message = err instanceof ApiError ? err.message : "Could not reach the server.";
        setScreen({ kind: "connect-failed", serverUrl, message });
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
      content = <ConnectServer onConnect={(serverUrl) => setScreen({ kind: "connecting", serverUrl })} />;
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
          <button onClick={() => setScreen({ kind: "connect" })}>Try a Different Server</button>
          <button onClick={() => setScreen({ kind: "connecting", serverUrl: screen.serverUrl })}>Retry</button>
        </main>
      );
      break;

    case "login": {
      const serverUrl = screen.serverUrl;
      content = (
        <Login
          serverUrl={serverUrl}
		  onAuthenticated={(auth) => authenticated(serverUrl,auth)}
          onCreateAccount={() => setScreen({ kind: "register", serverUrl })}
          onChangeServer={() => setScreen({ kind: "connect" })}
        />
      );
      break;
    }

    case "register": {
      const serverUrl = screen.serverUrl;
      content = (
        <Register
          serverUrl={serverUrl}
		  onAuthenticated={(auth) => authenticated(serverUrl,auth)}
          onBackToLogin={() => setScreen({ kind: "login", serverUrl })}
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
		  onSessionExpired={() => sessionInvalid(serverUrl)}
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
