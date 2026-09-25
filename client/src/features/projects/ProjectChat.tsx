import { FormEvent, KeyboardEvent, useEffect, useRef, useState } from "react";
import {
  ApiError,
  ProjectChatMessage,
  ProjectDetail,
  deleteProjectMessage,
	downloadProjectAttachment,
  isSessionExpired,
  listProjectMessages,
  sendProjectMessage,
	uploadProjectAttachment,
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
	const [nextCursor,setNextCursor]=useState("");
	const [loadingOlder,setLoadingOlder]=useState(false);
	const [attachment,setAttachment]=useState<File|null>(null);
  const timelineRef = useRef<HTMLDivElement>(null);
  const realtimeEvents = useProjectChatEvents(detail.id);
  const draftCharacterCount = Array.from(draft).length;

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    listProjectMessages(serverUrl, token, detail.id)
      .then((page) => {
        if (!cancelled) {
		  setMessages((current) => sortMessages([...page.messages, ...current.filter((message) => !page.messages.some((item) => item.id === message.id))]));
		  setNextCursor(page.next_cursor);
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
	if ((!body && !attachment) || sending || draftCharacterCount > MAX_MESSAGE_CHARACTERS) return;
    setSending(true);
    setError(null);
    try {
	  const message = attachment
		? await uploadProjectAttachment(serverUrl,token,detail.id,attachment,body)
		: await sendProjectMessage(serverUrl, token, detail.id, body);
      setMessages((current) => mergeMessage(current, message));
      setDraft("");
	  setAttachment(null);
    } catch (err) {
      handleError(err, "Could not send the message.");
    } finally {
      setSending(false);
    }
  };

	const loadOlder=async()=>{if(!nextCursor||loadingOlder)return;setLoadingOlder(true);setError(null);try{const page=await listProjectMessages(serverUrl,token,detail.id,nextCursor);setMessages((current)=>sortMessages([...page.messages,...current]));setNextCursor(page.next_cursor)}catch(err){handleError(err,"Could not load older messages.")}finally{setLoadingOlder(false)}};

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
		{nextCursor && <button type="button" className="project-chat__older" disabled={loadingOlder} onClick={()=>void loadOlder()}>{loadingOlder?"Loading…":"Load older messages"}</button>}
        {loading && <p className="project-chat__state">Loading messages...</p>}
        {!loading && messages.length === 0 && <p className="project-chat__state">No messages yet. Start the conversation.</p>}
        {messages.map((message) => {
          // Git activity and other Server-generated entries are not
          // user-authored messages: no author and no delete action
          // (docs/UX.md "System and Git Activity").
          if (message.kind && message.kind !== "user") {
            return (
              <article className="project-chat__message project-chat__message--system" key={message.id}>
                <div className="project-chat__message-header">
                  <strong>{message.kind === "git" ? "Git Activity" : "Activity"}</strong>
                  <time dateTime={message.created_at}>{new Date(message.created_at).toLocaleString()}</time>
                </div>
                <p>{message.body}</p>
              </article>
            );
          }
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
			  {message.body && <p>{message.body}</p>}
			  {(message.attachments??[]).map((item)=><button type="button" className="project-chat__attachment" key={item.id} onClick={()=>void downloadProjectAttachment(serverUrl,token,detail.id,item).catch((err)=>handleError(err,"Could not download the attachment."))}>📎 {item.filename} ({formatBytes(item.size_bytes)})</button>)}
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
		  <label className="project-chat__file">Attach file<input type="file" onChange={(event)=>setAttachment(event.target.files?.[0]??null)} /></label>
		  {attachment && <span title={attachment.name}>{attachment.name}</span>}
          <span className={draftCharacterCount > MAX_MESSAGE_CHARACTERS ? "project-chat__count--invalid" : undefined}>
            {draftCharacterCount.toLocaleString()} / {MAX_MESSAGE_CHARACTERS.toLocaleString()}
          </span>
		  <button type="submit" disabled={sending || (!draft.trim()&&!attachment) || draftCharacterCount > MAX_MESSAGE_CHARACTERS}>
            {sending ? "Sending..." : "Send"}
          </button>
        </div>
      </form>
    </section>
  );
}

function formatBytes(bytes:number):string{if(bytes<1024)return `${bytes} B`;if(bytes<1024*1024)return `${(bytes/1024).toFixed(1)} KB`;return `${(bytes/1024/1024).toFixed(1)} MB`}

export default ProjectChat;
