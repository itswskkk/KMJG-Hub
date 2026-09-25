// Package invitation implements Server-managed Direct Project Invitations.
package invitation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"
)

const credentialBytes = 32

// NewCredentialSecret creates an unguessable, URL-safe secret. Persistence
// receives HashCredentialSecret(secret), never the secret itself.
func NewCredentialSecret() (string, error) {
	bytes := make([]byte, credentialBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func HashCredentialSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

var (
	ErrNotFound          = errors.New("invitation: not found")
	ErrForbidden         = errors.New("invitation: forbidden")
	ErrConflict          = errors.New("invitation: conflict")
	ErrRecipientNotFound = errors.New("invitation: recipient not found")
	ErrInvalidExpiration = errors.New("invitation: invalid expiration")
	ErrInvalidUseLimit   = errors.New("invitation: invalid use limit")
	ErrExpired           = errors.New("invitation: expired")
	ErrRevoked           = errors.New("invitation: revoked")
	ErrUsesExhausted     = errors.New("invitation: uses exhausted")
	ErrAlreadyMember     = errors.New("invitation: already a member")
)

type Direct struct {
	ID                string     `json:"id"`
	ProjectID         string     `json:"project_id"`
	ProjectName       string     `json:"project_name"`
	InviterUserID     string     `json:"inviter_user_id"`
	InviterUsername   string     `json:"inviter_username"`
	RecipientUserID   string     `json:"recipient_user_id"`
	RecipientUsername string     `json:"recipient_username"`
	CreatedAt         time.Time  `json:"created_at"`
	ExpiresAt         *time.Time `json:"expires_at"`
}

type Credential struct {
	ID              string     `json:"id"`
	ProjectID       string     `json:"project_id"`
	ProjectName     string     `json:"project_name"`
	CreatorUserID   string     `json:"creator_user_id"`
	CreatorUsername string     `json:"creator_username"`
	Secret          string     `json:"code,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	ExpiresAt       *time.Time `json:"expires_at"`
	MaxUses         *int       `json:"max_uses"`
	Uses            int        `json:"uses"`
}

type Repository interface {
	CreateDirect(ctx context.Context, projectID, inviterID, recipientIdentifier string, expiresAt *time.Time) (*Direct, error)
	ListReceived(ctx context.Context, recipientID string) ([]Direct, error)
	ListForProject(ctx context.Context, projectID, actorID string) ([]Direct, error)
	Accept(ctx context.Context, invitationID, recipientID string, now time.Time) (string, error)
	Decline(ctx context.Context, invitationID, recipientID string, now time.Time) error
	Cancel(ctx context.Context, projectID, invitationID, inviterID string, now time.Time) error
	CreateCredential(ctx context.Context, projectID, creatorID, tokenHash string, expiresAt *time.Time, maxUses *int) (*Credential, error)
	ListCredentials(ctx context.Context, projectID, actorID string) ([]Credential, error)
	RevokeCredential(ctx context.Context, projectID, credentialID, actorID string, now time.Time) error
	ConsumeCredential(ctx context.Context, tokenHash, userID string, now time.Time) (string, error)
}

type Service struct {
	Repo     Repository
	Now      func() time.Time
	Notifier Notifier // optional; nil disables notifications
}

// Notifier creates persistent notifications. Satisfied by
// *notification.Service; declared here so this package does not import it.
type Notifier interface {
	Notify(ctx context.Context, userID, eventType string, payload any) error
}

// notify creates a notification best-effort: failures are logged and never
// fail the primary operation. A nil Notifier disables notifications.
func (s *Service) notify(ctx context.Context, userID, eventType string, payload any) {
	if s.Notifier == nil {
		return
	}
	if err := s.Notifier.Notify(ctx, userID, eventType, payload); err != nil {
		slog.Warn("invitation: create notification", "event_type", eventType, "error", err)
	}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func expirationTime(now time.Time, expiration string) (*time.Time, error) {
	if expiration == "never" {
		return nil, nil
	}
	durations := map[string]time.Duration{"1h": time.Hour, "1d": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour}
	duration, ok := durations[expiration]
	if !ok {
		return nil, ErrInvalidExpiration
	}
	value := now.Add(duration)
	return &value, nil
}

func (s *Service) CreateDirect(ctx context.Context, projectID, inviterID, recipient, expiration string) (*Direct, error) {
	expiresAt, err := expirationTime(s.now(), expiration)
	if err != nil {
		return nil, err
	}
	if recipient == "" {
		return nil, ErrRecipientNotFound
	}
	item, err := s.Repo.CreateDirect(ctx, projectID, inviterID, recipient, expiresAt)
	if err != nil {
		return nil, err
	}
	s.notify(ctx, item.RecipientUserID, "project_invitation", map[string]any{
		"project_id":       item.ProjectID,
		"project_name":     item.ProjectName,
		"invitation_id":    item.ID,
		"inviter_id":       item.InviterUserID,
		"inviter_username": item.InviterUsername,
	})
	return item, nil
}

func (s *Service) CreateCredential(ctx context.Context, projectID, creatorID, expiration string, maxUses *int) (*Credential, error) {
	if maxUses != nil && *maxUses <= 0 {
		return nil, ErrInvalidUseLimit
	}
	expiresAt, err := expirationTime(s.now(), expiration)
	if err != nil {
		return nil, err
	}
	secret, err := NewCredentialSecret()
	if err != nil {
		return nil, err
	}
	item, err := s.Repo.CreateCredential(ctx, projectID, creatorID, HashCredentialSecret(secret), expiresAt, maxUses)
	if err != nil {
		return nil, err
	}
	item.Secret = secret
	return item, nil
}

func (s *Service) ListCredentials(ctx context.Context, projectID, actorID string) ([]Credential, error) {
	return s.Repo.ListCredentials(ctx, projectID, actorID)
}

func (s *Service) RevokeCredential(ctx context.Context, projectID, credentialID, actorID string) error {
	return s.Repo.RevokeCredential(ctx, projectID, credentialID, actorID, s.now())
}

func (s *Service) JoinWithCredential(ctx context.Context, input, userID string) (string, error) {
	secret := strings.TrimSpace(input)
	if parsed, err := url.Parse(secret); err == nil && parsed.Scheme != "" {
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		secret = parts[len(parts)-1]
	}
	if secret == "" {
		return "", ErrNotFound
	}
	return s.Repo.ConsumeCredential(ctx, HashCredentialSecret(secret), userID, s.now())
}

func (s *Service) ListReceived(ctx context.Context, recipientID string) ([]Direct, error) {
	return s.Repo.ListReceived(ctx, recipientID)
}

func (s *Service) ListForProject(ctx context.Context, projectID, actorID string) ([]Direct, error) {
	return s.Repo.ListForProject(ctx, projectID, actorID)
}

func (s *Service) Accept(ctx context.Context, invitationID, recipientID string) (string, error) {
	return s.Repo.Accept(ctx, invitationID, recipientID, s.now())
}

func (s *Service) Decline(ctx context.Context, invitationID, recipientID string) error {
	return s.Repo.Decline(ctx, invitationID, recipientID, s.now())
}

func (s *Service) Cancel(ctx context.Context, projectID, invitationID, inviterID string) error {
	return s.Repo.Cancel(ctx, projectID, invitationID, inviterID, s.now())
}
