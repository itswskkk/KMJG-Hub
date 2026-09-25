package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itswskkk/KMJG-Hub/server/internal/directmessage"
)

// DirectMessageRepository implements directmessage.Repository against
// PostgreSQL. Access authorization is enforced inside the INSERT itself.
type DirectMessageRepository struct {
	pool *pgxpool.Pool
}

func NewDirectMessageRepository(pool *pgxpool.Pool) *DirectMessageRepository {
	return &DirectMessageRepository{pool: pool}
}

var _ directmessage.Repository = (*DirectMessageRepository)(nil)

// Create inserts a message only when, in the same statement, the pair has a
// permitted relationship (friendship or at least one shared Project) and
// neither user has blocked the other. The per-pair advisory lock (shared with
// FriendRepository's relationship writes) serializes the check against a
// concurrent Block, so a message can never be sent after a Block commits.
func (r *DirectMessageRepository) Create(ctx context.Context, senderID, recipientID, body string) (*directmessage.Message, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	for _, id := range []string{senderID, recipientID} {
		exists, err := userExists(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, directmessage.ErrNotFound
		}
	}
	if err := lockPair(ctx, tx, senderID, recipientID); err != nil {
		return nil, err
	}

	var message directmessage.Message
	err = tx.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO direct_messages (sender_id, recipient_id, body)
			SELECT $1, $2, $3
			WHERE $1::uuid <> $2::uuid
			  AND (
				EXISTS (
					SELECT 1 FROM friendships
					WHERE user_a_id = LEAST($1::uuid, $2::uuid)
					  AND user_b_id = GREATEST($1::uuid, $2::uuid)
				)
				OR EXISTS (
					SELECT 1 FROM project_members a
					JOIN project_members b ON b.project_id = a.project_id
					WHERE a.user_id = $1 AND b.user_id = $2
				)
			  )
			  AND NOT EXISTS (
				SELECT 1 FROM blocks
				WHERE (blocker_id = $1 AND blocked_id = $2)
				   OR (blocker_id = $2 AND blocked_id = $1)
			  )
			RETURNING id, sender_id, recipient_id, body, created_at
		)
		SELECT i.id, i.sender_id, su.username, i.recipient_id, ru.username, i.body, i.created_at
		FROM inserted i
		JOIN users su ON su.id = i.sender_id
		JOIN users ru ON ru.id = i.recipient_id
	`, senderID, recipientID, body).Scan(
		&message.ID, &message.SenderID, &message.SenderUsername,
		&message.RecipientID, &message.RecipientUsername, &message.Body, &message.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, directmessage.ErrForbidden
		}
		if isInvalidUUID(err) {
			return nil, directmessage.ErrNotFound
		}
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &message, nil
}

// ListPage returns the conversation between viewerID and otherUserID. The
// WHERE clause restricts rows to those where viewerID is a participant, so a
// caller can never read another pair's messages. Existing history stays
// readable after the relationship ends; sending is what gets re-checked.
func (r *DirectMessageRepository) ListPage(ctx context.Context, viewerID, otherUserID string, before *directmessage.Cursor, limit int) ([]directmessage.Message, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, otherUserID).Scan(&exists); err != nil {
		if isInvalidUUID(err) {
			return nil, directmessage.ErrNotFound
		}
		return nil, err
	}
	if !exists || otherUserID == viewerID {
		return nil, directmessage.ErrNotFound
	}

	var beforeTime, beforeID any
	if before != nil {
		beforeTime = before.CreatedAt
		beforeID = before.ID
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, sender_id, sender_username, recipient_id, recipient_username, body, created_at
		FROM (
			SELECT m.id, m.sender_id, su.username AS sender_username,
			       m.recipient_id, ru.username AS recipient_username,
			       m.body, m.created_at
			FROM direct_messages m
			JOIN users su ON su.id = m.sender_id
			JOIN users ru ON ru.id = m.recipient_id
			WHERE LEAST(m.sender_id, m.recipient_id) = LEAST($1::uuid, $2::uuid)
			  AND GREATEST(m.sender_id, m.recipient_id) = GREATEST($1::uuid, $2::uuid)
			  AND ((m.sender_id = $1 AND m.recipient_id = $2) OR (m.sender_id = $2 AND m.recipient_id = $1))
			  AND m.deleted_at IS NULL
			  AND ($4::timestamptz IS NULL OR (m.created_at, m.id) < ($4, $5::uuid))
			ORDER BY m.created_at DESC, m.id DESC
			LIMIT $3
		) recent
		ORDER BY created_at ASC, id ASC
	`, viewerID, otherUserID, limit, beforeTime, beforeID)
	if err != nil {
		if isInvalidUUID(err) {
			return nil, directmessage.ErrNotFound
		}
		return nil, err
	}
	defer rows.Close()

	messages := make([]directmessage.Message, 0)
	for rows.Next() {
		var m directmessage.Message
		if err := rows.Scan(&m.ID, &m.SenderID, &m.SenderUsername, &m.RecipientID,
			&m.RecipientUsername, &m.Body, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (r *DirectMessageRepository) ListConversations(ctx context.Context, viewerID string) ([]directmessage.Conversation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.other_id, u.username, c.body, c.created_at, c.from_me
		FROM (
			SELECT DISTINCT ON (other_id) other_id, id, body, created_at, from_me
			FROM (
				SELECT CASE WHEN sender_id = $1 THEN recipient_id ELSE sender_id END AS other_id,
				       id, body, created_at, sender_id = $1 AS from_me
				FROM direct_messages
				WHERE (sender_id = $1 OR recipient_id = $1) AND deleted_at IS NULL
			) mine
			ORDER BY other_id, created_at DESC, id DESC
		) c
		JOIN users u ON u.id = c.other_id
		ORDER BY c.created_at DESC, c.id DESC
	`, viewerID)
	if err != nil {
		if isInvalidUUID(err) {
			return []directmessage.Conversation{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	conversations := make([]directmessage.Conversation, 0)
	for rows.Next() {
		var c directmessage.Conversation
		if err := rows.Scan(&c.OtherUserID, &c.OtherUsername, &c.LastMessageBody, &c.LastMessageAt, &c.LastMessageFromMe); err != nil {
			return nil, err
		}
		conversations = append(conversations, c)
	}
	return conversations, rows.Err()
}

// SoftDelete deletes a message only when actorID is its sender. A
// non-participant sees not_found (existence is not leaked); the recipient
// sees forbidden.
func (r *DirectMessageRepository) SoftDelete(ctx context.Context, messageID, actorID string) (*directmessage.Message, error) {
	var (
		outcome                          string
		id, senderID, senderName         string
		recipientID, recipientName, body string
		createdAt                        *time.Time
	)
	err := r.pool.QueryRow(ctx, `
		WITH target AS MATERIALIZED (
			SELECT m.id, m.sender_id, su.username AS sender_username,
			       m.recipient_id, ru.username AS recipient_username, m.body, m.created_at
			FROM direct_messages m
			JOIN users su ON su.id = m.sender_id
			JOIN users ru ON ru.id = m.recipient_id
			WHERE m.id = $1 AND m.deleted_at IS NULL
			  AND (m.sender_id = $2 OR m.recipient_id = $2)
		), updated AS (
			UPDATE direct_messages m
			SET deleted_at = now(), deleted_by_user_id = $2
			FROM target
			WHERE m.id = target.id AND target.sender_id = $2 AND m.deleted_at IS NULL
			RETURNING m.id
		)
		SELECT CASE
			WHEN NOT EXISTS (SELECT 1 FROM target) THEN 'not_found'
			WHEN NOT EXISTS (SELECT 1 FROM updated) THEN 'forbidden'
			ELSE 'deleted'
		END,
		COALESCE((SELECT id::text FROM target), ''),
		COALESCE((SELECT sender_id::text FROM target), ''),
		COALESCE((SELECT sender_username FROM target), ''),
		COALESCE((SELECT recipient_id::text FROM target), ''),
		COALESCE((SELECT recipient_username FROM target), ''),
		COALESCE((SELECT body FROM target), ''),
		(SELECT created_at FROM target)
	`, messageID, actorID).Scan(&outcome, &id, &senderID, &senderName, &recipientID, &recipientName, &body, &createdAt)
	if err != nil {
		if isInvalidUUID(err) {
			return nil, directmessage.ErrNotFound
		}
		return nil, err
	}
	switch outcome {
	case "not_found":
		return nil, directmessage.ErrNotFound
	case "forbidden":
		return nil, directmessage.ErrForbidden
	}
	return &directmessage.Message{
		ID: id, SenderID: senderID, SenderUsername: senderName,
		RecipientID: recipientID, RecipientUsername: recipientName,
		Body: body, CreatedAt: *createdAt,
	}, nil
}

func (r *DirectMessageRepository) PurgeDeletedBefore(ctx context.Context, cutoff time.Time) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM direct_messages
		WHERE deleted_at IS NOT NULL AND deleted_at <= $1
	`, cutoff)
	return err
}
