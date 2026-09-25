package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itswskkk/KMJG-Hub/server/internal/notification"
)

// NotificationRepository persists per-user notifications. Every read and
// write is scoped to the owning user_id in SQL, so another user's
// notification is indistinguishable from a missing one (ErrNotFound).
type NotificationRepository struct {
	pool *pgxpool.Pool
}

func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{pool: pool}
}

func (r *NotificationRepository) Create(ctx context.Context, userID string, eventType notification.EventType, payload any) (*notification.Notification, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var n notification.Notification
	var stored []byte
	err = r.pool.QueryRow(ctx, `
		INSERT INTO notifications (user_id, event_type, payload)
		VALUES ($1, $2, $3::jsonb)
		RETURNING id, user_id, event_type, payload, created_at, read_at
	`, userID, string(eventType), string(raw)).Scan(
		&n.ID, &n.UserID, &n.EventType, &stored, &n.CreatedAt, &n.ReadAt,
	)
	if err != nil {
		if isInvalidUUID(err) {
			return nil, notification.ErrNotFound
		}
		return nil, err
	}
	n.Payload = json.RawMessage(stored)
	return &n, nil
}

func (r *NotificationRepository) ListPage(ctx context.Context, userID string, before *notification.Cursor, limit int) ([]notification.Notification, int, error) {
	var beforeTime, beforeID any
	if before != nil {
		beforeTime = before.CreatedAt
		beforeID = before.ID
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, event_type, payload, created_at, read_at
		FROM notifications
		WHERE user_id = $1
		  AND ($3::timestamptz IS NULL OR (created_at, id) < ($3, $4::uuid))
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, userID, limit, beforeTime, beforeID)
	if err != nil {
		if isInvalidUUID(err) {
			return nil, 0, notification.ErrNotFound
		}
		return nil, 0, err
	}
	defer rows.Close()

	notifications := make([]notification.Notification, 0)
	for rows.Next() {
		var n notification.Notification
		var stored []byte
		if err := rows.Scan(&n.ID, &n.UserID, &n.EventType, &stored, &n.CreatedAt, &n.ReadAt); err != nil {
			return nil, 0, err
		}
		n.Payload = json.RawMessage(stored)
		notifications = append(notifications, n)
	}
	if err := rows.Err(); err != nil {
		if isInvalidUUID(err) {
			return nil, 0, notification.ErrNotFound
		}
		return nil, 0, err
	}

	var unread int
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL
	`, userID).Scan(&unread); err != nil {
		return nil, 0, err
	}
	return notifications, unread, nil
}

// MarkRead marks the notification read. Marking an already-read notification
// is an idempotent success; a missing notification or one owned by another
// user is ErrNotFound.
func (r *NotificationRepository) MarkRead(ctx context.Context, notificationID, userID string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE notifications SET read_at = now()
		WHERE id = $1 AND user_id = $2 AND read_at IS NULL
	`, notificationID, userID)
	if err != nil {
		if isInvalidUUID(err) {
			return notification.ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	// Zero rows: either already read (owned by userID), or not visible.
	var exists bool
	err = r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM notifications WHERE id = $1 AND user_id = $2)
	`, notificationID, userID).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
			return notification.ErrNotFound
		}
		return err
	}
	if !exists {
		return notification.ErrNotFound
	}
	return nil
}

func (r *NotificationRepository) Delete(ctx context.Context, notificationID, userID string) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM notifications WHERE id = $1 AND user_id = $2
	`, notificationID, userID)
	if err != nil {
		if isInvalidUUID(err) {
			return notification.ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return notification.ErrNotFound
	}
	return nil
}
