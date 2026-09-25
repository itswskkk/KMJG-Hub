// Package github implements KMJG Hub's optional GitHub integration:
// connecting a user's GitHub identity for authentication, and connecting a
// Project to at most one GitHub repository, per docs/PRD.md "GitHub
// Authentication" and "Project Git Integration". KMJG Hub Project membership
// and GitHub repository permissions are deliberately kept separate — this
// package never bypasses or overrides GitHub's own permission model.
package github

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound          = errors.New("github: not found")
	ErrForbidden         = errors.New("github: forbidden")
	ErrNotConnected      = errors.New("github: user has not connected a GitHub account")
	ErrAlreadyConnected  = errors.New("github: project already has a connected repository")
	ErrInvalidOAuthState = errors.New("github: invalid or expired oauth state")
	// ErrNotConfigured is returned by every GitHub operation when the
	// deployment has not configured a GitHub OAuth App (see
	// internal/config KMJG_GITHUB_* variables).
	ErrNotConfigured = errors.New("github: integration is not configured on this server")
	// ErrIdentityInUse is returned when a GitHub account is already linked
	// to a different KMJG Hub user.
	ErrIdentityInUse = errors.New("github: this GitHub account is already linked to another user")
	// ErrRepositoryUnavailable is returned when the requested repository is
	// not accessible to the user's connected GitHub account.
	ErrRepositoryUnavailable = errors.New("github: repository is not accessible to your GitHub account")
	// ErrInvalidSignature is returned when a webhook delivery fails HMAC
	// signature verification.
	ErrInvalidSignature = errors.New("github: invalid webhook signature")
)

// Project role names as reported by Membership.ViewerRole. They mirror
// internal/project's Role values without importing that package.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// Identity is a user's connected GitHub account.
type Identity struct {
	UserID      string
	GitHubLogin string
	ConnectedAt time.Time
}

// Repository (Git repository, not the persistence Repository interface
// below) is the GitHub repo connected to one KMJG Hub Project.
type Repository struct {
	ID                string
	ProjectID         string
	Provider          string // always "github" in v1; kept provider-neutral for future providers
	ExternalID        int64  // the provider's repository ID (GitHub repository id)
	OwnerLogin        string
	Name              string
	HTMLURL           string
	DefaultBranch     string
	ConnectedByUserID string
	ConnectedAt       time.Time

	// Git push notification configuration, per docs/PRD.md "Git Push Notifications".
	PostPushesToChat  bool
	NotifyAllMembers  bool
	NotifyAllBranches bool
}

// AvailableRepository is one repository the connected GitHub account can
// access, as returned by the GitHub API listing (before connection).
type AvailableRepository struct {
	ExternalID    int64
	OwnerLogin    string
	Name          string
	HTMLURL       string
	Private       bool
	DefaultBranch string
}

// PushEvent is the KMJG Hub-relevant subset of a GitHub push webhook payload.
type PushEvent struct {
	RepositoryExternalID int64
	Branch               string
	PusherLogin          string
	CommitCount          int
	CommitSummaries      []string // short, e.g. first line of each commit message
}

// Store persists GitHub identities, repository connections, and app config.
type Store interface {
	// SaveIdentity links a GitHub account to a KMJG Hub user, storing the
	// access token encrypted. Returns ErrIdentityInUse when githubUserID is
	// already linked to a different user.
	SaveIdentity(ctx context.Context, userID string, githubUserID int64, login string, encryptedToken []byte) error

	// GetIdentity returns userID's connected GitHub identity, or ErrNotConnected.
	GetIdentity(ctx context.Context, userID string) (*Identity, error)

	// GetEncryptedAccessToken returns userID's stored (still encrypted)
	// GitHub access token, or ErrNotConnected. Decryption is the service's
	// job so the key never reaches the persistence layer.
	GetEncryptedAccessToken(ctx context.Context, userID string) ([]byte, error)

	// DisconnectIdentity removes userID's GitHub account link.
	DisconnectIdentity(ctx context.Context, userID string) error

	// ConnectRepository associates a GitHub repository with projectID,
	// returning ErrAlreadyConnected if projectID already has one.
	// Authorization (Owner/Admin) must be enforced by the caller/service;
	// implementations must still enforce "at most one repository per
	// Project" via the unique project_id constraint.
	ConnectRepository(ctx context.Context, projectID, connectedByUserID string, repo AvailableRepository) (*Repository, error)

	// GetRepository returns the repository connected to projectID, or ErrNotFound.
	GetRepository(ctx context.Context, projectID string) (*Repository, error)

	// ListRepositoriesByExternalID returns every Project connection to the
	// repository with GitHub's repository ID externalID (used to route
	// incoming webhooks; several Projects may connect the same repository).
	// An empty result is not an error.
	ListRepositoriesByExternalID(ctx context.Context, externalID int64) ([]Repository, error)

	// DisconnectRepository removes projectID's repository connection, or
	// returns ErrNotFound.
	DisconnectRepository(ctx context.Context, projectID string) error

	// UpdateNotificationConfig updates a connected repository's push
	// notification settings, or returns ErrNotFound.
	UpdateNotificationConfig(ctx context.Context, projectID string, postToChat, allMembers, allBranches bool) error
}

// Client wraps calls to the GitHub API (OAuth + REST). A real implementation
// talks to api.github.com (or a self-hosted GitHub Enterprise base URL); a
// fake implementation backs tests.
type Client interface {
	// AuthURL returns the GitHub OAuth authorization URL for the given
	// opaque state token.
	AuthURL(state string) string

	// ExchangeCode exchanges an OAuth callback code for an access token and
	// the authenticated GitHub user's identity.
	ExchangeCode(ctx context.Context, code string) (accessToken string, githubUserID int64, login string, err error)

	// ListRepositories returns the repositories accessible to accessToken.
	ListRepositories(ctx context.Context, accessToken string) ([]AvailableRepository, error)

	// VerifyWebhookSignature checks a webhook payload's HMAC signature
	// against the configured webhook secret.
	VerifyWebhookSignature(payload []byte, signatureHeader string) bool
}

// Membership supplies Project role information so the service can enforce
// "Owner or Admin may connect/disconnect a repository" without depending on
// the project package directly (narrow interface, same pattern as
// internal/chat.Membership).
type Membership interface {
	// ViewerRole returns userID's role in projectID (RoleOwner, RoleAdmin
	// or RoleMember). It must return an error matching ErrNotFound when
	// userID is not a member (or the Project does not exist), so
	// non-members cannot distinguish the two.
	ViewerRole(ctx context.Context, projectID, userID string) (string, error)

	// MemberUserIDs returns projectID's current members, for push
	// notification broadcast.
	MemberUserIDs(ctx context.Context, projectID string) ([]string, error)
}

// Publisher delivers real-time Git activity events to one user (the
// service fans out over Server-owned Project membership, same pattern as
// internal/chat.Publisher).
type Publisher interface {
	PublishRepositoryConnected(userID string, repo Repository)
	PublishRepositoryDisconnected(userID, projectID string)
	PublishPush(userID, projectID string, event PushEvent)
}

// Notifier creates persistent per-user notifications (satisfied by
// *notification.Service without importing it).
type Notifier interface {
	Notify(ctx context.Context, userID, eventType string, payload any) error
}

// ValidationError is safe to expose to API clients.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }
