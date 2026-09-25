package workcontext

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

var ErrNotFound = errors.New("work context: not found")

type Context struct {
	ProjectID     string `json:"project_id"`
	UserID        string `json:"user_id"`
	Working       bool   `json:"working"`
	StatusMode    string `json:"status_mode"`
	CurrentBranch string `json:"current_branch,omitempty"`
}

type Repository interface {
	Upsert(ctx context.Context, projectID, userID string, working bool, mode, branch string) (*Context, error)
	List(ctx context.Context, projectID, viewerID string) ([]Context, error)
}

type OnlineUsers interface{ IsOnline(userID string) bool }
type Membership interface {
	MemberUserIDs(ctx context.Context, projectID string) ([]string, error)
}
type Publisher interface {
	PublishWorkContext(userID string, value Context)
}

type Service struct {
	Repo       Repository
	Online     OnlineUsers
	Membership Membership
	Publisher  Publisher
}

func (s *Service) List(ctx context.Context, viewerID, projectID string) ([]Context, error) {
	values, err := s.Repo.List(ctx, projectID, viewerID)
	if err != nil {
		return nil, err
	}
	visible := values[:0]
	for _, value := range values {
		if value.UserID == viewerID || (s.Online != nil && s.Online.IsOnline(value.UserID)) {
			visible = append(visible, value)
		}
	}
	return visible, nil
}

func (s *Service) Update(ctx context.Context, userID, projectID string, working bool, mode, branch string) (*Context, error) {
	mode = strings.TrimSpace(mode)
	if mode != "automatic" && mode != "manual" {
		return nil, &ValidationError{Field: "status_mode", Message: "Status mode must be automatic or manual"}
	}
	branch = strings.TrimSpace(branch)
	if !utf8.ValidString(branch) || utf8.RuneCountInString(branch) > 255 {
		return nil, &ValidationError{Field: "current_branch", Message: "Current branch must be 255 characters or fewer"}
	}
	value, err := s.Repo.Upsert(ctx, projectID, userID, working, mode, branch)
	if err != nil {
		return nil, err
	}
	if s.Membership != nil && s.Publisher != nil {
		if ids, listErr := s.Membership.MemberUserIDs(ctx, projectID); listErr == nil {
			for _, id := range ids {
				s.Publisher.PublishWorkContext(id, *value)
			}
		}
	}
	return value, nil
}

type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return e.Message }
