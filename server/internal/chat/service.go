package chat

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxMessageCharacters bounds persistent and real-time message payloads.
// ASSUMPTION A-260925-4: product maximum is not specified; see ASSUMPTIONS.md.
const MaxMessageCharacters = 4000

// RecentMessageLimit bounds initial history retrieval.
// ASSUMPTION A-260925-5: product page size is not specified; see ASSUMPTIONS.md.
const RecentMessageLimit = 50

// DeletedMessageRetention is required by docs/PRD.md "Deleted Messages".
const DeletedMessageRetention = 30 * 24 * time.Hour

type Service struct {
	Repo       Repository
	Membership Membership
	Publisher  Publisher
}

func (s *Service) ListRecent(ctx context.Context, userID, projectID string) ([]Message, error) {
	return s.Repo.ListRecent(ctx, projectID, userID, RecentMessageLimit)
}

func (s *Service) Send(ctx context.Context, userID, projectID, body string) (*Message, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, &ValidationError{Field: "body", Message: "Message cannot be empty"}
	}
	if !utf8.ValidString(body) || utf8.RuneCountInString(body) > MaxMessageCharacters {
		return nil, &ValidationError{Field: "body", Message: "Message must be 4,000 characters or fewer"}
	}

	message, err := s.Repo.Create(ctx, projectID, userID, body)
	if err != nil {
		return nil, err
	}
	s.publishCreated(ctx, *message)
	return message, nil
}

func (s *Service) Delete(ctx context.Context, userID, projectID, messageID string) error {
	message, err := s.Repo.SoftDelete(ctx, projectID, messageID, userID)
	if err != nil {
		return err
	}
	s.publishDeleted(ctx, message.ProjectID, message.ID)
	return nil
}

// PurgeExpiredDeleted permanently removes messages whose soft-deletion
// retention window has ended. It is an operator lifecycle action and never
// exposes deleted content to normal Project members.
func (s *Service) PurgeExpiredDeleted(ctx context.Context, now time.Time) error {
	return s.Repo.PurgeDeletedBefore(ctx, now.Add(-DeletedMessageRetention))
}

func (s *Service) publishCreated(ctx context.Context, message Message) {
	if s.Membership == nil || s.Publisher == nil {
		return
	}
	userIDs, err := s.Membership.MemberUserIDs(ctx, message.ProjectID)
	if err != nil {
		return
	}
	for _, userID := range userIDs {
		s.Publisher.PublishMessageCreated(userID, message)
	}
}

func (s *Service) publishDeleted(ctx context.Context, projectID, messageID string) {
	if s.Membership == nil || s.Publisher == nil {
		return
	}
	userIDs, err := s.Membership.MemberUserIDs(ctx, projectID)
	if err != nil {
		return
	}
	for _, userID := range userIDs {
		s.Publisher.PublishMessageDeleted(userID, projectID, messageID)
	}
}
