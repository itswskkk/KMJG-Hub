import { FormEvent, useState } from "react";
import { parseServerAddress } from "../../lib/serverAddress";
import "./ConnectServer.css";

interface ConnectServerProps {
  onConnect: (serverUrl: string) => void;
}

function ConnectServer({ onConnect }: ConnectServerProps) {
  const [address, setAddress] = useState("");
  const [error, setError] = useState<string | null>(null);

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const parsed = parseServerAddress(address);
    if (!parsed) {
      setError("Enter a valid server address, such as hub.example.com");
      return;
    }

    setError(null);
    onConnect(parsed.toString());
  }

  return (
    <main className="connect-server">
      <div className="connect-server__card">
        <h1>KMJG Hub</h1>
        <p className="connect-server__subtitle">
          Connect to a KMJG Hub Server to access your projects and workspace.
        </p>

        <form
          className="connect-server__form"
          onSubmit={handleSubmit}
          noValidate
        >
          <label className="connect-server__label" htmlFor="server-address">
            Server Address
          </label>
          <input
            id="server-address"
            type="text"
            inputMode="url"
            autoComplete="url"
            placeholder="hub.example.com"
            value={address}
            onChange={(event) => {
              setAddress(event.target.value);
              if (error) {
                setError(null);
              }
            }}
            aria-invalid={error ? "true" : "false"}
            aria-describedby={error ? "server-address-error" : undefined}
          />

          {error && (
            <p
              id="server-address-error"
              className="connect-server__error"
              role="alert"
            >
              {error}
            </p>
          )}

          <button type="submit">Connect to Server</button>
        </form>
      </div>
    </main>
  );
}

export default ConnectServer;
