import { useEffect, useState } from "react";
import ConnectServer from "./features/connect-server/ConnectServer";
import Login from "./features/auth/Login";
import Register from "./features/auth/Register";
import ServerHome from "./features/server-home/ServerHome";
import CreateProject from "./features/projects/CreateProject";
import ProjectWorkspace from "./features/projects/ProjectWorkspace";
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

  switch (screen.kind) {
    case "connect":
      return <ConnectServer onConnect={(serverUrl) => setScreen({ kind: "connecting", serverUrl })} />;

    case "connecting":
      return (
        <main className="container">
          <h1>Connecting...</h1>
          <p>{screen.serverUrl}</p>
        </main>
      );

    case "connect-failed":
      return (
        <main className="container">
          <h1>Couldn&apos;t Connect</h1>
          <p>{screen.serverUrl}</p>
          <p>{screen.message}</p>
          <button onClick={() => setScreen({ kind: "connect" })}>Try a Different Server</button>
          <button onClick={() => setScreen({ kind: "connecting", serverUrl: screen.serverUrl })}>Retry</button>
        </main>
      );

    case "login": {
      const serverUrl = screen.serverUrl;
      return (
        <Login
          serverUrl={serverUrl}
          onAuthenticated={(auth) => setScreen({ kind: "server-home", serverUrl, auth })}
          onCreateAccount={() => setScreen({ kind: "register", serverUrl })}
          onChangeServer={() => setScreen({ kind: "connect" })}
        />
      );
    }

    case "register": {
      const serverUrl = screen.serverUrl;
      return (
        <Register
          serverUrl={serverUrl}
          onAuthenticated={(auth) => setScreen({ kind: "server-home", serverUrl, auth })}
          onBackToLogin={() => setScreen({ kind: "login", serverUrl })}
        />
      );
    }

    case "server-home": {
      const { serverUrl, auth } = screen;
      return (
        <ServerHome
          serverUrl={serverUrl}
          token={auth.session.token}
          username={auth.user.username}
          onOpenProject={(projectId) => setScreen({ kind: "project", serverUrl, auth, projectId })}
          onCreateProject={() => setScreen({ kind: "create-project", serverUrl, auth })}
          onSessionExpired={() => setScreen({ kind: "login", serverUrl })}
        />
      );
    }

    case "create-project": {
      const { serverUrl, auth } = screen;
      return (
        <CreateProject
          serverUrl={serverUrl}
          token={auth.session.token}
          onCreated={(project) => setScreen({ kind: "project", serverUrl, auth, projectId: project.id })}
          onCancel={() => setScreen({ kind: "server-home", serverUrl, auth })}
        />
      );
    }

    case "project": {
      const { serverUrl, auth, projectId } = screen;
      return (
        <ProjectWorkspace
          serverUrl={serverUrl}
          token={auth.session.token}
          projectId={projectId}
          onBackToServerHome={() => setScreen({ kind: "server-home", serverUrl, auth })}
          onSessionExpired={() => setScreen({ kind: "login", serverUrl })}
        />
      );
    }
  }
}

export default App;
