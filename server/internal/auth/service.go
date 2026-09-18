// Package auth implements local username/email + password authentication,
// per docs/ARCHITECTURE.md "Authentication Architecture" -> "Local
// Authentication" and "Session Management". GitHub authentication is out of
// scope for this checkpoint; see docs/ARCHITECTURE.md "GitHub Authentication"
// for the deferred design.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/session"
	"github.com/itswskkk/KMJG-Hub/server/internal/user"
)

// ErrInvalidCredentials is returned for a login attempt with an unknown
// identifier or a password that does not match.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")

// ErrSessionInvalid is returned when a bearer token does not resolve to an
// active session (missing, expired, revoked, or its user no longer exists).
// This is deliberately distinct from ErrInvalidCredentials: a failed login
// attempt and an expired/invalid session are different situations the
// Client should present differently (docs/ARCHITECTURE.md "Session
// Security": a revoked or expired session "must no longer authorize" API
// requests, which is a different condition than a wrong password).
var ErrSessionInvalid = errors.New("auth: invalid or expired session")

// Result is the outcome of a successful Register or Login call: the
// authenticated user together with a newly issued session.
type Result struct {
	User      *user.User
	Token     string
	ExpiresAt time.Time
}

// Service implements local account registration and authentication.
type Service struct {
	Users      user.Repository
	Sessions   session.Repository
	SessionTTL time.Duration
}

// RegisterInput carries the minimum information required to create a local
// account, per docs/UX.md "Registration".
type RegisterInput struct {
	Username string
	Email    string
	Password string
}

// Register creates a new local account and immediately issues a session for
// it, per docs/UX.md: "After successful registration, the user continues
// into that Server's KMJG Hub experience."
func (s *Service) Register(ctx context.Context, in RegisterInput) (*Result, error) {
	username := strings.TrimSpace(in.Username)
	email := strings.TrimSpace(strings.ToLower(in.Email))

	if err := validateUsername(username); err != nil {
		return nil, err
	}
	if err := validateEmail(email); err != nil {
		return nil, err
	}
	if err := validatePassword(in.Password); err != nil {
		return nil, err
	}

	hash, err := HashPassword(in.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	u := &user.User{
		Username:     username,
		Email:        email,
		PasswordHash: hash,
	}
	if err := s.Users.Create(ctx, u); err != nil {
		return nil, err // may be user.ErrDuplicate
	}

	return s.issueSession(ctx, u)
}

// LoginInput carries the credentials for a login attempt.
type LoginInput struct {
	Identifier string // username or email
	Password   string
}

// Login verifies credentials against the stored account and issues a new
// session on success.
func (s *Service) Login(ctx context.Context, in LoginInput) (*Result, error) {
	identifier := normalizeIdentifier(in.Identifier)
	if identifier == "" || in.Password == "" {
		return nil, ErrInvalidCredentials
	}

	u, err := s.Users.GetByUsernameOrEmail(ctx, identifier)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	ok, err := VerifyPassword(u.PasswordHash, in.Password)
	if err != nil {
		return nil, fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return nil, ErrInvalidCredentials
	}

	return s.issueSession(ctx, u)
}

// Logout revokes the session identified by the given bearer token. Revoking
// an already-invalid token is not an error, per docs/ARCHITECTURE.md
// "Logout and Revocation".
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.Sessions.Revoke(ctx, HashSessionToken(token))
}

// CurrentUser resolves the bearer token to its authenticated user, or
// ErrSessionInvalid if the token is missing, expired, or revoked.
func (s *Service) CurrentUser(ctx context.Context, token string) (*user.User, error) {
	if token == "" {
		return nil, ErrSessionInvalid
	}

	sess, err := s.Sessions.GetActiveByTokenHash(ctx, HashSessionToken(token))
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, ErrSessionInvalid
		}
		return nil, err
	}

	u, err := s.Users.GetByID(ctx, sess.UserID)
	if err != nil {
		if errors.Is(err, user.ErrNotFound) {
			// The session is valid but the user it names no longer exists;
			// treat this the same as an invalid session rather than a bad
			// login attempt.
			return nil, ErrSessionInvalid
		}
		return nil, err
	}
	return u, nil
}

// SessionActive reports whether tokenHash (as produced by
// HashSessionToken) still names an active, non-expired, non-revoked
// session. Used by the real-time layer to periodically revalidate
// already-open WebSocket connections against authoritative session state
// (docs/ARCHITECTURE.md "Logout and Revocation": "A revoked or expired
// session must no longer authorize ... WebSocket connections") without
// ever handling or logging the raw token itself.
func (s *Service) SessionActive(ctx context.Context, tokenHash string) bool {
	_, err := s.Sessions.GetActiveByTokenHash(ctx, tokenHash)
	return err == nil
}

func (s *Service) issueSession(ctx context.Context, u *user.User) (*Result, error) {
	token, err := GenerateSessionToken()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	sess := &session.Session{
		UserID:     u.ID,
		TokenHash:  HashSessionToken(token),
		CreatedAt:  now,
		ExpiresAt:  now.Add(s.SessionTTL),
		LastActive: now,
	}
	if err := s.Sessions.Create(ctx, sess); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &Result{User: u, Token: token, ExpiresAt: sess.ExpiresAt}, nil
}
