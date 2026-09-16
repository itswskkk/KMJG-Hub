// Package session implements Server-managed opaque session tokens, per
// docs/ARCHITECTURE.md "Session Management" and "Session Security".
package session

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a session token does not correspond to an
// active (non-expired, non-revoked) session.
var ErrNotFound = errors.New("session: not found or invalid")

// Session is a Server-side authoritative session record. The Client only
// ever holds the opaque bearer token; TokenHash is what the Server persists.
type Session struct {
	ID         string
	UserID     string
	TokenHash  string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	LastActive time.Time
}

// Repository persists and retrieves sessions. Implementations must treat a
// session as invalid once it is expired or revoked.
type Repository interface {
	Create(ctx context.Context, s *Session) error
	// GetActiveByTokenHash returns the session for tokenHash only if it is
	// neither expired nor revoked; otherwise it returns ErrNotFound.
	GetActiveByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	Revoke(ctx context.Context, tokenHash string) error
}
