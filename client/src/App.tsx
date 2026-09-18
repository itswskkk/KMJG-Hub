import { ReactElement, useEffect, useState } from "react";
import ConnectServer from "./features/connect-server/ConnectServer";
import Login from "./features/auth/Login";
import Register from "./features/auth/Register";
import ServerHome from "./features/server-home/ServerHome";
import CreateProject from "./features/projects/CreateProject";
import ProjectWorkspace from "./features/projects/ProjectWorkspace";
import { PresenceProvider } from "./features/presence/PresenceProvider";
import { ApiError, AuthResponse, checkHealth } from "./lib/apiClient";
import "./App.css";

type Screen =
  | { kind: "connect" }
  | { kind: "connecting"; serverUrl: string }
  | { kind: "connect-failed"; serverUrl: string; message: string }
  | { kind: "login"; serverUrl: string }
  | { kind: "register"; serverUrl: string }
  | { kind: "server-home"; serverUrl: string; auth: AuthResponse }
  | { kind: "create-project"; serverUrl: string; auth: AuthResponse }
  | { kind: "project"; serverUrl: string; auth: AuthResponse; projectId: string };

function App() {
  const [screen, setScreen] = useState<Screen>({ kind: "connect" });

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
          onAuthenticated={(auth) => setScreen({ kind: "server-home", serverUrl, auth })}
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
          onAuthenticated={(auth) => setScreen({ kind: "server-home", serverUrl, auth })}
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
          onSessionExpired={() => setScreen({ kind: "login", serverUrl })}
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
          onBackToServerHome={() => setScreen({ kind: "server-home", serverUrl, auth })}
          onSessionExpired={() => setScreen({ kind: "login", serverUrl })}
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
      setScreen({ kind: "login", serverUrl: screen.serverUrl });
    }
  }

  return (
    <PresenceProvider serverUrl={presenceServerUrl} token={presenceToken} onSessionExpired={handleSessionInvalid}>
      {content}
    </PresenceProvider>
  );
}

export default App;
