package httpapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/task"
)

// fakeTaskRepository is deliberately small in behavior but complete at the
// interface boundary, so these tests exercise the HTTP contract through the
// real task.Service rather than duplicating validation in a handler fake.
type fakeTaskRepository struct{}

func (fakeTaskRepository) List(context.Context, string, string) ([]task.Task, error) { return nil, nil }
func (fakeTaskRepository) Get(_ context.Context, projectID, taskID, _ string) (*task.Task, error) {
	return &task.Task{ID: taskID, ProjectID: projectID}, nil
}
func (fakeTaskRepository) Create(_ context.Context, projectID, creatorID, title, description string, _ *time.Time) (*task.Task, error) {
	return &task.Task{ID: "task-1", ProjectID: projectID, CreatorID: creatorID, Title: title, Description: description, Status: task.StatusTodo}, nil
}
func (fakeTaskRepository) SetStatus(_ context.Context, projectID, taskID, _ string, status task.Status) (*task.Task, error) {
	return &task.Task{ID: taskID, ProjectID: projectID, Status: status}, nil
}
func (fakeTaskRepository) AssignSelf(_ context.Context, projectID, taskID, _ string) (*task.Task, error) {
	return &task.Task{ID: taskID, ProjectID: projectID}, nil
}
func (fakeTaskRepository) RequestAssignment(_ context.Context, projectID, taskID, requesterID, recipientID string) (*task.AssignmentRequest, error) {
	return &task.AssignmentRequest{ID: "request-1", ProjectID: projectID, TaskID: taskID, RequesterID: requesterID, RecipientID: recipientID, Status: "pending"}, nil
}
func (fakeTaskRepository) ListPendingAssignments(context.Context, string) ([]task.AssignmentRequest, error) {
	return nil, nil
}
func (fakeTaskRepository) RespondAssignment(_ context.Context, _, recipientID string, _ bool) (*task.Task, error) {
	return &task.Task{ID: "task-1", ProjectID: "project-1", AssigneeID: &recipientID}, nil
}
func (fakeTaskRepository) SetCurrent(_ context.Context, projectID, taskID, _ string) (*task.Task, error) {
	return &task.Task{ID: taskID, ProjectID: projectID}, nil
}
func (fakeTaskRepository) ListComments(context.Context, string, string, string) ([]task.Comment, error) {
	return nil, nil
}
func (fakeTaskRepository) AddComment(_ context.Context, _, taskID, authorID, body string) (*task.Comment, error) {
	return &task.Comment{ID: "comment-1", TaskID: taskID, AuthorID: authorID, Body: body}, nil
}

func TestTaskEndpointsAuthenticateAndExposeValidatedTaskDTOs(t *testing.T) {
	router, handlers, _ := newTestRouterWithHandlers()
	handlers.Tasks = &task.Service{Repo: fakeTaskRepository{}}
	token := registerAndToken(t, router, "taskhttp")

	if rec := doJSON(t, router, http.MethodPost, "/api/v1/projects/project-1/tasks", map[string]string{"title": "Task"}, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated create = %d, want 401", rec.Code)
	}
	if rec := doJSON(t, router, http.MethodPost, "/api/v1/projects/project-1/tasks", map[string]string{"title": "   "}, token); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty title = %d, want 400: %s", rec.Code, rec.Body.String())
	}

	created := doJSON(t, router, http.MethodPost, "/api/v1/projects/project-1/tasks", map[string]string{"title": "  Task title  ", "description": "  detail  "}, token)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	var body struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Status string `json:"status"`
	}
	decodeJSON(t, created, &body)
	if body.ID != "task-1" || body.Title != "Task title" || body.Status != "todo" {
		t.Fatalf("unexpected task DTO: %+v", body)
	}

	if rec := doJSON(t, router, http.MethodPatch, "/api/v1/projects/project-1/tasks/task-1/status", map[string]string{"status": "blocked"}, token); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}
