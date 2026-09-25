package notification

import (
	"context"
	"encoding/base64"
	"encoding/json"
)

// Service implements notification creation and retrieval.
type Service struct {
	Repo      Repository
	Publisher Publisher
}

// RecentLimit bounds a single notification list page. Same value as chat's
// RecentMessageLimit for consistency across the product.
const RecentLimit = 50

// Create persists a notification and publishes it in real time to userID.
// Called by other domain services (task, chat, directmessage, friend,
// invitation) when a notification-worthy event occurs — never by an HTTP
// handler directly.
func (s *Service) Create(ctx context.Context, userID string, eventType EventType, payload any) (*Notification, error) {
	n, err := s.Repo.Create(ctx, userID, eventType, payload)
	if err != nil {
		return nil, err
	}
	if s.Publisher != nil {
		s.Publisher.PublishNotificationCreated(*n)
	}
	return n, nil
}

// Notify is Create with a plain string event type and no return value
// besides the error. It lets other domain packages (task, directmessage,
// friend, invitation) depend on their own narrow Notifier interface
// without importing this package's types.
func (s *Service) Notify(ctx context.Context, userID, eventType string, payload any) error {
	_, err := s.Create(ctx, userID, EventType(eventType), payload)
	return err
}

// ListPage returns userID's notifications, newest first.
func (s *Service) ListPage(ctx context.Context, userID, encodedCursor string) (*Page, error) {
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

	notifications, unreadCount, err := s.Repo.ListPage(ctx, userID, before, RecentLimit+1)
	if err != nil {
		return nil, err
	}

	page := &Page{Notifications: notifications, UnreadCount: unreadCount}
	if len(notifications) > RecentLimit {
		page.Notifications = notifications[:RecentLimit]
		last := page.Notifications[len(page.Notifications)-1]
		cursor := Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
		raw, _ := json.Marshal(cursor)
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return page, nil
}

// MarkRead marks one notification read for userID.
func (s *Service) MarkRead(ctx context.Context, notificationID, userID string) error {
	return s.Repo.MarkRead(ctx, notificationID, userID)
}

// Delete dismisses one notification for userID.
func (s *Service) Delete(ctx context.Context, notificationID, userID string) error {
	return s.Repo.Delete(ctx, notificationID, userID)
}

// ValidationError is safe to expose to API clients.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }
