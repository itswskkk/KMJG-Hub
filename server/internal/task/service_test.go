package task_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/task"
)

type fakeRepo struct {
	createdTitle string
	createErr    error
	task         *task.Task // returned by Get when set
}

func (f *fakeRepo) List(context.Context, string, string) ([]task.Task, error) { return nil, nil }
func (f *fakeRepo) Get(context.Context, string, string, string) (*task.Task, error) {
	if f.task != nil {
		return f.task, nil
	}
	return nil, task.ErrNotFound
}
func (f *fakeRepo) Create(_ context.Context, projectID, creatorID, title, description string, _ *time.Time) (*task.Task, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.createdTitle = title
	return &task.Task{ID: "task-1", ProjectID: projectID, CreatorID: creatorID, Title: title, Description: description}, nil
}
func (f *fakeRepo) SetStatus(_ context.Context, projectID, taskID, _ string, status task.Status) (*task.Task, error) {
	return &task.Task{ID: taskID, ProjectID: projectID, Status: status}, nil
}
func (f *fakeRepo) AssignSelf(_ context.Context, projectID, taskID, _ string) (*task.Task, error) {
	return &task.Task{ID: taskID, ProjectID: projectID}, nil
}
func (f *fakeRepo) RequestAssignment(_ context.Context, projectID, taskID, requesterID, recipientID string) (*task.AssignmentRequest, error) {
	return &task.AssignmentRequest{ID: "request-1", ProjectID: projectID, TaskID: taskID, RequesterID: requesterID, RecipientID: recipientID, Status: "pending"}, nil
}
func (f *fakeRepo) ListPendingAssignments(context.Context, string) ([]task.AssignmentRequest, error) {
	return nil, nil
}
func (f *fakeRepo) RespondAssignment(_ context.Context, _, recipientID string, _ bool) (*task.Task, error) {
	return &task.Task{ID: "task-1", ProjectID: "project-1", AssigneeID: &recipientID}, nil
}
func (f *fakeRepo) SetCurrent(_ context.Context, projectID, taskID, _ string) (*task.Task, error) {
	return &task.Task{ID: taskID, ProjectID: projectID}, nil
}
func (f *fakeRepo) ListComments(context.Context, string, string, string) ([]task.Comment, error) {
	return nil, nil
}
func (f *fakeRepo) AddComment(_ context.Context, _, taskID, authorID, body string) (*task.Comment, error) {
	return &task.Comment{ID: "comment-1", TaskID: taskID, AuthorID: authorID, Body: body}, nil
}

type fakeMembership struct{ ids []string }

func (f fakeMembership) MemberUserIDs(context.Context, string) ([]string, error) { return f.ids, nil }

type fakePublisher struct{ changed []string }

func (f *fakePublisher) PublishTaskChanged(userID, projectID, taskID string) {
	f.changed = append(f.changed, userID+":"+projectID+":"+taskID)
}

func TestCreateTrimsPersistsThenNotifiesCurrentMembers(t *testing.T) {
	repo := &fakeRepo{}
	publisher := &fakePublisher{}
	svc := &task.Service{Repo: repo, Membership: fakeMembership{ids: []string{"u1", "u2"}}, Publisher: publisher}

	created, err := svc.Create(context.Background(), "u1", "project-1", "  Design board  ", "  details  ", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Title != "Design board" || repo.createdTitle != "Design board" {
		t.Fatalf("title was not trimmed: %+v", created)
	}
	if got, want := strings.Join(publisher.changed, ","), "u1:project-1:task-1,u2:project-1:task-1"; got != want {
		t.Fatalf("notifications = %q, want %q", got, want)
	}
}

func TestFailedMutationNeverNotifies(t *testing.T) {
	publisher := &fakePublisher{}
	svc := &task.Service{Repo: &fakeRepo{createErr: task.ErrNotFound}, Membership: fakeMembership{ids: []string{"u1"}}, Publisher: publisher}
	if _, err := svc.Create(context.Background(), "u1", "project-1", "Task", "", nil); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("Create error = %v", err)
	}
	if len(publisher.changed) != 0 {
		t.Fatalf("published failed mutation: %v", publisher.changed)
	}
}

func TestAssignmentRequestAndResponseNotifyProjectMembers(t *testing.T) {
	publisher := &fakePublisher{}
	svc := &task.Service{Repo: &fakeRepo{}, Membership: fakeMembership{ids: []string{"u1", "u2"}}, Publisher: publisher}
	if _, request, err := svc.Assign(context.Background(), "u1", "project-1", "task-1", "u2"); err != nil || request == nil {
		t.Fatalf("Assign request = %v, %v", request, err)
	}
	if _, err := svc.RespondAssignment(context.Background(), "u2", "request-1", true); err != nil {
		t.Fatalf("RespondAssignment: %v", err)
	}
	if got, want := len(publisher.changed), 4; got != want {
		t.Fatalf("notification count = %d, want %d", got, want)
	}
}
