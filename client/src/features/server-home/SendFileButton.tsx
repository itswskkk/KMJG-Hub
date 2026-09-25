import { ChangeEvent, useRef, useState } from "react";
import { ApiError, createFileTransferRequest, isSessionExpired } from "../../lib/apiClient";
import { rememberTransferFile } from "../../lib/pendingTransferFiles";

interface SendFileButtonProps {
  serverUrl: string;
  token: string;
  recipientId: string;
  recipientUsername: string;
  onSent: (message: string) => void;
  onError: (message: string) => void;
  onSessionExpired: () => void;
}

/** Starts a Direct File Transfer: picks a local file and sends only its
 * name and size as a request. The bytes stay on this device until the
 * recipient accepts (docs/PRD.md "Direct File Transfer"). */
function SendFileButton({ serverUrl, token, recipientId, recipientUsername, onSent, onError, onSessionExpired }: SendFileButtonProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [sending, setSending] = useState(false);

  async function handleChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    setSending(true);
    try {
      const transfer = await createFileTransferRequest(serverUrl, token, recipientId, file);
      rememberTransferFile(transfer.id, file);
      onSent(`File transfer request for “${file.name}” sent to ${recipientUsername}. You can upload it once they accept.`);
    } catch (err) {
      if (isSessionExpired(err)) { onSessionExpired(); return; }
      onError(err instanceof ApiError ? err.message : "Could not send the file transfer request.");
    } finally {
      setSending(false);
    }
  }

  return (
    <>
      <input ref={inputRef} type="file" hidden onChange={(event) => void handleChange(event)} aria-label={`Choose a file to send to ${recipientUsername}`} />
      <button type="button" disabled={sending} onClick={() => inputRef.current?.click()}>
        {sending ? "Sending…" : "Send File"}
      </button>
    </>
  );
}

export default SendFileButton;
