package auth_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/auth"
	"github.com/itswskkk/KMJG-Hub/server/internal/session"
	"github.com/itswskkk/KMJG-Hub/server/internal/user"
)

// fakeUserRepo is a minimal in-memory user.Repository for exercising
// auth.Service without a real PostgreSQL instance.
type fakeUserRepo struct {
	mu     sync.Mutex
	nextID int
	users  []*user.User
}

func (f *fakeUserRepo) Create(_ context.Context, u *user.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, existing := range f.users {
		if strings.EqualFold(existing.Username, u.Username) || strings.EqualFold(existing.Email, u.Email) {
			return user.ErrDuplicate
		}
	}

	f.nextID++
	u.ID = fmt.Sprintf("user-%d", f.nextID)
	u.CreatedAt = time.Now().UTC()
	stored := *u
	f.users = append(f.users, &stored)
	return nil
}

func (f *fakeUserRepo) GetByUsernameOrEmail(_ context.Context, identifier string) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, u := range f.users {
		if strings.EqualFold(u.Username, identifier) || strings.EqualFold(u.Email, identifier) {
			copy := *u
			return &copy, nil
		}
	}
	return nil, user.ErrNotFound
}

func (f *fakeUserRepo) GetByID(_ context.Context, id string) (*user.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, u := range f.users {
		if u.ID == id {
			copy := *u
			return &copy, nil
		}
	}
	return nil, user.ErrNotFound
}

// fakeSessionRepo is a minimal in-memory session.Repository.
type fakeSessionRepo struct {
	mu       sync.Mutex
	nextID   int
	sessions []*session.Session
}

func (f *fakeSessionRepo) Create(_ context.Context, s *session.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nextID++
	s.ID = fmt.Sprintf("session-%d", f.nextID)
	stored := *s
	f.sessions = append(f.sessions, &stored)
	return nil
}

func (f *fakeSessionRepo) GetActiveByTokenHash(_ context.Context, tokenHash string) (*session.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now().UTC()
	for _, s := range f.sessions {
		if s.TokenHash == tokenHash && s.RevokedAt == nil && s.ExpiresAt.After(now) {
			copy := *s
			return &copy, nil
		}
	}
	return nil, session.ErrNotFound
}

func (f *fakeSessionRepo) Revoke(_ context.Context, tokenHash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now().UTC()
	for _, s := range f.sessions {
		if s.TokenHash == tokenHash && s.RevokedAt == nil {
			s.RevokedAt = &now
		}
	}
	return nil
}

func newTestService() *auth.Service {
	return &auth.Service{
		Users:      &fakeUserRepo{},
		Sessions:   &fakeSessionRepo{},
		SessionTTL: time.Hour,
	}
}

func TestRegisterAndLogin(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	reg, err := svc.Register(ctx, auth.RegisterInput{
		Username: "korn",
		Email:    "korn@example.com",
		Password: "hunter22222",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if reg.User.Username != "korn" {
		t.Fatalf("expected username korn, got %s", reg.User.Username)
	}
	if reg.Token == "" {
		t.Fatal("expected non-empty session token")
	}

	// Login by username.
	byUsername, err := svc.Login(ctx, auth.LoginInput{Identifier: "korn", Password: "hunter22222"})
	if err != nil {
		t.Fatalf("Login by username: %v", err)
	}
	if byUsername.User.ID != reg.User.ID {
		t.Fatal("expected login to resolve the same user")
	}

	// Login by email, case-insensitive.
	byEmail, err := svc.Login(ctx, auth.LoginInput{Identifier: "KORN@EXAMPLE.COM", Password: "hunter22222"})
	if err != nil {
		t.Fatalf("Login by email: %v", err)
	}
	if byEmail.User.ID != reg.User.ID {
		t.Fatal("expected login to resolve the same user")
	}
}

func TestRegisterRejectsDuplicateUsernameOrEmail(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	if _, err := svc.Register(ctx, auth.RegisterInput{Username: "korn", Email: "korn@example.com", Password: "hunter22222"}); err != nil {
		t.Fatalf("first Register: %v", err)
	}

	_, err := svc.Register(ctx, auth.RegisterInput{Username: "korn", Email: "other@example.com", Password: "hunter22222"})
	if !errors.Is(err, user.ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate for duplicate username, got %v", err)
	}

	_, err = svc.Register(ctx, auth.RegisterInput{Username: "someoneelse", Email: "korn@example.com", Password: "hunter22222"})
	if !errors.Is(err, user.ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate for duplicate email, got %v", err)
	}
}

func TestRegisterRejectsInvalidInput(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	cases := []struct {
		name  string
		input auth.RegisterInput
	}{
		{"short username", auth.RegisterInput{Username: "ab", Email: "a@example.com", Password: "hunter22222"}},
		{"invalid email", auth.RegisterInput{Username: "validname", Email: "not-an-email", Password: "hunter22222"}},
		{"short password", auth.RegisterInput{Username: "validname", Email: "a@example.com", Password: "short"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var validationErr *auth.ValidationError
			_, err := svc.Register(ctx, tc.input)
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected ValidationError, got %v", err)
			}
		})
	}
}

func TestLoginRejectsWrongPasswordOrUnknownIdentifier(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	if _, err := svc.Register(ctx, auth.RegisterInput{Username: "korn", Email: "korn@example.com", Password: "hunter22222"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if _, err := svc.Login(ctx, auth.LoginInput{Identifier: "korn", Password: "wrongpassword"}); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials for wrong password, got %v", err)
	}

	if _, err := svc.Login(ctx, auth.LoginInput{Identifier: "nobody", Password: "hunter22222"}); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials for unknown identifier, got %v", err)
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	reg, err := svc.Register(ctx, auth.RegisterInput{Username: "korn", Email: "korn@example.com", Password: "hunter22222"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if _, err := svc.CurrentUser(ctx, reg.Token); err != nil {
		t.Fatalf("expected active session before logout, got %v", err)
	}

	if err := svc.Logout(ctx, reg.Token); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	if _, err := svc.CurrentUser(ctx, reg.Token); !errors.Is(err, auth.ErrSessionInvalid) {
		t.Fatalf("expected ErrSessionInvalid after logout, got %v", err)
	}
}

func TestCurrentUserRejectsUnknownToken(t *testing.T) {
	svc := newTestService()
	if _, err := svc.CurrentUser(context.Background(), "not-a-real-token"); !errors.Is(err, auth.ErrSessionInvalid) {
		t.Fatalf("expected ErrSessionInvalid, got %v", err)
	}
}
