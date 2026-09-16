import { FormEvent, useState } from "react";
import { ApiError, AuthResponse, registerAccount } from "../../lib/apiClient";
import "./Auth.css";

interface RegisterProps {
  serverUrl: string;
  onAuthenticated: (auth: AuthResponse) => void;
  onBackToLogin: () => void;
}

function Register({ serverUrl, onAuthenticated, onBackToLogin }: RegisterProps) {
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setSubmitting(true);

    try {
      const auth = await registerAccount(serverUrl, { username, email, password });
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
        <h1>Create Account</h1>
        <p className="auth-screen__subtitle">{serverUrl}</p>

        <form className="auth-screen__form" onSubmit={handleSubmit} noValidate>
          <label className="auth-screen__label" htmlFor="register-username">
            Username
          </label>
          <input
            id="register-username"
            type="text"
            autoComplete="username"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
          />

          <label className="auth-screen__label" htmlFor="register-email">
            Email
          </label>
          <input
            id="register-email"
            type="email"
            autoComplete="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />

          <label className="auth-screen__label" htmlFor="register-password">
            Password
          </label>
          <input
            id="register-password"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />

          {error && (
            <p className="auth-screen__error" role="alert">
              {error}
            </p>
          )}

          <button type="submit" disabled={submitting}>
            {submitting ? "Creating Account..." : "Create Account"}
          </button>
        </form>

        <p className="auth-screen__footer">
          Already have an account?{" "}
          <button type="button" className="auth-screen__link" onClick={onBackToLogin}>
            Log In
          </button>
        </p>
      </div>
    </main>
  );
}

export default Register;
