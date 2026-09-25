// Package directmessage implements Server-level Direct Messages between two
// users, per docs/PRD.md "Friends and Direct Messages" § Project Member
// Messaging. Two users may exchange Direct Messages only while at least one
// permitted relationship exists between them: an accepted friendship, or
// shared membership in at least one Project. Access is re-checked on every
// send (not only when a conversation starts), so it is correctly revoked if
// both users leave their only shared Project and are not friends.
package directmessage

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrForbidden covers both "not a permitted relationship" and "blocked".
	ErrForbidden = errors.New("directmessage: forbidden")
	ErrNotFound  = errors.New("directmessage: not found")
)

// MaxMessageCharacters mirrors internal/chat.MaxMessageCharacters — no
// product-specified different limit for Direct Messages.
const MaxMessageCharacters = 4000

// RecentMessageLimit bounds initial history retrieval, matching Project Chat.
const RecentMessageLimit = 50

// DeletedMessageRetention is required by docs/PRD.md "Deleted Messages" for
// all user-generated message deletion, including Direct Messages.
const DeletedMessageRetention = 30 * 24 * time.Hour

// Message is one active Direct Message between two users. Soft-deleted
// content is never returned through this normal Client-facing model.
type Message struct {
	ID                string
	SenderID          string
	SenderUsername    string
	RecipientID       string
	RecipientUsername string
	Body              string
	CreatedAt         time.Time
}

// Conversation summarizes one DM thread for the conversations list.
type Conversation struct {
	OtherUserID       string
	OtherUsername     string
	LastMessageBody   string
	LastMessageAt     time.Time
	LastMessageFromMe bool
}

type Cursor struct {
	CreatedAt time.Time
	ID        string
}

type Page struct {
	Messages   []Message
	NextCursor string
}

// Repository stores authoritative Direct Message history. Access
// authorization (permitted relationship, not blocked) must be enforced
// inside the same statement that creates or reads messages — never only in
// the service layer — per docs/ARCHITECTURE.md "every protected Server
// operation must verify the authenticated user's current permissions".
type Repository interface {
	// Create persists a message from senderID to recipientID. Implementations
	// must verify, as part of the same statement/transaction, that the two
	// users share a permitted relationship and that neither has blocked the
	// other; otherwise return ErrForbidden. ErrNotFound if recipientID does
	// not exist.
	Create(ctx context.Context, senderID, recipientID, body string) (*Message, error)

	// ListPage returns a page of the conversation between viewerID and
	// otherUserID, newest first. Requires viewerID to be one of the two
	// participants; unauthorized access (or attempting to read a
	// conversation with a nonexistent user) is ErrNotFound.
	ListPage(ctx context.Context, viewerID, otherUserID string, before *Cursor, limit int) ([]Message, error)

	// ListConversations returns viewerID's active conversations (one row per
	// distinct other participant with at least one non-deleted message),
	// ordered by most recent message first.
	ListConversations(ctx context.Context, viewerID string) ([]Conversation, error)

	// SoftDelete marks a message deleted. Only the original sender may
	// delete; anyone else gets ErrForbidden (per docs/PRD.md "Message
	// Deletion Permissions": "A user may not delete a Direct Message
	// originally sent by the other participant" and project roles grant no
	// DM deletion permission).
	SoftDelete(ctx context.Context, messageID, actorID string) (*Message, error)

	// PurgeDeletedBefore permanently removes messages soft-deleted before
	// cutoff, per the 30-day retention window.
	PurgeDeletedBefore(ctx context.Context, cutoff time.Time) error
}

// Publisher is the narrow real-time capability Direct Messages needs.
// Recipients are always exactly the two conversation participants, derived
// from the message itself — never a Client-supplied audience.
type Publisher interface {
	PublishMessageCreated(message Message)
	PublishMessageDeleted(senderID, recipientID, messageID string)
}

// ValidationError is safe to expose to API clients.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }
