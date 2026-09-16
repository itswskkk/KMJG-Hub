import { useState } from "react";
import ConnectServer from "./features/connect-server/ConnectServer";
import "./App.css";

function App() {
  const [serverAddress, setServerAddress] = useState<string | null>(null);

  if (!serverAddress) {
    return <ConnectServer onConnect={setServerAddress} />;
  }

  return (
    <main className="container">
      <h1>Server Selected</h1>
      <p>Server address: {serverAddress}</p>
      <p>Login is not implemented yet.</p>
    </main>
  );
}

export default App;
