// Package chat implements the persistent, Project-scoped conversation
// described by docs/PRD.md "Project Communication Model".
package chat

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrNotFound deliberately covers both a missing resource and a caller
	// who is not a Project member, so private Project existence is not leaked.
	ErrNotFound  = errors.New("chat: not found")
	ErrForbidden = errors.New("chat: forbidden")
)

// Message is one active user-authored Project Chat message. Soft-deleted
// content is never returned through this normal Client-facing model.
type Message struct {
	ID             string
	ProjectID      string
	AuthorID       string
	AuthorUsername string
	Body           string
	CreatedAt      time.Time
}

// Repository stores authoritative Project Chat history.
type Repository interface {
	Create(ctx context.Context, projectID, authorID, body string) (*Message, error)
	ListRecent(ctx context.Context, projectID, viewerID string, limit int) ([]Message, error)
	SoftDelete(ctx context.Context, projectID, messageID, actorID string) (*Message, error)
	PurgeDeletedBefore(ctx context.Context, cutoff time.Time) error
}

// Membership supplies the current broadcast audience from Server-owned
// Project membership rather than any recipient list supplied by a Client.
type Membership interface {
	MemberUserIDs(ctx context.Context, projectID string) ([]string, error)
}

// Publisher is the narrow real-time capability Chat needs.
type Publisher interface {
	PublishMessageCreated(userID string, message Message)
	PublishMessageDeleted(userID, projectID, messageID string)
}

// ValidationError is safe to expose to API clients.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }
