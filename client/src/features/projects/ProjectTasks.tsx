import { FormEvent, useEffect, useState } from "react";
import {
  ApiError,
  createProjectTask,
  isSessionExpired,
  listProjectTasks,
  ProjectDetail,
  ProjectTask,
  setProjectTaskStatus,
  TaskStatus,
} from "../../lib/apiClient";
import "./ProjectTasks.css";

interface Props {
  detail: ProjectDetail;
  serverUrl: string;
  token: string;
  onSessionExpired: () => void;
}

const columns: { status: TaskStatus; label: string }[] = [
  { status: "todo", label: "To Do" },
  { status: "in_progress", label: "In Progress" },
  { status: "done", label: "Done" },
];

function ProjectTasks({ detail, serverUrl, token, onSessionExpired }: Props) {
  const [tasks, setTasks] = useState<ProjectTask[]>([]);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

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

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setCreating(true); setError(null);
    try {
      const task = await createProjectTask(serverUrl, token, detail.id, { title, description });
      setTasks((items) => [...items, task]); setTitle(""); setDescription("");
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

  return <section className="project-tasks">
    <header className="project-tasks__header"><div><h2>Tasks</h2><p>Organize project work on a simple Kanban board.</p></div></header>
    <form className="project-tasks__new" onSubmit={submit}>
      <input value={title} onChange={(e) => setTitle(e.target.value)} maxLength={200} required placeholder="New task title" aria-label="New task title" />
      <input value={description} onChange={(e) => setDescription(e.target.value)} maxLength={10000} placeholder="Description (optional)" aria-label="Task description" />
      <button type="submit" disabled={creating}>{creating ? "Creating…" : "+ New Task"}</button>
    </form>
    {error && <p className="project-tasks__error" role="alert">{error}</p>}
    <div className="project-tasks__board">
      {columns.map((column) => <section className="task-column" key={column.status}><h3>{column.label}</h3>
        {tasks.filter((task) => task.status === column.status).map((task) => <article className="task-card" key={task.id}>
          <strong>{task.title}</strong>{task.description && <p>{task.description}</p>}
          <small>{task.assignee_username ? `Assigned to ${task.assignee_username}` : "Unassigned"}</small>
          <label>Move to <select value={task.status} onChange={(e) => void move(task, e.target.value as TaskStatus)}>{columns.map((option) => <option key={option.status} value={option.status}>{option.label}</option>)}</select></label>
        </article>)}
        {tasks.every((task) => task.status !== column.status) && <p className="task-column__empty">No tasks</p>}
      </section>)}
    </div>
  </section>;
}
export default ProjectTasks;
