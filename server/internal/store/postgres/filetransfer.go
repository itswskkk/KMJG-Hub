package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itswskkk/KMJG-Hub/server/internal/filetransfer"
)

// FileTransferRepository implements filetransfer.Repository against
// PostgreSQL. Every state transition is a single guarded UPDATE (actor and
// current status in the WHERE clause), so concurrent accept/decline/cancel
// requests can never both succeed.
type FileTransferRepository struct {
	pool *pgxpool.Pool
}

func NewFileTransferRepository(pool *pgxpool.Pool) *FileTransferRepository {
	return &FileTransferRepository{pool: pool}
}

var _ filetransfer.Repository = (*FileTransferRepository)(nil)

const fileTransferColumns = `
	t.id, t.sender_id, su.username, t.recipient_id, ru.username, t.file_name,
	t.declared_file_size, t.status, t.storage_id, t.actual_file_size, t.content_type,
	t.created_at, t.responded_at, t.uploaded_at`

const fileTransferFrom = `
	FROM file_transfers t
	JOIN users su ON su.id = t.sender_id
	JOIN users ru ON ru.id = t.recipient_id`

func scanFileTransfer(row pgx.Row) (*filetransfer.Transfer, error) {
	var t filetransfer.Transfer
	var status string
	if err := row.Scan(&t.ID, &t.SenderID, &t.SenderUsername, &t.RecipientID, &t.RecipientUsername,
		&t.FileName, &t.DeclaredFileSize, &status, &t.StorageID, &t.ActualFileSize, &t.ContentType,
		&t.CreatedAt, &t.RespondedAt, &t.UploadedAt); err != nil {
		return nil, err
	}
	t.Status = filetransfer.Status(status)
	return &t, nil
}

// Create inserts a pending request only when, in the same statement, the
// pair has a permitted relationship (friendship or a shared Project) and
// neither user has blocked the other — the same rule as Direct Messages.
// The per-pair advisory lock serializes this against a concurrent Block.
func (r *FileTransferRepository) Create(ctx context.Context, senderID, recipientID, fileName string, declaredSize int64) (*filetransfer.Transfer, error) {
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
			return nil, filetransfer.ErrNotFound
		}
	}
	if err := lockPair(ctx, tx, senderID, recipientID); err != nil {
		return nil, err
	}

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO file_transfers (sender_id, recipient_id, file_name, declared_file_size)
		SELECT $1, $2, $3, $4
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
		RETURNING id
	`, senderID, recipientID, fileName, declaredSize).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, filetransfer.ErrForbidden
		}
		return nil, err
	}
	t, err := scanFileTransfer(tx.QueryRow(ctx, `SELECT `+fileTransferColumns+fileTransferFrom+` WHERE t.id = $1`, id))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return t, nil
}

// Get returns a transfer only to its sender or recipient; anyone else (or a
// malformed ID) sees ErrNotFound, so transfer existence is not leaked.
func (r *FileTransferRepository) Get(ctx context.Context, transferID, userID string) (*filetransfer.Transfer, error) {
	t, err := scanFileTransfer(r.pool.QueryRow(ctx, `SELECT `+fileTransferColumns+fileTransferFrom+`
		WHERE t.id = $1 AND (t.sender_id = $2 OR t.recipient_id = $2)`, transferID, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
			return nil, filetransfer.ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

func (r *FileTransferRepository) list(ctx context.Context, column, userID string) ([]filetransfer.Transfer, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+fileTransferColumns+fileTransferFrom+`
		WHERE t.`+column+` = $1 ORDER BY t.created_at DESC, t.id DESC LIMIT 200`, userID)
	if err != nil {
		if isInvalidUUID(err) {
			return []filetransfer.Transfer{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := make([]filetransfer.Transfer, 0)
	for rows.Next() {
		t, err := scanFileTransfer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (r *FileTransferRepository) ListIncoming(ctx context.Context, userID string) ([]filetransfer.Transfer, error) {
	return r.list(ctx, "recipient_id", userID)
}

func (r *FileTransferRepository) ListSent(ctx context.Context, userID string) ([]filetransfer.Transfer, error) {
	return r.list(ctx, "sender_id", userID)
}

func (r *FileTransferRepository) Accept(ctx context.Context, transferID, recipientID string) (*filetransfer.Transfer, error) {
	return r.transition(ctx, transferID, recipientID, false, `
		UPDATE file_transfers SET status = 'accepted', responded_at = now()
		WHERE id = $1 AND recipient_id = $2 AND status = 'pending'`)
}

func (r *FileTransferRepository) Decline(ctx context.Context, transferID, recipientID string) (*filetransfer.Transfer, error) {
	return r.transition(ctx, transferID, recipientID, false, `
		UPDATE file_transfers SET status = 'declined', responded_at = now()
		WHERE id = $1 AND recipient_id = $2 AND status = 'pending'`)
}

func (r *FileTransferRepository) Cancel(ctx context.Context, transferID, senderID string) (*filetransfer.Transfer, error) {
	return r.transition(ctx, transferID, senderID, true, `
		UPDATE file_transfers SET status = 'cancelled', responded_at = COALESCE(responded_at, now())
		WHERE id = $1 AND sender_id = $2 AND status IN ('pending', 'accepted')`)
}

func (r *FileTransferRepository) MarkUploaded(ctx context.Context, transferID, senderID, storageID string, actualSize int64, contentType string) (*filetransfer.Transfer, error) {
	return r.transition(ctx, transferID, senderID, true, `
		UPDATE file_transfers
		SET status = 'uploaded', storage_id = $3, actual_file_size = $4, content_type = $5, uploaded_at = now()
		WHERE id = $1 AND sender_id = $2 AND status = 'accepted' AND declared_file_size = $4`,
		storageID, actualSize, contentType)
}

// transition runs one guarded UPDATE (whose $1 is the transfer ID and $2
// the actor). When it affects no row, a follow-up lookup distinguishes the
// cause: not a participant (or no such transfer) -> ErrNotFound; the wrong
// participant for this action -> ErrForbidden; otherwise the transfer is
// in the wrong state -> ErrInvalidState.
func (r *FileTransferRepository) transition(ctx context.Context, transferID, actorID string, actorIsSender bool, update string, extra ...any) (*filetransfer.Transfer, error) {
	args := append([]any{transferID, actorID}, extra...)
	tag, err := r.pool.Exec(ctx, update, args...)
	if err != nil {
		if isInvalidUUID(err) {
			return nil, filetransfer.ErrNotFound
		}
		return nil, err
	}
	t, err := r.Get(ctx, transferID, actorID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 1 {
		return t, nil
	}
	if (actorIsSender && t.SenderID != actorID) || (!actorIsSender && t.RecipientID != actorID) {
		return nil, filetransfer.ErrForbidden
	}
	return nil, filetransfer.ErrInvalidState
}
