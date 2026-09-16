// Package user defines the local KMJG Hub account model, per
// docs/ARCHITECTURE.md "Local Authentication" and "User Profile and Privacy
// Architecture". This checkpoint only implements the fields required for
// local username/email + password authentication; the broader profile model
// (avatar, bio, presence, etc.) belongs to a later slice.
package user

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a user cannot be located.
var ErrNotFound = errors.New("user: not found")

// ErrDuplicate is returned when a username or email is already registered.
var ErrDuplicate = errors.New("user: username or email already in use")

// User is a Server-local account belonging to this KMJG Hub Server.
type User struct {
	ID           string
	Username     string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

// Repository persists and retrieves local accounts. Implementations must
// enforce case-insensitive uniqueness on username and email.
type Repository interface {
	Create(ctx context.Context, u *User) error
	GetByUsernameOrEmail(ctx context.Context, identifier string) (*User, error)
	GetByID(ctx context.Context, id string) (*User, error)
}
