import { FormEvent, KeyboardEvent, useEffect, useRef, useState } from "react";
import {
  ApiError,
  Conversation,
  DirectMessage,
  Friend,
  deleteDM,
  isSessionExpired,
  listConversations,
  listDMMessages,
  listFriends,
  sendDM,
} from "../../lib/apiClient";
import { useDMEvents } from "../presence/PresenceProvider";
import "./ServerHome.css";
import "../projects/ProjectChat.css";
import "./DirectMessages.css";

const MAX_MESSAGE_CHARACTERS = 4000;

interface DirectMessagesProps {
  serverUrl: string;
  token: string;
  viewerUserId: string;
  onBack: () => void;
  onSessionExpired: () => void;
}

interface Partner {
  userId: string;
  username: string;
}

function sortMessages(messages: DirectMessage[]): DirectMessage[] {
  return [...messages].sort((a, b) => a.created_at.localeCompare(b.created_at) || a.id.localeCompare(b.id));
}

function mergeMessages(current: DirectMessage[], incoming: DirectMessage[]): DirectMessage[] {
  const ids = new Set(incoming.map((m) => m.id));
  return sortMessages([...current.filter((m) => !ids.has(m.id)), ...incoming]);
}

/** Server Home's Direct Messages section: conversation list plus one open thread. */
function DirectMessages({ serverUrl, token, viewerUserId, onBack, onSessionExpired }: DirectMessagesProps) {
  const [conversations, setConversations] = useState<Conversation[] | null>(null);
  const [friends, setFriends] = useState<Friend[]>([]);
  const [listError, setListError] = useState<string | null>(null);
  const [partner, setPartner] = useState<Partner | null>(null);
  const [conversationsKey, setConversationsKey] = useState(0);
  const dmEvents = useDMEvents();
  const latestEvent = dmEvents[dmEvents.length - 1];

  useEffect(() => {
    let cancelled = false;
    Promise.all([listConversations(serverUrl, token), listFriends(serverUrl, token)])
      .then(([convs, friendList]) => {
        if (cancelled) return;
        setConversations(convs);
        setFriends(friendList);
      })
      .catch((err) => {
        if (cancelled) return;
        if (isSessionExpired(err)) { onSessionExpired(); return; }
        setListError(err instanceof ApiError ? err.message : "Could not load your conversations.");
      });
    return () => { cancelled = true; };
    // Real-time DM events are reload hints for the conversation list.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverUrl, token, conversationsKey, latestEvent]);

  const startableFriends = friends.filter((f) => !(conversations ?? []).some((c) => c.other_user_id === f.user_id));

  return (
    <main className="server-home">
      <div className="server-home__header">
        <div>
          <h1>Direct Messages</h1>
          <p className="server-home__subtitle">Private conversations with friends and Project teammates.</p>
        </div>
        <div className="server-home__header-actions">
          <button type="button" onClick={onBack}>Back to Server Home</button>
        </div>
      </div>

      <div className="direct-messages">
        <aside className="direct-messages__sidebar">
          <h2>Conversations</h2>
          {listError && <p className="server-home__error" role="alert">{listError}</p>}
          {conversations === null && !listError && <p className="server-home__loading">Loading…</p>}
          {conversations !== null && conversations.length === 0 && <p className="server-home__loading">No conversations yet.</p>}
          <ul className="direct-messages__list">
            {(conversations ?? []).map((c) => (
              <li key={c.other_user_id}>
                <button
                  type="button"
                  className={`direct-messages__conversation${partner?.userId === c.other_user_id ? " direct-messages__conversation--active" : ""}`}
                  onClick={() => setPartner({ userId: c.other_user_id, username: c.other_username })}
                >
                  <strong>{c.other_username}</strong>
                  <span>{c.last_message_from_me ? "You: " : ""}{c.last_message_body}</span>
                </button>
              </li>
            ))}
          </ul>
          {startableFriends.length > 0 && (
            <label className="direct-messages__start">
              New message
              <select
                value=""
                onChange={(event) => {
                  const friend = friends.find((f) => f.user_id === event.target.value);
                  if (friend) setPartner({ userId: friend.user_id, username: friend.username });
                }}
              >
                <option value="">Choose a friend…</option>
                {startableFriends.map((f) => <option key={f.user_id} value={f.user_id}>{f.username}</option>)}
              </select>
            </label>
          )}
        </aside>

        <section className="direct-messages__thread-pane">
          {partner ? (
            <DirectMessageThread
              key={partner.userId}
              serverUrl={serverUrl}
              token={token}
              viewerUserId={viewerUserId}
              partner={partner}
              onSent={() => setConversationsKey((k) => k + 1)}
              onSessionExpired={onSessionExpired}
            />
          ) : (
            <p className="direct-messages__placeholder">Select a conversation to start messaging.</p>
          )}
        </section>
      </div>
    </main>
  );
}

interface ThreadProps {
  serverUrl: string;
  token: string;
  viewerUserId: string;
  partner: Partner;
  onSent: () => void;
  onSessionExpired: () => void;
}

function DirectMessageThread({ serverUrl, token, viewerUserId, partner, onSent, onSessionExpired }: ThreadProps) {
  const [messages, setMessages] = useState<DirectMessage[]>([]);
  const [nextCursor, setNextCursor] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const timelineRef = useRef<HTMLDivElement>(null);
  const dmEvents = useDMEvents();
  const draftCharacterCount = Array.from(draft).length;

  const handleError = (err: unknown, fallback: string) => {
    if (isSessionExpired(err)) {
      onSessionExpired();
    } else {
      setError(err instanceof ApiError ? err.message : fallback);
    }
  };

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    listDMMessages(serverUrl, token, partner.userId)
      .then((page) => {
        if (cancelled) return;
        setMessages((current) => mergeMessages(current, page.messages));
        setNextCursor(page.next_cursor);
      })
      .catch((err) => {
        if (cancelled) return;
        if (isSessionExpired(err)) { onSessionExpired(); return; }
        setError(err instanceof ApiError ? err.message : "Could not load this conversation.");
      })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverUrl, token, partner.userId]);

  useEffect(() => {
    for (const event of dmEvents) {
      if (event.type === "created") {
        const m = event.data;
        const inThread =
          (m.sender_id === partner.userId && m.recipient_id === viewerUserId) ||
          (m.sender_id === viewerUserId && m.recipient_id === partner.userId);
        if (inThread) setMessages((current) => mergeMessages(current, [m]));
      } else {
        setMessages((current) => current.filter((m) => m.id !== event.data.message_id));
      }
    }
  }, [dmEvents, partner.userId, viewerUserId]);

  useEffect(() => {
    timelineRef.current?.scrollTo({ top: timelineRef.current.scrollHeight, behavior: "smooth" });
  }, [messages.length]);

  const loadOlder = async () => {
    if (!nextCursor || loadingOlder) return;
    setLoadingOlder(true);
    setError(null);
    try {
      const page = await listDMMessages(serverUrl, token, partner.userId, nextCursor);
      setMessages((current) => mergeMessages(current, page.messages));
      setNextCursor(page.next_cursor);
    } catch (err) {
      handleError(err, "Could not load older messages.");
    } finally {
      setLoadingOlder(false);
    }
  };

  const handleSubmit = async (event?: FormEvent) => {
    event?.preventDefault();
    const body = draft.trim();
    if (!body || sending || draftCharacterCount > MAX_MESSAGE_CHARACTERS) return;
    setSending(true);
    setError(null);
    try {
      const message = await sendDM(serverUrl, token, partner.userId, body);
      setMessages((current) => mergeMessages(current, [message]));
      setDraft("");
      onSent();
    } catch (err) {
      if (err instanceof ApiError && err.status === 403) {
        setError(`You can't message ${partner.username} right now. Direct Messages need a friendship or a shared Project, and neither of you can have blocked the other.`);
      } else {
        handleError(err, "Could not send the message.");
      }
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

  const handleDelete = async (message: DirectMessage) => {
    if (!window.confirm("Delete this message?\n\nIt will be removed from the conversation and retained by the Server for 30 days.")) return;
    setDeletingId(message.id);
    setError(null);
    try {
      await deleteDM(serverUrl, token, message.id);
      setMessages((current) => current.filter((m) => m.id !== message.id));
      onSent();
    } catch (err) {
      handleError(err, "Could not delete the message.");
    } finally {
      setDeletingId(null);
    }
  };

  return (
    <div className="project-chat direct-messages__thread">
      <header>
        <h2>{partner.username}</h2>
      </header>

      {error && <p className="project-chat__error" role="alert">{error}</p>}

      <div className="project-chat__timeline" ref={timelineRef} aria-live="polite">
        {nextCursor && (
          <button type="button" className="project-chat__older" disabled={loadingOlder} onClick={() => void loadOlder()}>
            {loadingOlder ? "Loading…" : "Load older messages"}
          </button>
        )}
        {loading && <p className="project-chat__state">Loading messages...</p>}
        {!loading && messages.length === 0 && <p className="project-chat__state">No messages yet. Say hello.</p>}
        {messages.map((message) => (
          <article className="project-chat__message" key={message.id}>
            <div className="project-chat__message-header">
              <strong>{message.sender_id === viewerUserId ? "You" : message.sender_username}</strong>
              <time dateTime={message.created_at}>{new Date(message.created_at).toLocaleString()}</time>
              {message.sender_id === viewerUserId && (
                <button type="button" className="project-chat__delete" disabled={deletingId === message.id} onClick={() => void handleDelete(message)}>
                  {deletingId === message.id ? "Deleting..." : "Delete"}
                </button>
              )}
            </div>
            <p>{message.body}</p>
          </article>
        ))}
      </div>

      <form className="project-chat__composer" onSubmit={(event) => void handleSubmit(event)}>
        <textarea
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={`Message ${partner.username}...`}
          aria-label={`Message ${partner.username}`}
          rows={2}
        />
        <div className="project-chat__composer-actions">
          <span className={draftCharacterCount > MAX_MESSAGE_CHARACTERS ? "project-chat__count--invalid" : undefined}>
            {draftCharacterCount.toLocaleString()} / {MAX_MESSAGE_CHARACTERS.toLocaleString()}
          </span>
          <button type="submit" disabled={sending || !draft.trim() || draftCharacterCount > MAX_MESSAGE_CHARACTERS}>
            {sending ? "Sending..." : "Send"}
          </button>
        </div>
      </form>
    </div>
  );
}

export default DirectMessages;
