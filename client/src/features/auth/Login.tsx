import { FormEvent, useState } from "react";
import { ApiError, AuthResponse, login } from "../../lib/apiClient";
import "./Auth.css";

interface LoginProps {
  serverUrl: string;
  onAuthenticated: (auth: AuthResponse) => void;
  onCreateAccount: () => void;
  onChangeServer: () => void;
}

function Login({ serverUrl, onAuthenticated, onCreateAccount, onChangeServer }: LoginProps) {
  const [identifier, setIdentifier] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setSubmitting(true);

    try {
      const auth = await login(serverUrl, { identifier, password });
      onAuthenticated(auth);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Something went wrong, please try again.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="auth-screen">
      <div className="auth-screen__card">
        <h1>Welcome to KMJG Hub</h1>
        <p className="auth-screen__subtitle">{serverUrl}</p>

        <form className="auth-screen__form" onSubmit={handleSubmit} noValidate>
          <label className="auth-screen__label" htmlFor="login-identifier">
            Username or Email
          </label>
          <input
            id="login-identifier"
            type="text"
            autoComplete="username"
            value={identifier}
            onChange={(event) => setIdentifier(event.target.value)}
          />

          <label className="auth-screen__label" htmlFor="login-password">
            Password
          </label>
          <input
            id="login-password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />

          {error && (
            <p className="auth-screen__error" role="alert">
              {error}
            </p>
          )}

          <button type="submit" disabled={submitting}>
            {submitting ? "Logging In..." : "Log In"}
          </button>
        </form>

        <div className="auth-screen__divider">
          <span>or</span>
        </div>

        <button
          type="button"
          className="auth-screen__secondary-button"
          disabled
          title="GitHub authentication is not available yet"
        >
          Continue with GitHub (Not Available Yet)
        </button>

        <p className="auth-screen__footer">
          Don&apos;t have an account?{" "}
          <button type="button" className="auth-screen__link" onClick={onCreateAccount}>
            Create Account
          </button>
        </p>

        <button type="button" className="auth-screen__link" onClick={onChangeServer}>
          Not this server? Change Server
        </button>
      </div>
    </main>
  );
}

export default Login;
