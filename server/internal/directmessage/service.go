package directmessage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

// Service implements Direct Message use cases. Access control (permitted
// relationship, blocking, author-only deletion) is enforced by Repository
// inside PostgreSQL; Service handles input validation, pagination cursors,
// and real-time publication.
type Service struct {
	Repo      Repository
	Publisher Publisher
}

func (s *Service) Send(ctx context.Context, senderID, recipientID, body string) (*Message, error) {
	recipientID = strings.TrimSpace(recipientID)
	if recipientID == "" {
		return nil, ErrNotFound
	}
	if recipientID == senderID {
		return nil, &ValidationError{Field: "user_id", Message: "You cannot send a Direct Message to yourself"}
	}
	body, err := validateBody(body)
	if err != nil {
		return nil, err
	}
	message, err := s.Repo.Create(ctx, senderID, recipientID, body)
	if err != nil {
		return nil, err
	}
	if s.Publisher != nil {
		s.Publisher.PublishMessageCreated(*message)
	}
	return message, nil
}

func (s *Service) ListPage(ctx context.Context, viewerID, otherUserID, encodedCursor string) (*Page, error) {
	var before *Cursor
	if encodedCursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(encodedCursor)
		if err != nil {
			return nil, &ValidationError{Field: "cursor", Message: "Cursor is invalid"}
		}
		var value Cursor
		if json.Unmarshal(decoded, &value) != nil || value.ID == "" || value.CreatedAt.IsZero() {
			return nil, &ValidationError{Field: "cursor", Message: "Cursor is invalid"}
		}
		before = &value
	}
	messages, err := s.Repo.ListPage(ctx, viewerID, otherUserID, before, RecentMessageLimit+1)
	if err != nil {
		return nil, err
	}
	page := &Page{Messages: messages}
	// Repository returns oldest-first; an extra leading row means older
	// history exists beyond this page.
	if len(messages) > RecentMessageLimit {
		page.Messages = messages[1:]
		cursor := Cursor{CreatedAt: page.Messages[0].CreatedAt, ID: page.Messages[0].ID}
		raw, _ := json.Marshal(cursor)
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return page, nil
}

func (s *Service) ListConversations(ctx context.Context, viewerID string) ([]Conversation, error) {
	return s.Repo.ListConversations(ctx, viewerID)
}

func (s *Service) Delete(ctx context.Context, messageID, actorID string) error {
	message, err := s.Repo.SoftDelete(ctx, messageID, actorID)
	if err != nil {
		return err
	}
	if s.Publisher != nil {
		s.Publisher.PublishMessageDeleted(message.SenderID, message.RecipientID, message.ID)
	}
	return nil
}

// PurgeExpiredDeleted permanently removes Direct Messages whose 30-day
// soft-deletion retention window has ended.
func (s *Service) PurgeExpiredDeleted(ctx context.Context, now time.Time) error {
	return s.Repo.PurgeDeletedBefore(ctx, now.Add(-DeletedMessageRetention))
}
