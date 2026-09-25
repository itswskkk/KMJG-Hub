// Package task implements persistent Project-scoped work items.
package task

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound also covers a Project to which the caller has no access.
	ErrNotFound  = errors.New("task: not found")
	ErrForbidden = errors.New("task: forbidden")
)

type Status string

const (
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

type Task struct {
	ID, ProjectID, Title, Description string
	Status                            Status
	CreatorID, CreatorUsername        string
	AssigneeID, AssigneeUsername      *string
	DueDate                           *time.Time
	CreatedAt, UpdatedAt              time.Time
}

type Comment struct {
	ID, TaskID, AuthorID, AuthorUsername, Body string
	CreatedAt                                  time.Time
}

type AssignmentRequest struct {
	ID, TaskID, ProjectID, RequesterID, RecipientID string
	TaskTitle, RequesterUsername                    string
	Status                                          string
	CreatedAt                                       time.Time
}

type Repository interface {
	List(ctx context.Context, projectID, viewerID string) ([]Task, error)
	Get(ctx context.Context, projectID, taskID, viewerID string) (*Task, error)
	Create(ctx context.Context, projectID, creatorID, title, description string, dueDate *time.Time) (*Task, error)
	SetStatus(ctx context.Context, projectID, taskID, actorID string, status Status) (*Task, error)
	AssignSelf(ctx context.Context, projectID, taskID, userID string) (*Task, error)
	RequestAssignment(ctx context.Context, projectID, taskID, requesterID, recipientID string) (*AssignmentRequest, error)
	ListPendingAssignments(ctx context.Context, recipientID string) ([]AssignmentRequest, error)
	RespondAssignment(ctx context.Context, requestID, recipientID string, accept bool) (*Task, error)
	SetCurrent(ctx context.Context, projectID, taskID, userID string) (*Task, error)
	ListComments(ctx context.Context, projectID, taskID, viewerID string) ([]Comment, error)
	AddComment(ctx context.Context, projectID, taskID, authorID, body string) (*Comment, error)
}

// Membership supplies the authoritative audience for a Project event. A
// caller never chooses WebSocket recipients itself.
type Membership interface {
	MemberUserIDs(ctx context.Context, projectID string) ([]string, error)
}

// Publisher is the narrow real-time capability Tasks needs. The event only
// identifies a changed resource; Clients reload its Server-authorized state.
type Publisher interface {
	PublishTaskChanged(userID, projectID, taskID string)
}

type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return e.Message }
