import { FormEvent, useState } from "react";
import { ApiError, ProjectSummary, createProject } from "../../lib/apiClient";
import "./CreateProject.css";

interface CreateProjectProps {
  serverUrl: string;
  token: string;
  onCreated: (project: ProjectSummary) => void;
  onCancel: () => void;
}

function CreateProject({ serverUrl, token, onCreated, onCancel }: CreateProjectProps) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setSubmitting(true);

    try {
      const project = await createProject(serverUrl, token, { name, description });
      onCreated(project);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Something went wrong, please try again.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="create-project">
      <div className="create-project__card">
        <h1>Create Project</h1>

        <form className="create-project__form" onSubmit={handleSubmit} noValidate>
          <label className="create-project__label" htmlFor="project-name">
            Project Name
          </label>
          <input
            id="project-name"
            type="text"
            value={name}
            onChange={(event) => setName(event.target.value)}
          />

          <label className="create-project__label" htmlFor="project-description">
            Description (Optional)
          </label>
          <textarea
            id="project-description"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            rows={3}
          />

          <fieldset className="create-project__repo">
            <legend>Repository</legend>
            <label className="create-project__radio">
              <input type="radio" name="repo-setup" disabled />
              Create New GitHub Repository (Not Available Yet)
            </label>
            <label className="create-project__radio">
              <input type="radio" name="repo-setup" disabled />
              Connect Existing Repository (Not Available Yet)
            </label>
            <label className="create-project__radio">
              <input type="radio" name="repo-setup" checked readOnly />
              Set Up Later
            </label>
          </fieldset>

          {error && (
            <p className="create-project__error" role="alert">
              {error}
            </p>
          )}

          <div className="create-project__buttons">
            <button type="button" onClick={onCancel} disabled={submitting}>
              Cancel
            </button>
            <button type="submit" disabled={submitting}>
              {submitting ? "Creating..." : "Create Project"}
            </button>
          </div>
        </form>
      </div>
    </main>
  );
}

export default CreateProject;
