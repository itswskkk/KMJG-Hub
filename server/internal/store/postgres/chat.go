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
			RETURNING id, project_id, kind, author_user_id, body, created_at
		)
		SELECT i.id, i.project_id, i.kind, i.author_user_id, u.username, i.body, i.created_at
		FROM inserted i
		JOIN users u ON u.id = i.author_user_id
	`, projectID, authorID, body).Scan(
		&message.ID, &message.ProjectID, &message.Kind, &message.AuthorID,
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

func (r *ChatRepository) ListPage(ctx context.Context, projectID, viewerID string, before *chat.Cursor, limit int) ([]chat.Message, error) {
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

	var beforeTime any
	var beforeID any
	if before != nil {
		beforeTime = before.CreatedAt
		beforeID = before.ID
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, project_id, kind, author_id, author_username, body, created_at
		FROM (
			SELECT m.id, m.project_id, m.kind,
			       COALESCE(m.author_user_id::text, '') AS author_id,
			       COALESCE(u.username, '') AS author_username,
			       m.body, m.created_at
			FROM project_chat_messages m
			LEFT JOIN users u ON u.id = m.author_user_id
			WHERE m.project_id = $1
			  AND m.deleted_at IS NULL
			  AND ($4::timestamptz IS NULL OR (m.created_at,m.id) < ($4,$5::uuid))
			  AND EXISTS (
				SELECT 1 FROM project_members
				WHERE project_id = m.project_id AND user_id = $2
			  )
			ORDER BY m.created_at DESC, m.id DESC
			LIMIT $3
		) recent
		ORDER BY created_at ASC, id ASC
	`, projectID, viewerID, limit, beforeTime, beforeID)
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
		if err := rows.Scan(&message.ID, &message.ProjectID, &message.Kind, &message.AuthorID,
			&message.AuthorUsername, &message.Body, &message.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return messages, nil
	}
	messageByID := make(map[string]*chat.Message, len(messages))
	ids := make([]string, len(messages))
	for i := range messages {
		messageByID[messages[i].ID] = &messages[i]
		ids[i] = messages[i].ID
	}
	attachmentRows, err := r.pool.Query(ctx, `SELECT id,message_id,project_id,storage_id,filename,content_type,size_bytes FROM project_chat_attachments WHERE message_id=ANY($1::uuid[]) ORDER BY created_at,id`, ids)
	if err != nil {
		return nil, err
	}
	defer attachmentRows.Close()
	for attachmentRows.Next() {
		var a chat.Attachment
		if err := attachmentRows.Scan(&a.ID, &a.MessageID, &a.ProjectID, &a.StorageID, &a.Filename, &a.ContentType, &a.SizeBytes); err != nil {
			return nil, err
		}
		messageByID[a.MessageID].Attachments = append(messageByID[a.MessageID].Attachments, a)
	}
	return messages, attachmentRows.Err()
}

func (r *ChatRepository) CreateWithAttachment(ctx context.Context, projectID, authorID, body string, attachment chat.Attachment, maxProjectBytes int64) (*chat.Message, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// Serialize quota accounting per Project without introducing a mutable
	// counter that can drift from authoritative attachment rows.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, projectID); err != nil {
		return nil, err
	}
	var used int64
	err = tx.QueryRow(ctx, `SELECT COALESCE(sum(size_bytes),0) FROM project_chat_attachments WHERE project_id=$1`, projectID).Scan(&used)
	if isInvalidUUID(err) {
		return nil, chat.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if used+attachment.SizeBytes > maxProjectBytes {
		return nil, chat.ErrStorageLimit
	}
	var message chat.Message
	err = tx.QueryRow(ctx, `WITH inserted AS (
		INSERT INTO project_chat_messages(project_id,author_user_id,body)
		SELECT $1,$2,$3 WHERE EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$2)
		RETURNING id,project_id,kind,author_user_id,body,created_at)
		SELECT i.id,i.project_id,i.kind,i.author_user_id,u.username,i.body,i.created_at FROM inserted i JOIN users u ON u.id=i.author_user_id`, projectID, authorID, body).Scan(&message.ID, &message.ProjectID, &message.Kind, &message.AuthorID, &message.AuthorUsername, &message.Body, &message.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return nil, chat.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	attachment.MessageID = message.ID
	attachment.ProjectID = projectID
	err = tx.QueryRow(ctx, `INSERT INTO project_chat_attachments(message_id,project_id,storage_id,filename,content_type,size_bytes) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, message.ID, projectID, attachment.StorageID, attachment.Filename, attachment.ContentType, attachment.SizeBytes).Scan(&attachment.ID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	message.Attachments = []chat.Attachment{attachment}
	return &message, nil
}

// CreateSystem inserts a Server-generated message (no author) into an
// existing, non-deleted Project.
func (r *ChatRepository) CreateSystem(ctx context.Context, projectID, kind, body string) (*chat.Message, error) {
	message := chat.Message{Kind: kind}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO project_chat_messages (project_id, kind, body)
		SELECT p.id, $2::text, $3::text FROM projects p WHERE p.id = $1::uuid AND p.deleted_at IS NULL
		RETURNING id, project_id, body, created_at
	`, projectID, kind, body).Scan(&message.ID, &message.ProjectID, &message.Body, &message.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return nil, chat.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &message, nil
}

// ListFiles lists every attachment of projectID's active messages, newest
// first, to a Project member.
func (r *ChatRepository) ListFiles(ctx context.Context, projectID, viewerID string) ([]chat.ProjectFile, error) {
	var member bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$2)`, projectID, viewerID).Scan(&member)
	if isInvalidUUID(err) {
		return nil, chat.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !member {
		return nil, chat.ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `
		SELECT a.id, a.message_id, a.project_id, a.storage_id, a.filename, a.content_type, a.size_bytes,
		       COALESCE(m.author_user_id::text, ''), COALESCE(u.username, ''), a.created_at
		FROM project_chat_attachments a
		JOIN project_chat_messages m ON m.id = a.message_id
		LEFT JOIN users u ON u.id = m.author_user_id
		WHERE a.project_id = $1 AND m.deleted_at IS NULL
		ORDER BY a.created_at DESC, a.id DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := make([]chat.ProjectFile, 0)
	for rows.Next() {
		var f chat.ProjectFile
		if err := rows.Scan(&f.ID, &f.MessageID, &f.ProjectID, &f.StorageID, &f.Filename, &f.ContentType, &f.SizeBytes,
			&f.AuthorID, &f.AuthorUsername, &f.CreatedAt); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

func (r *ChatRepository) GetAttachment(ctx context.Context, projectID, attachmentID, viewerID string) (*chat.Attachment, error) {
	var a chat.Attachment
	err := r.pool.QueryRow(ctx, `SELECT a.id,a.message_id,a.project_id,a.storage_id,a.filename,a.content_type,a.size_bytes FROM project_chat_attachments a JOIN project_chat_messages m ON m.id=a.message_id WHERE a.project_id=$1 AND a.id=$2 AND m.deleted_at IS NULL AND EXISTS(SELECT 1 FROM project_members WHERE project_id=a.project_id AND user_id=$3)`, projectID, attachmentID, viewerID).Scan(&a.ID, &a.MessageID, &a.ProjectID, &a.StorageID, &a.Filename, &a.ContentType, &a.SizeBytes)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return nil, chat.ErrNotFound
	}
	return &a, err
}

func (r *ChatRepository) ListExpiredAttachmentStorageIDs(ctx context.Context, cutoff time.Time) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT a.storage_id FROM project_chat_attachments a JOIN project_chat_messages m ON m.id=a.message_id WHERE m.deleted_at IS NOT NULL AND m.deleted_at <= $1`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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
			SELECT m.id, m.project_id, m.kind, m.author_user_id, u.username, m.body, m.created_at
			FROM project_chat_messages m
			LEFT JOIN users u ON u.id = m.author_user_id
			WHERE m.project_id = $1 AND m.id = $2 AND m.deleted_at IS NULL
		), updated AS (
			UPDATE project_chat_messages m
			SET deleted_at = now(), deleted_by_user_id = $3
			FROM actor, target
			WHERE m.id = target.id
			  -- Server-generated activity (kind <> 'user') is not an ordinary
			  -- user-authored message: the normal delete action never applies.
			  AND target.kind = 'user'
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
		ID: id, ProjectID: returnedProjectID, Kind: chat.KindUser, AuthorID: authorID,
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
