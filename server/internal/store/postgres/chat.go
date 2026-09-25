package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itswskkk/KMJG-Hub/server/internal/chat"
)

type ChatRepository struct {
	pool *pgxpool.Pool
}

func NewChatRepository(pool *pgxpool.Pool) *ChatRepository {
	return &ChatRepository{pool: pool}
}

func (r *ChatRepository) Create(ctx context.Context, projectID, authorID, body string) (*chat.Message, error) {
	var message chat.Message
	err := r.pool.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO project_chat_messages (project_id, author_user_id, body)
			SELECT $1, $2, $3
			WHERE EXISTS (
				SELECT 1 FROM project_members
				WHERE project_id = $1 AND user_id = $2
			)
			RETURNING id, project_id, author_user_id, body, created_at
		)
		SELECT i.id, i.project_id, i.author_user_id, u.username, i.body, i.created_at
		FROM inserted i
		JOIN users u ON u.id = i.author_user_id
	`, projectID, authorID, body).Scan(
		&message.ID, &message.ProjectID, &message.AuthorID,
		&message.AuthorUsername, &message.Body, &message.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
			return nil, chat.ErrNotFound
		}
		return nil, err
	}
	return &message, nil
}

func (r *ChatRepository) ListRecent(ctx context.Context, projectID, viewerID string, limit int) ([]chat.Message, error) {
	var member bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM project_members WHERE project_id = $1 AND user_id = $2
		)
	`, projectID, viewerID).Scan(&member)
	if err != nil {
		if isInvalidUUID(err) {
			return nil, chat.ErrNotFound
		}
		return nil, err
	}
	if !member {
		return nil, chat.ErrNotFound
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, project_id, author_user_id, author_username, body, created_at
		FROM (
			SELECT m.id, m.project_id, m.author_user_id, u.username AS author_username,
			       m.body, m.created_at
			FROM project_chat_messages m
			JOIN users u ON u.id = m.author_user_id
			WHERE m.project_id = $1
			  AND m.deleted_at IS NULL
			  AND EXISTS (
				SELECT 1 FROM project_members
				WHERE project_id = m.project_id AND user_id = $2
			  )
			ORDER BY m.created_at DESC, m.id DESC
			LIMIT $3
		) recent
		ORDER BY created_at ASC, id ASC
	`, projectID, viewerID, limit)
	if err != nil {
		if isInvalidUUID(err) {
			return nil, chat.ErrNotFound
		}
		return nil, err
	}
	defer rows.Close()

	messages := make([]chat.Message, 0)
	for rows.Next() {
		var message chat.Message
		if err := rows.Scan(&message.ID, &message.ProjectID, &message.AuthorID,
			&message.AuthorUsername, &message.Body, &message.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (r *ChatRepository) SoftDelete(ctx context.Context, projectID, messageID, actorID string) (*chat.Message, error) {
	var (
		outcome                  string
		id, returnedProjectID    string
		authorID, username, body string
		createdAt                *time.Time
	)
	err := r.pool.QueryRow(ctx, `
		WITH actor AS MATERIALIZED (
			SELECT role FROM project_members WHERE project_id = $1 AND user_id = $3
		), target AS MATERIALIZED (
			SELECT m.id, m.project_id, m.author_user_id, u.username, m.body, m.created_at
			FROM project_chat_messages m
			JOIN users u ON u.id = m.author_user_id
			WHERE m.project_id = $1 AND m.id = $2 AND m.deleted_at IS NULL
		), updated AS (
			UPDATE project_chat_messages m
			SET deleted_at = now(), deleted_by_user_id = $3
			FROM actor, target
			WHERE m.id = target.id
			  AND (target.author_user_id = $3 OR actor.role IN ('owner', 'admin'))
			RETURNING m.id
		)
		SELECT CASE
			WHEN NOT EXISTS (SELECT 1 FROM actor) THEN 'not_found'
			WHEN NOT EXISTS (SELECT 1 FROM target) THEN 'not_found'
			WHEN NOT EXISTS (SELECT 1 FROM updated) THEN 'forbidden'
			ELSE 'deleted'
		END,
		COALESCE((SELECT id::text FROM target), ''),
		COALESCE((SELECT project_id::text FROM target), ''),
		COALESCE((SELECT author_user_id::text FROM target), ''),
		COALESCE((SELECT username FROM target), ''),
		COALESCE((SELECT body FROM target), ''),
		(SELECT created_at FROM target)
	`, projectID, messageID, actorID).Scan(
		&outcome, &id, &returnedProjectID, &authorID, &username, &body, &createdAt,
	)
	if err != nil {
		if isInvalidUUID(err) {
			return nil, chat.ErrNotFound
		}
		return nil, err
	}
	switch outcome {
	case "not_found":
		return nil, chat.ErrNotFound
	case "forbidden":
		return nil, chat.ErrForbidden
	}
	return &chat.Message{
		ID: id, ProjectID: returnedProjectID, AuthorID: authorID,
		AuthorUsername: username, Body: body, CreatedAt: *createdAt,
	}, nil
}

func (r *ChatRepository) PurgeDeletedBefore(ctx context.Context, cutoff time.Time) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM project_chat_messages
		WHERE deleted_at IS NOT NULL AND deleted_at <= $1
	`, cutoff)
	return err
}

func isInvalidUUID(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == invalidTextRepresentation
}
