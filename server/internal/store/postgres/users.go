package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itswskkk/KMJG-Hub/server/internal/user"
)

// uniqueViolation is the PostgreSQL SQLSTATE for a unique constraint
// violation (23505), used here to translate a duplicate username/email
// insert into the domain-level user.ErrDuplicate.
const uniqueViolation = "23505"

// UserRepository implements user.Repository against PostgreSQL.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository constructs a UserRepository backed by pool.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Create(ctx context.Context, u *user.User) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO users (username, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`, u.Username, u.Email, u.PasswordHash).Scan(&u.ID, &u.CreatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return user.ErrDuplicate
		}
		return err
	}
	return nil
}

func (r *UserRepository) GetByUsernameOrEmail(ctx context.Context, identifier string) (*user.User, error) {
	u := &user.User{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, username, email, password_hash, created_at
		FROM users
		WHERE lower(username) = lower($1) OR lower(email) = lower($1)
	`, identifier).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, user.ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*user.User, error) {
	u := &user.User{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, username, email, password_hash, created_at
		FROM users
		WHERE id = $1
	`, id).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, user.ErrNotFound
		}
		return nil, err
	}
	return u, nil
}
