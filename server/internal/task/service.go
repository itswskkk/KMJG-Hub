package task

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxTitleCharacters       = 200
	maxDescriptionCharacters = 10000
	maxCommentCharacters     = 4000
)

type Service struct{ Repo Repository }

func (s *Service) List(ctx context.Context, userID, projectID string) ([]Task, error) {
	return s.Repo.List(ctx, projectID, userID)
}
func (s *Service) Get(ctx context.Context, userID, projectID, taskID string) (*Task, error) {
	return s.Repo.Get(ctx, projectID, taskID, userID)
}

func (s *Service) Create(ctx context.Context, userID, projectID, title, description string, dueDate *time.Time) (*Task, error) {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	if title == "" || !utf8.ValidString(title) || utf8.RuneCountInString(title) > maxTitleCharacters {
		return nil, &ValidationError{"title", "Title must be between 1 and 200 characters"}
	}
	if !utf8.ValidString(description) || utf8.RuneCountInString(description) > maxDescriptionCharacters {
		return nil, &ValidationError{"description", "Description must be 10,000 characters or fewer"}
	}
	return s.Repo.Create(ctx, projectID, userID, title, description, dueDate)
}

func (s *Service) SetStatus(ctx context.Context, userID, projectID, taskID string, status Status) (*Task, error) {
	if status != StatusTodo && status != StatusInProgress && status != StatusDone {
		return nil, &ValidationError{"status", "Status must be todo, in_progress, or done"}
	}
	return s.Repo.SetStatus(ctx, projectID, taskID, userID, status)
}
func (s *Service) Assign(ctx context.Context, userID, projectID, taskID, assigneeID string) (*Task, *AssignmentRequest, error) {
	if strings.TrimSpace(assigneeID) == "" {
		return nil, nil, &ValidationError{"assignee_id", "Assignee is required"}
	}
	if userID == assigneeID {
		t, e := s.Repo.AssignSelf(ctx, projectID, taskID, userID)
		return t, nil, e
	}
	r, e := s.Repo.RequestAssignment(ctx, projectID, taskID, userID, assigneeID)
	return nil, r, e
}
func (s *Service) RespondAssignment(ctx context.Context, userID, requestID string, accept bool) (*Task, error) {
	return s.Repo.RespondAssignment(ctx, requestID, userID, accept)
}
func (s *Service) SetCurrent(ctx context.Context, userID, projectID, taskID string) (*Task, error) {
	return s.Repo.SetCurrent(ctx, projectID, taskID, userID)
}
func (s *Service) ListComments(ctx context.Context, userID, projectID, taskID string) ([]Comment, error) {
	return s.Repo.ListComments(ctx, projectID, taskID, userID)
}
func (s *Service) AddComment(ctx context.Context, userID, projectID, taskID, body string) (*Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" || !utf8.ValidString(body) || utf8.RuneCountInString(body) > maxCommentCharacters {
		return nil, &ValidationError{"body", "Comment must be between 1 and 4,000 characters"}
	}
	return s.Repo.AddComment(ctx, projectID, taskID, userID, body)
}
