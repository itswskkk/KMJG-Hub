package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itswskkk/KMJG-Hub/server/internal/session"
)

// SessionRepository implements session.Repository against PostgreSQL.
type SessionRepository struct {
	pool *pgxpool.Pool
}

// NewSessionRepository constructs a SessionRepository backed by pool.
func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

func (r *SessionRepository) Create(ctx context.Context, s *session.Session) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, last_active_at
	`, s.UserID, s.TokenHash, s.ExpiresAt).Scan(&s.ID, &s.CreatedAt, &s.LastActive)
}

func (r *SessionRepository) GetActiveByTokenHash(ctx context.Context, tokenHash string) (*session.Session, error) {
	s := &session.Session{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, token_hash, created_at, expires_at, revoked_at, last_active_at
		FROM sessions
		WHERE token_hash = $1
		  AND revoked_at IS NULL
		  AND expires_at > now()
	`, tokenHash).Scan(&s.ID, &s.UserID, &s.TokenHash, &s.CreatedAt, &s.ExpiresAt, &s.RevokedAt, &s.LastActive)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, session.ErrNotFound
		}
		return nil, err
	}
	return s, nil
}

func (r *SessionRepository) Revoke(ctx context.Context, tokenHash string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`, tokenHash)
	return err
}
