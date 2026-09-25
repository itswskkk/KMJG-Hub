import { FormEvent, useEffect, useState } from "react";
import {
  ApiError,
  addTaskComment,
  assignProjectTask,
  createProjectTask,
  isSessionExpired,
  listTaskComments,
  listTaskAssignmentRequests,
  listProjectTasks,
  ProjectDetail,
  ProjectTask,
  TaskComment,
  TaskAssignmentRequest,
  respondTaskAssignment,
  setProjectTaskStatus,
  setCurrentProjectTask,
  TaskStatus,
} from "../../lib/apiClient";
import "./ProjectTasks.css";
import { useProjectTaskEvents } from "../presence/PresenceProvider";

interface Props {
  detail: ProjectDetail;
  serverUrl: string;
  token: string;
  viewerUserId: string;
  onSessionExpired: () => void;
}

const columns: { status: TaskStatus; label: string }[] = [
  { status: "todo", label: "To Do" },
  { status: "in_progress", label: "In Progress" },
  { status: "done", label: "Done" },
];

function ProjectTasks({ detail, serverUrl, token, viewerUserId, onSessionExpired }: Props) {
  const [tasks, setTasks] = useState<ProjectTask[]>([]);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [dueDate, setDueDate] = useState("");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<ProjectTask | null>(null);
  const [comments, setComments] = useState<TaskComment[]>([]);
  const [comment, setComment] = useState("");
  const [savingDetail, setSavingDetail] = useState(false);
  const [requests, setRequests] = useState<TaskAssignmentRequest[]>([]);
  const taskEvents = useProjectTaskEvents(detail.id);

  useEffect(() => {
    let cancelled = false;
    setError(null);
    listProjectTasks(serverUrl, token, detail.id)
      .then((items) => { if (!cancelled) setTasks(items); })
      .catch((err) => {
        if (cancelled) return;
        if (isSessionExpired(err)) { onSessionExpired(); return; }
        setError(err instanceof ApiError ? err.message : "Could not load tasks.");
      });
    return () => { cancelled = true; };
  }, [detail.id, serverUrl, token, onSessionExpired]);

  useEffect(() => { let cancelled = false; listTaskAssignmentRequests(serverUrl, token).then((items) => { if (!cancelled) setRequests(items.filter((item) => item.project_id === detail.id)); }).catch(() => {}); return () => { cancelled = true; }; }, [detail.id, serverUrl, token]);

  useEffect(() => {
    if (taskEvents.length === 0) return;
    let cancelled = false;
    Promise.all([listProjectTasks(serverUrl, token, detail.id), listTaskAssignmentRequests(serverUrl, token)])
      .then(([items, pending]) => {
        if (cancelled) return;
        setTasks(items);
        setRequests(pending.filter((item) => item.project_id === detail.id));
        setSelected((current) => current ? items.find((item) => item.id === current.id) ?? null : null);
      })
      .catch((err) => { if (!cancelled && isSessionExpired(err)) onSessionExpired(); });
    return () => { cancelled = true; };
  }, [taskEvents.length, detail.id, serverUrl, token, onSessionExpired]);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setCreating(true); setError(null);
    try {
      const task = await createProjectTask(serverUrl, token, detail.id, {
        title,
        description,
        due_date: dueDate ? new Date(`${dueDate}T00:00:00.000Z`).toISOString() : null,
      });
      setTasks((items) => [...items, task]); setTitle(""); setDescription(""); setDueDate("");
    } catch (err) {
      if (isSessionExpired(err)) onSessionExpired();
      else setError(err instanceof ApiError ? err.message : "Could not create task.");
    } finally { setCreating(false); }
  };

  const move = async (task: ProjectTask, status: TaskStatus) => {
    if (task.status === status) return;
    setError(null);
    try {
      const updated = await setProjectTaskStatus(serverUrl, token, detail.id, task.id, status);
      setTasks((items) => items.map((item) => item.id === updated.id ? updated : item));
    } catch (err) {
      if (isSessionExpired(err)) onSessionExpired();
      else setError(err instanceof ApiError ? err.message : "Could not update task.");
    }
  };

  const replaceTask = (updated: ProjectTask) => {
    setTasks((items) => items.map((item) => item.id === updated.id ? updated : item));
    setSelected(updated);
  };

  const selectTask = (task: ProjectTask) => {
    setSelected(task); setComments([]); setError(null);
    listTaskComments(serverUrl, token, detail.id, task.id)
      .then(setComments)
      .catch((err) => {
        if (isSessionExpired(err)) onSessionExpired();
        else setError(err instanceof ApiError ? err.message : "Could not load task comments.");
      });
  };

  const assignSelf = async () => {
    if (!selected) return;
    setSavingDetail(true); setError(null);
    try { const result = await assignProjectTask(serverUrl, token, detail.id, selected.id, viewerUserId); if ("assignment_request_id" in result) throw new Error("Unexpected assignment request"); replaceTask(result); }
    catch (err) { if (isSessionExpired(err)) onSessionExpired(); else setError(err instanceof ApiError ? err.message : "Could not assign this task."); }
    finally { setSavingDetail(false); }
  };
  const assignMember = async (assigneeId: string) => { if (!selected || !assigneeId) return; setSavingDetail(true); try { const result = await assignProjectTask(serverUrl, token, detail.id, selected.id, assigneeId); if ("assignment_request_id" in result) setError("Assignment request sent."); else replaceTask(result); } catch (err) { setError(err instanceof ApiError ? err.message : "Assignment request could not be sent."); } finally { setSavingDetail(false); } };

  const setCurrent = async () => {
    if (!selected) return;
    setSavingDetail(true); setError(null);
    try { replaceTask(await setCurrentProjectTask(serverUrl, token, detail.id, selected.id)); }
    catch (err) { if (isSessionExpired(err)) onSessionExpired(); else setError(err instanceof ApiError ? err.message : "Could not set the current task."); }
    finally { setSavingDetail(false); }
  };

  const submitComment = async (event: FormEvent) => {
    event.preventDefault(); if (!selected || !comment.trim()) return;
    setSavingDetail(true); setError(null);
    try { const added = await addTaskComment(serverUrl, token, detail.id, selected.id, comment); setComments((items) => [...items, added]); setComment(""); }
    catch (err) { if (isSessionExpired(err)) onSessionExpired(); else setError(err instanceof ApiError ? err.message : "Could not add comment."); }
    finally { setSavingDetail(false); }
  };
  const respondRequest = async (request: TaskAssignmentRequest, accept: boolean) => { setSavingDetail(true); try { const updated = await respondTaskAssignment(serverUrl, token, request.id, accept); setRequests((items) => items.filter((item) => item.id !== request.id)); setTasks((items) => items.map((item) => item.id === updated.id ? updated : item)); } catch (err) { setError(err instanceof ApiError ? err.message : "Could not respond to assignment."); } finally { setSavingDetail(false); } };

  return <section className="project-tasks">
    <header className="project-tasks__header"><div><h2>Tasks</h2><p>Organize project work on a simple Kanban board.</p></div></header>
    <form className="project-tasks__new" onSubmit={submit}>
      <input value={title} onChange={(e) => setTitle(e.target.value)} maxLength={200} required placeholder="New task title" aria-label="New task title" />
      <input value={description} onChange={(e) => setDescription(e.target.value)} maxLength={10000} placeholder="Description (optional)" aria-label="Task description" />
      <label className="project-tasks__due-date">Due date (optional)<input type="date" value={dueDate} onChange={(e) => setDueDate(e.target.value)} /></label>
      <button type="submit" disabled={creating}>{creating ? "Creating…" : "+ New Task"}</button>
    </form>
    {error && <p className="project-tasks__error" role="alert">{error}</p>}
    {requests.length > 0 && <section className="task-requests"><h3>Assignment requests</h3>{requests.map((request) => <article key={request.id}><span><strong>{request.requester_username}</strong> assigned you <strong>{request.task_title}</strong></span><button disabled={savingDetail} onClick={() => void respondRequest(request, false)}>Decline</button><button disabled={savingDetail} onClick={() => void respondRequest(request, true)}>Accept</button></article>)}</section>}
    <div className="project-tasks__layout">
    <div className="project-tasks__board">
      {columns.map((column) => <section className="task-column" key={column.status}><h3>{column.label}</h3>
        {tasks.filter((task) => task.status === column.status).map((task) => <article className="task-card" key={task.id} onClick={() => selectTask(task)}>
          <strong>{task.title}</strong>{task.description && <p>{task.description}</p>}
          <small>{task.assignee_username ? `Assigned to ${task.assignee_username}` : "Unassigned"}</small>
          <label>Move to <select value={task.status} onChange={(e) => void move(task, e.target.value as TaskStatus)}>{columns.map((option) => <option key={option.status} value={option.status}>{option.label}</option>)}</select></label>
        </article>)}
        {tasks.every((task) => task.status !== column.status) && <p className="task-column__empty">No tasks</p>}
      </section>)}
    </div>
    {selected && <aside className="task-detail" aria-label="Task details">
      <button type="button" className="task-detail__close" onClick={() => setSelected(null)}>× Close</button>
      <h3>{selected.title}</h3>
      {selected.description && <p className="task-detail__description">{selected.description}</p>}
      <p><strong>Status:</strong> {columns.find((column) => column.status === selected.status)?.label}</p>
      <p><strong>Creator:</strong> {selected.creator_username}</p>
      <p><strong>Assignee:</strong> {selected.assignee_username ?? "Unassigned"}</p>
      {selected.due_date && <p><strong>Due:</strong> {new Date(selected.due_date).toLocaleDateString()}</p>}
      <div className="task-detail__actions">
        {!selected.assignee_id && <button type="button" disabled={savingDetail} onClick={() => void assignSelf()}>Assign to me</button>}
        {!selected.assignee_id && <label>Assign member<select defaultValue="" disabled={savingDetail} onChange={(e) => void assignMember(e.target.value)}><option value="" disabled>Select a member</option>{detail.members.filter((member) => member.id !== viewerUserId).map((member) => <option key={member.id} value={member.id}>{member.username}</option>)}</select></label>}
        {selected.assignee_id === viewerUserId && <button type="button" disabled={savingDetail} onClick={() => void setCurrent()}>Set as Current Task</button>}
      </div>
      <h4>Comments</h4>
      <div className="task-detail__comments">{comments.map((item) => <article key={item.id}><strong>{item.author_username}</strong><p>{item.body}</p></article>)}{comments.length === 0 && <p className="task-column__empty">No comments yet.</p>}</div>
      <form className="task-detail__comment-form" onSubmit={submitComment}><textarea value={comment} onChange={(e) => setComment(e.target.value)} maxLength={4000} placeholder="Write a comment" required /><button type="submit" disabled={savingDetail}>{savingDetail ? "Saving…" : "Comment"}</button></form>
    </aside>}
    </div>
  </section>;
}
export default ProjectTasks;
