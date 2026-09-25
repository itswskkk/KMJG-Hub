import { FormEvent, KeyboardEvent, useEffect, useRef, useState } from "react";
import {
  ApiError,
  ProjectChatMessage,
  ProjectDetail,
  deleteProjectMessage,
  isSessionExpired,
  listProjectMessages,
  sendProjectMessage,
} from "../../lib/apiClient";
import { useProjectChatEvents } from "../presence/PresenceProvider";
import "./ProjectChat.css";

const MAX_MESSAGE_CHARACTERS = 4000;

interface ProjectChatProps {
  detail: ProjectDetail;
  serverUrl: string;
  token: string;
  viewerUserId: string;
  onSessionExpired: () => void;
}

function sortMessages(messages: ProjectChatMessage[]): ProjectChatMessage[] {
  return [...messages].sort((a, b) => a.created_at.localeCompare(b.created_at) || a.id.localeCompare(b.id));
}

function mergeMessage(messages: ProjectChatMessage[], incoming: ProjectChatMessage): ProjectChatMessage[] {
  const withoutDuplicate = messages.filter((message) => message.id !== incoming.id);
  return sortMessages([...withoutDuplicate, incoming]);
}

function ProjectChat({ detail, serverUrl, token, viewerUserId, onSessionExpired }: ProjectChatProps) {
  const [messages, setMessages] = useState<ProjectChatMessage[]>([]);
  const [draft, setDraft] = useState("");
  const [loading, setLoading] = useState(true);
  const [sending, setSending] = useState(false);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const timelineRef = useRef<HTMLDivElement>(null);
  const realtimeEvents = useProjectChatEvents(detail.id);
  const draftCharacterCount = Array.from(draft).length;

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    listProjectMessages(serverUrl, token, detail.id)
      .then((history) => {
        if (!cancelled) {
          setMessages((current) => sortMessages([...history, ...current.filter((message) => !history.some((item) => item.id === message.id))]));
        }
      })
      .catch((err) => {
        if (cancelled) return;
        if (isSessionExpired(err)) {
          onSessionExpired();
          return;
        }
        setError(err instanceof ApiError ? err.message : "Could not load Project Chat.");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [detail.id, onSessionExpired, serverUrl, token]);

  useEffect(() => {
    for (const event of realtimeEvents) {
      if (event.type === "created") {
        setMessages((current) => mergeMessage(current, event.data));
      } else {
        setMessages((current) => current.filter((message) => message.id !== event.data.message_id));
      }
    }
  }, [realtimeEvents]);

  useEffect(() => {
    timelineRef.current?.scrollTo({ top: timelineRef.current.scrollHeight, behavior: "smooth" });
  }, [messages]);

  const handleError = (err: unknown, fallback: string) => {
    if (isSessionExpired(err)) {
      onSessionExpired();
    } else {
      setError(err instanceof ApiError ? err.message : fallback);
    }
  };

  const handleSubmit = async (event?: FormEvent) => {
    event?.preventDefault();
    const body = draft.trim();
    if (!body || sending || draftCharacterCount > MAX_MESSAGE_CHARACTERS) return;
    setSending(true);
    setError(null);
    try {
      const message = await sendProjectMessage(serverUrl, token, detail.id, body);
      setMessages((current) => mergeMessage(current, message));
      setDraft("");
    } catch (err) {
      handleError(err, "Could not send the message.");
    } finally {
      setSending(false);
    }
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void handleSubmit();
    }
  };

  const handleDelete = async (message: ProjectChatMessage) => {
    const own = message.author_id === viewerUserId;
    const prompt = own
      ? "Delete this message?\n\nIt will be removed from normal access and retained by the Server for 30 days."
      : `Delete ${message.author_username}'s message?\n\nIt will be removed from Project Chat for all members.`;
    if (!window.confirm(prompt)) return;
    setDeletingId(message.id);
    setError(null);
    try {
      await deleteProjectMessage(serverUrl, token, detail.id, message.id);
      setMessages((current) => current.filter((item) => item.id !== message.id));
    } catch (err) {
      handleError(err, "Could not delete the message.");
    } finally {
      setDeletingId(null);
    }
  };

  const canModerate = detail.role === "owner" || detail.role === "admin";

  return (
    <section className="project-chat">
      <header>
        <h1>Project Chat</h1>
        <p>Shared with all current members of {detail.name}.</p>
      </header>

      {error && <p className="project-chat__error" role="alert">{error}</p>}

      <div className="project-chat__timeline" ref={timelineRef} aria-live="polite">
        {loading && <p className="project-chat__state">Loading messages...</p>}
        {!loading && messages.length === 0 && <p className="project-chat__state">No messages yet. Start the conversation.</p>}
        {messages.map((message) => {
          const mayDelete = message.author_id === viewerUserId || canModerate;
          return (
            <article className="project-chat__message" key={message.id}>
              <div className="project-chat__message-header">
                <strong>{message.author_username}</strong>
                <time dateTime={message.created_at}>{new Date(message.created_at).toLocaleString()}</time>
                {mayDelete && (
                  <button type="button" className="project-chat__delete" disabled={deletingId === message.id} onClick={() => void handleDelete(message)}>
                    {deletingId === message.id ? "Deleting..." : "Delete"}
                  </button>
                )}
              </div>
              <p>{message.body}</p>
            </article>
          );
        })}
      </div>

      <form className="project-chat__composer" onSubmit={(event) => void handleSubmit(event)}>
        <textarea
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Message Project..."
          aria-label="Message Project"
          rows={2}
        />
        <div className="project-chat__composer-actions">
          <span className={draftCharacterCount > MAX_MESSAGE_CHARACTERS ? "project-chat__count--invalid" : undefined}>
            {draftCharacterCount.toLocaleString()} / {MAX_MESSAGE_CHARACTERS.toLocaleString()}
          </span>
          <button type="submit" disabled={sending || draft.trim().length === 0 || draftCharacterCount > MAX_MESSAGE_CHARACTERS}>
            {sending ? "Sending..." : "Send"}
          </button>
        </div>
      </form>
      <p className="project-chat__attachment-note">File attachments are not available yet.</p>
    </section>
  );
}

export default ProjectChat;
