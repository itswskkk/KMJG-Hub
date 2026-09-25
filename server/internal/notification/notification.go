// Package notification implements KMJG Hub's persistent, per-user
// notification list, per docs/PRD.md "Notifications". Notifications are
// in-app only: KMJG Hub v1 has no built-in email or push delivery. Clicking
// a notification must re-verify the underlying resource still exists and
// that the user is still authorized to see it — the notification payload
// itself is never treated as an authorization grant.
package notification

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("notification: not found")
)

// EventType identifies what kind of collaboration event a notification is
// about. Routine Project Chat messages deliberately have no EventType —
// docs/PRD.md "Project Chat messages do not require individual
// notifications for every message by default."
type EventType string

const (
	EventTaskAssigned        EventType = "task_assigned"
	EventTaskComment         EventType = "task_comment"
	EventDirectMessage       EventType = "direct_message"
	EventFileTransferRequest EventType = "file_transfer_request"
	EventProjectInvitation   EventType = "project_invitation"
	EventRoleChanged         EventType = "role_changed"
	EventGitPush             EventType = "git_push"
	EventFriendRequest       EventType = "friend_request"
)

// Notification is one persisted event for one recipient.
type Notification struct {
	ID        string
	UserID    string
	EventType EventType
	Payload   json.RawMessage // event-specific context: project_id, task_id, sender_id, etc.
	CreatedAt time.Time
	ReadAt    *time.Time
}

type Cursor struct {
	CreatedAt time.Time
	ID        string
}

type Page struct {
	Notifications []Notification
	NextCursor    string
	UnreadCount   int
}

// Repository persists notifications.
type Repository interface {
	// Create persists a new notification for userID.
	Create(ctx context.Context, userID string, eventType EventType, payload any) (*Notification, error)

	// ListPage returns a page of userID's notifications, newest first, plus
	// their current total unread count.
	ListPage(ctx context.Context, userID string, before *Cursor, limit int) ([]Notification, int, error)

	// MarkRead marks one notification read. Only the owning userID may mark
	// it; otherwise ErrNotFound (never reveal another user's notification
	// exists).
	MarkRead(ctx context.Context, notificationID, userID string) error

	// Delete removes (dismisses) one notification. Owner-only, same as MarkRead.
	Delete(ctx context.Context, notificationID, userID string) error
}

// Publisher delivers real-time notification events. The recipient is always
// exactly the notified userID.
type Publisher interface {
	PublishNotificationCreated(n Notification)
}
