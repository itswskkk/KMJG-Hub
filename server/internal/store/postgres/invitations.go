package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itswskkk/KMJG-Hub/server/internal/invitation"
)

type InvitationRepository struct{ pool *pgxpool.Pool }

func invitationNotFound(err error) bool {
	if errors.Is(err, pgx.ErrNoRows) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == invalidTextRepresentation
}

func NewInvitationRepository(pool *pgxpool.Pool) *InvitationRepository {
	return &InvitationRepository{pool: pool}
}

func scanDirect(row pgx.Row) (*invitation.Direct, error) {
	var item invitation.Direct
	err := row.Scan(&item.ID, &item.ProjectID, &item.ProjectName, &item.InviterUserID, &item.InviterUsername,
		&item.RecipientUserID, &item.RecipientUsername, &item.CreatedAt, &item.ExpiresAt)
	return &item, err
}

func (r *InvitationRepository) CreateDirect(ctx context.Context, projectID, inviterID, recipientIdentifier string, expiresAt *time.Time) (*invitation.Direct, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var role string
	if err = tx.QueryRow(ctx, `SELECT role FROM project_members WHERE project_id=$1 AND user_id=$2`, projectID, inviterID).Scan(&role); err != nil {
		if invitationNotFound(err) {
			return nil, invitation.ErrNotFound
		}
		return nil, err
	}
	if role != "owner" && role != "admin" {
		return nil, invitation.ErrForbidden
	}

	var recipientID string
	if err = tx.QueryRow(ctx, `SELECT id FROM users WHERE lower(username)=lower($1) OR lower(email)=lower($1)`, recipientIdentifier).Scan(&recipientID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invitation.ErrRecipientNotFound
		}
		return nil, err
	}
	if recipientID == inviterID {
		return nil, invitation.ErrConflict
	}
	// Serialize create attempts for this Project/recipient pair so two
	// concurrent requests cannot both pass the pending-invitation check.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text || ':' || $2::text, 0))`, projectID, recipientID); err != nil {
		return nil, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$2)`, projectID, recipientID).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, invitation.ErrConflict
	}
	if err = tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM project_direct_invitations WHERE project_id=$1 AND recipient_user_id=$2
		AND accepted_at IS NULL AND declined_at IS NULL AND cancelled_at IS NULL
		AND (expires_at IS NULL OR expires_at > now()))`, projectID, recipientID).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, invitation.ErrConflict
	}

	item, err := scanDirect(tx.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO project_direct_invitations(project_id, inviter_user_id, recipient_user_id, expires_at)
			VALUES($1,$2,$3,$4) RETURNING *
		)
		SELECT i.id,i.project_id,p.name,i.inviter_user_id,iu.username,i.recipient_user_id,ru.username,i.created_at,i.expires_at
		FROM inserted i JOIN projects p ON p.id=i.project_id JOIN users iu ON iu.id=i.inviter_user_id JOIN users ru ON ru.id=i.recipient_user_id
	`, projectID, inviterID, recipientID, expiresAt))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *InvitationRepository) ListReceived(ctx context.Context, recipientID string) ([]invitation.Direct, error) {
	return r.list(ctx, `WHERE i.recipient_user_id=$1`, recipientID)
}

func (r *InvitationRepository) ListForProject(ctx context.Context, projectID, actorID string) ([]invitation.Direct, error) {
	var role string
	err := r.pool.QueryRow(ctx, `SELECT role FROM project_members WHERE project_id=$1 AND user_id=$2`, projectID, actorID).Scan(&role)
	if err != nil {
		if invitationNotFound(err) {
			return nil, invitation.ErrNotFound
		}
		return nil, err
	}
	if role != "owner" && role != "admin" {
		return nil, invitation.ErrForbidden
	}
	return r.list(ctx, `WHERE i.project_id=$1`, projectID)
}

func (r *InvitationRepository) list(ctx context.Context, where string, arg string) ([]invitation.Direct, error) {
	rows, err := r.pool.Query(ctx, `SELECT i.id,i.project_id,p.name,i.inviter_user_id,iu.username,i.recipient_user_id,ru.username,i.created_at,i.expires_at
		FROM project_direct_invitations i JOIN projects p ON p.id=i.project_id JOIN users iu ON iu.id=i.inviter_user_id JOIN users ru ON ru.id=i.recipient_user_id `+where+`
		AND i.accepted_at IS NULL AND i.declined_at IS NULL AND i.cancelled_at IS NULL AND (i.expires_at IS NULL OR i.expires_at>now()) ORDER BY i.created_at DESC`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []invitation.Direct
	for rows.Next() {
		item, err := scanDirect(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *InvitationRepository) Accept(ctx context.Context, invitationID, recipientID string, now time.Time) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var projectID string
	err = tx.QueryRow(ctx, `SELECT project_id FROM project_direct_invitations WHERE id=$1 AND recipient_user_id=$2 AND accepted_at IS NULL AND declined_at IS NULL AND cancelled_at IS NULL AND (expires_at IS NULL OR expires_at>$3) FOR UPDATE`, invitationID, recipientID, now).Scan(&projectID)
	if invitationNotFound(err) {
		return "", invitation.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	var member bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$2)`, projectID, recipientID).Scan(&member); err != nil {
		return "", err
	}
	if member {
		return "", invitation.ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO project_members(project_id,user_id,role) VALUES($1,$2,'member')`, projectID, recipientID); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE project_direct_invitations SET accepted_at=$2 WHERE id=$1`, invitationID, now); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return projectID, nil
}

func (r *InvitationRepository) Decline(ctx context.Context, invitationID, recipientID string, now time.Time) error {
	tag, err := r.pool.Exec(ctx, `UPDATE project_direct_invitations SET declined_at=$3 WHERE id=$1 AND recipient_user_id=$2 AND accepted_at IS NULL AND declined_at IS NULL AND cancelled_at IS NULL AND (expires_at IS NULL OR expires_at>$3)`, invitationID, recipientID, now)
	if err != nil {
		if invitationNotFound(err) {
			return invitation.ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() != 1 {
		return invitation.ErrNotFound
	}
	return nil
}

func (r *InvitationRepository) Cancel(ctx context.Context, projectID, invitationID, inviterID string, now time.Time) error {
	tag, err := r.pool.Exec(ctx, `UPDATE project_direct_invitations SET cancelled_at=$4 WHERE id=$1 AND project_id=$2 AND inviter_user_id=$3 AND accepted_at IS NULL AND declined_at IS NULL AND cancelled_at IS NULL AND (expires_at IS NULL OR expires_at>$4)`, invitationID, projectID, inviterID, now)
	if err != nil {
		if invitationNotFound(err) {
			return invitation.ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() != 1 {
		return invitation.ErrNotFound
	}
	return nil
}

func scanCredential(row pgx.Row) (*invitation.Credential, error) {
	var item invitation.Credential
	err := row.Scan(&item.ID, &item.ProjectID, &item.ProjectName, &item.CreatorUserID,
		&item.CreatorUsername, &item.CreatedAt, &item.ExpiresAt, &item.MaxUses, &item.Uses)
	return &item, err
}

func requireProjectManager(ctx context.Context, tx pgx.Tx, projectID, userID string) error {
	var role string
	err := tx.QueryRow(ctx, `SELECT role FROM project_members WHERE project_id=$1 AND user_id=$2`, projectID, userID).Scan(&role)
	if invitationNotFound(err) {
		return invitation.ErrNotFound
	}
	if err != nil {
		return err
	}
	if role != "owner" && role != "admin" {
		return invitation.ErrForbidden
	}
	return nil
}

func (r *InvitationRepository) CreateCredential(ctx context.Context, projectID, creatorID, tokenHash string, expiresAt *time.Time, maxUses *int) (*invitation.Credential, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = requireProjectManager(ctx, tx, projectID, creatorID); err != nil {
		return nil, err
	}
	item, err := scanCredential(tx.QueryRow(ctx, `WITH inserted AS (
		INSERT INTO project_invite_credentials(project_id,creator_user_id,token_hash,expires_at,max_uses)
		VALUES($1,$2,$3,$4,$5) RETURNING *)
		SELECT i.id,i.project_id,p.name,i.creator_user_id,u.username,i.created_at,i.expires_at,i.max_uses,i.uses
		FROM inserted i JOIN projects p ON p.id=i.project_id JOIN users u ON u.id=i.creator_user_id`, projectID, creatorID, tokenHash, expiresAt, maxUses))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *InvitationRepository) ListCredentials(ctx context.Context, projectID, actorID string) ([]invitation.Credential, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = requireProjectManager(ctx, tx, projectID, actorID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT i.id,i.project_id,p.name,i.creator_user_id,u.username,i.created_at,i.expires_at,i.max_uses,i.uses
		FROM project_invite_credentials i JOIN projects p ON p.id=i.project_id JOIN users u ON u.id=i.creator_user_id
		WHERE i.project_id=$1 AND i.revoked_at IS NULL AND (i.expires_at IS NULL OR i.expires_at>now()) AND (i.max_uses IS NULL OR i.uses<i.max_uses) ORDER BY i.created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []invitation.Credential
	for rows.Next() {
		item, err := scanCredential(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *InvitationRepository) RevokeCredential(ctx context.Context, projectID, credentialID, actorID string, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = requireProjectManager(ctx, tx, projectID, actorID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE project_invite_credentials SET revoked_at=$3 WHERE id=$1 AND project_id=$2 AND revoked_at IS NULL`, credentialID, projectID, now)
	if err != nil {
		if invitationNotFound(err) {
			return invitation.ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() != 1 {
		return invitation.ErrNotFound
	}
	return tx.Commit(ctx)
}

func (r *InvitationRepository) ConsumeCredential(ctx context.Context, tokenHash, userID string, now time.Time) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var projectID string
	var expiresAt, revokedAt *time.Time
	var maxUses *int
	var uses int
	err = tx.QueryRow(ctx, `SELECT project_id,expires_at,max_uses,uses,revoked_at FROM project_invite_credentials WHERE token_hash=$1 FOR UPDATE`, tokenHash).Scan(&projectID, &expiresAt, &maxUses, &uses, &revokedAt)
	if invitationNotFound(err) {
		return "", invitation.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if revokedAt != nil {
		return "", invitation.ErrRevoked
	}
	if expiresAt != nil && !expiresAt.After(now) {
		return "", invitation.ErrExpired
	}
	if maxUses != nil && uses >= *maxUses {
		return "", invitation.ErrUsesExhausted
	}
	var member bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id=$2)`, projectID, userID).Scan(&member); err != nil {
		return "", err
	}
	if member {
		return "", invitation.ErrAlreadyMember
	}
	if _, err = tx.Exec(ctx, `INSERT INTO project_members(project_id,user_id,role) VALUES($1,$2,'member')`, projectID, userID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return "", invitation.ErrAlreadyMember
		}
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE project_invite_credentials SET uses=uses+1 WHERE token_hash=$1`, tokenHash); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return projectID, nil
}
