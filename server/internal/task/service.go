package task

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxTitleCharacters       = 200
	maxDescriptionCharacters = 10000
	maxCommentCharacters     = 4000
)

type Service struct {
	Repo       Repository
	Membership Membership
	Publisher  Publisher
	// Notifier is optional; nil disables notification creation. Creation is
	// best-effort and never fails the primary task operation.
	Notifier Notifier
}

// Notifier creates persistent notifications. Satisfied by
// *notification.Service; declared here so task does not import it.
type Notifier interface {
	Notify(ctx context.Context, userID, eventType string, payload any) error
}

const (
	notificationTaskAssigned = "task_assigned"
	notificationTaskComment  = "task_comment"
	commentPreviewCharacters = 100
)

func (s *Service) notify(ctx context.Context, userID, eventType string, payload any) {
	if s.Notifier == nil {
		return
	}
	if err := s.Notifier.Notify(ctx, userID, eventType, payload); err != nil {
		slog.Warn("task: create notification", "event_type", eventType, "error", err)
	}
}

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
	t, err := s.Repo.Create(ctx, projectID, userID, title, description, dueDate)
	if err == nil {
		s.publishChanged(ctx, projectID, t.ID)
	}
	return t, err
}

func (s *Service) SetStatus(ctx context.Context, userID, projectID, taskID string, status Status) (*Task, error) {
	if status != StatusTodo && status != StatusInProgress && status != StatusDone {
		return nil, &ValidationError{"status", "Status must be todo, in_progress, or done"}
	}
	t, err := s.Repo.SetStatus(ctx, projectID, taskID, userID, status)
	if err == nil {
		s.publishChanged(ctx, projectID, t.ID)
	}
	return t, err
}
func (s *Service) Assign(ctx context.Context, userID, projectID, taskID, assigneeID string) (*Task, *AssignmentRequest, error) {
	if strings.TrimSpace(assigneeID) == "" {
		return nil, nil, &ValidationError{"assignee_id", "Assignee is required"}
	}
	if userID == assigneeID {
		t, e := s.Repo.AssignSelf(ctx, projectID, taskID, userID)
		if e == nil {
			s.publishChanged(ctx, projectID, t.ID)
		}
		return t, nil, e
	}
	r, e := s.Repo.RequestAssignment(ctx, projectID, taskID, userID, assigneeID)
	if e == nil {
		s.publishChanged(ctx, projectID, taskID)
		s.notifyAssignment(ctx, userID, projectID, taskID, assigneeID, r)
	}
	return nil, r, e
}

func (s *Service) notifyAssignment(ctx context.Context, userID, projectID, taskID, assigneeID string, r *AssignmentRequest) {
	if r == nil {
		return
	}
	s.notify(ctx, assigneeID, notificationTaskAssigned, map[string]any{
		"project_id":            projectID,
		"task_id":               taskID,
		"task_title":            r.TaskTitle,
		"assigned_by_user_id":   userID,
		"assigned_by_username":  r.RequesterUsername,
		"assignment_request_id": r.ID,
	})
}
func (s *Service) RespondAssignment(ctx context.Context, userID, requestID string, accept bool) (*Task, error) {
	t, err := s.Repo.RespondAssignment(ctx, requestID, userID, accept)
	if err == nil {
		s.publishChanged(ctx, t.ProjectID, t.ID)
	}
	return t, err
}

func (s *Service) ListPendingAssignments(ctx context.Context, userID string) ([]AssignmentRequest, error) {
	return s.Repo.ListPendingAssignments(ctx, userID)
}
func (s *Service) SetCurrent(ctx context.Context, userID, projectID, taskID string) (*Task, error) {
	t, err := s.Repo.SetCurrent(ctx, projectID, taskID, userID)
	if err == nil {
		s.publishChanged(ctx, projectID, t.ID)
	}
	return t, err
}
func (s *Service) ListComments(ctx context.Context, userID, projectID, taskID string) ([]Comment, error) {
	return s.Repo.ListComments(ctx, projectID, taskID, userID)
}
func (s *Service) AddComment(ctx context.Context, userID, projectID, taskID, body string) (*Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" || !utf8.ValidString(body) || utf8.RuneCountInString(body) > maxCommentCharacters {
		return nil, &ValidationError{"body", "Comment must be between 1 and 4,000 characters"}
	}
	c, err := s.Repo.AddComment(ctx, projectID, taskID, userID, body)
	if err == nil {
		s.publishChanged(ctx, projectID, taskID)
		s.notifyComment(ctx, userID, projectID, taskID, c)
	}
	return c, err
}

// notifyComment notifies the Task's creator and assignee (the users a Task
// is "relevant" to, per docs/PRD.md "Notifications"), never the comment
// author themselves.
func (s *Service) notifyComment(ctx context.Context, authorID, projectID, taskID string, c *Comment) {
	if s.Notifier == nil || c == nil {
		return
	}
	t, err := s.Repo.Get(ctx, projectID, taskID, authorID)
	if err != nil {
		slog.Warn("task: load task for comment notification", "error", err)
		return
	}
	recipients := []string{t.CreatorID}
	if t.AssigneeID != nil {
		recipients = append(recipients, *t.AssigneeID)
	}
	seen := map[string]bool{authorID: true}
	for _, recipient := range recipients {
		if recipient == "" || seen[recipient] {
			continue
		}
		seen[recipient] = true
		s.notify(ctx, recipient, notificationTaskComment, map[string]any{
			"project_id":      projectID,
			"task_id":         taskID,
			"task_title":      t.Title,
			"comment_id":      c.ID,
			"author_id":       authorID,
			"author_username": c.AuthorUsername,
			"body_preview":    preview(c.Body, commentPreviewCharacters),
		})
	}
}

// preview truncates body to at most n runes.
func preview(body string, n int) string {
	runes := []rune(body)
	if len(runes) <= n {
		return body
	}
	return string(runes[:n])
}

func (s *Service) publishChanged(ctx context.Context, projectID, taskID string) {
	if s.Membership == nil || s.Publisher == nil {
		return
	}
	userIDs, err := s.Membership.MemberUserIDs(ctx, projectID)
	if err != nil {
		return
	}
	for _, userID := range userIDs {
		s.Publisher.PublishTaskChanged(userID, projectID, taskID)
	}
}
