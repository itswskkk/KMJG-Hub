package httpapi

import (
	"errors"
	"github.com/itswskkk/KMJG-Hub/server/internal/task"
	"net/http"
	"time"
)

type taskDTO struct {
	ID               string      `json:"id"`
	ProjectID        string      `json:"project_id"`
	Title            string      `json:"title"`
	Description      string      `json:"description"`
	Status           task.Status `json:"status"`
	CreatorID        string      `json:"creator_id"`
	CreatorUsername  string      `json:"creator_username"`
	AssigneeID       *string     `json:"assignee_id"`
	AssigneeUsername *string     `json:"assignee_username"`
	DueDate          *time.Time  `json:"due_date"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
}

func toTaskDTO(t task.Task) taskDTO {
	return taskDTO{t.ID, t.ProjectID, t.Title, t.Description, t.Status, t.CreatorID, t.CreatorUsername, t.AssigneeID, t.AssigneeUsername, t.DueDate, t.CreatedAt, t.UpdatedAt}
}

type commentDTO struct {
	ID             string    `json:"id"`
	TaskID         string    `json:"task_id"`
	AuthorID       string    `json:"author_id"`
	AuthorUsername string    `json:"author_username"`
	Body           string    `json:"body"`
	CreatedAt      time.Time `json:"created_at"`
}

func toCommentDTO(c task.Comment) commentDTO {
	return commentDTO{c.ID, c.TaskID, c.AuthorID, c.AuthorUsername, c.Body, c.CreatedAt}
}
func (h *Handlers) handleListTasks(w http.ResponseWriter, r *http.Request) {
	ts, e := h.Tasks.List(r.Context(), currentAuth(r).User.ID, r.PathValue("id"))
	if e != nil {
		writeTaskError(w, e)
		return
	}
	out := make([]taskDTO, len(ts))
	for i, t := range ts {
		out[i] = toTaskDTO(t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": out})
}

type createTaskRequest struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DueDate     *time.Time `json:"due_date"`
}

func (h *Handlers) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var q createTaskRequest
	if readJSON(r, &q) != nil {
		writeError(w, 400, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}
	t, e := h.Tasks.Create(r.Context(), currentAuth(r).User.ID, r.PathValue("id"), q.Title, q.Description, q.DueDate)
	if e != nil {
		writeTaskError(w, e)
		return
	}
	writeJSON(w, 201, toTaskDTO(*t))
}
func (h *Handlers) handleGetTask(w http.ResponseWriter, r *http.Request) {
	t, e := h.Tasks.Get(r.Context(), currentAuth(r).User.ID, r.PathValue("id"), r.PathValue("taskID"))
	if e != nil {
		writeTaskError(w, e)
		return
	}
	writeJSON(w, 200, toTaskDTO(*t))
}

type statusRequest struct {
	Status task.Status `json:"status"`
}

func (h *Handlers) handleSetTaskStatus(w http.ResponseWriter, r *http.Request) {
	var q statusRequest
	if readJSON(r, &q) != nil {
		writeError(w, 400, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}
	t, e := h.Tasks.SetStatus(r.Context(), currentAuth(r).User.ID, r.PathValue("id"), r.PathValue("taskID"), q.Status)
	if e != nil {
		writeTaskError(w, e)
		return
	}
	writeJSON(w, 200, toTaskDTO(*t))
}

type assignRequest struct {
	AssigneeID string `json:"assignee_id"`
}

func (h *Handlers) handleAssignTask(w http.ResponseWriter, r *http.Request) {
	var q assignRequest
	if readJSON(r, &q) != nil {
		writeError(w, 400, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}
	t, x, e := h.Tasks.Assign(r.Context(), currentAuth(r).User.ID, r.PathValue("id"), r.PathValue("taskID"), q.AssigneeID)
	if e != nil {
		writeTaskError(w, e)
		return
	}
	if x != nil {
		writeJSON(w, 202, map[string]any{"assignment_request_id": x.ID, "status": x.Status})
		return
	}
	writeJSON(w, 200, toTaskDTO(*t))
}
func (h *Handlers) handleSetCurrentTask(w http.ResponseWriter, r *http.Request) {
	t, e := h.Tasks.SetCurrent(r.Context(), currentAuth(r).User.ID, r.PathValue("id"), r.PathValue("taskID"))
	if e != nil {
		writeTaskError(w, e)
		return
	}
	writeJSON(w, 200, toTaskDTO(*t))
}
func (h *Handlers) handleListTaskComments(w http.ResponseWriter, r *http.Request) {
	cs, e := h.Tasks.ListComments(r.Context(), currentAuth(r).User.ID, r.PathValue("id"), r.PathValue("taskID"))
	if e != nil {
		writeTaskError(w, e)
		return
	}
	out := make([]commentDTO, len(cs))
	for i, c := range cs {
		out[i] = toCommentDTO(c)
	}
	writeJSON(w, 200, map[string]any{"comments": out})
}

type commentRequest struct {
	Body string `json:"body"`
}

func (h *Handlers) handleAddTaskComment(w http.ResponseWriter, r *http.Request) {
	var q commentRequest
	if readJSON(r, &q) != nil {
		writeError(w, 400, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}
	c, e := h.Tasks.AddComment(r.Context(), currentAuth(r).User.ID, r.PathValue("id"), r.PathValue("taskID"), q.Body)
	if e != nil {
		writeTaskError(w, e)
		return
	}
	writeJSON(w, 201, toCommentDTO(*c))
}
func (h *Handlers) respondTaskAssignment(w http.ResponseWriter, r *http.Request, accept bool) {
	t, e := h.Tasks.RespondAssignment(r.Context(), currentAuth(r).User.ID, r.PathValue("requestID"), accept)
	if e != nil {
		writeTaskError(w, e)
		return
	}
	writeJSON(w, 200, toTaskDTO(*t))
}
func (h *Handlers) handleAcceptTaskAssignment(w http.ResponseWriter, r *http.Request) {
	h.respondTaskAssignment(w, r, true)
}
func (h *Handlers) handleDeclineTaskAssignment(w http.ResponseWriter, r *http.Request) {
	h.respondTaskAssignment(w, r, false)
}

type assignmentRequestDTO struct {
	ID                string    `json:"id"`
	TaskID            string    `json:"task_id"`
	ProjectID         string    `json:"project_id"`
	TaskTitle         string    `json:"task_title"`
	RequesterUsername string    `json:"requester_username"`
	CreatedAt         time.Time `json:"created_at"`
}

func (h *Handlers) handleListTaskAssignmentRequests(w http.ResponseWriter, r *http.Request) {
	items, err := h.Tasks.ListPendingAssignments(r.Context(), currentAuth(r).User.ID)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	out := make([]assignmentRequestDTO, len(items))
	for i, item := range items {
		out[i] = assignmentRequestDTO{item.ID, item.TaskID, item.ProjectID, item.TaskTitle, item.RequesterUsername, item.CreatedAt}
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": out})
}
func writeTaskError(w http.ResponseWriter, e error) {
	var v *task.ValidationError
	switch {
	case errors.As(e, &v):
		writeFieldError(w, 400, "validation_error", v.Message, v.Field)
	case errors.Is(e, task.ErrNotFound):
		writeError(w, 404, "not_found", "Project or task not found")
	case errors.Is(e, task.ErrForbidden):
		writeError(w, 403, "forbidden", "You do not have permission for this task")
	default:
		writeError(w, 500, "internal_error", "Something went wrong, please try again")
	}
}
