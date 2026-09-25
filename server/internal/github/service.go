package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// EventGitPush is the notification event type for Git pushes (matches
	// internal/notification.EventGitPush).
	EventGitPush = "git_push"

	maxCommitSummaries   = 20
	maxCommitSummaryRune = 200
)

// Service implements GitHub account connection, Project repository
// connection, and push-webhook handling. A nil Client means the deployment
// has not configured a GitHub OAuth App: every operation that needs GitHub
// then returns ErrNotConfigured, while reading/removing already-stored
// connections keeps working.
type Service struct {
	Store      Store
	Client     Client
	Membership Membership
	Publisher  Publisher  // optional
	Notifier   Notifier   // optional
	ChatPoster ChatPoster // optional

	// EncryptionKey (32 bytes) encrypts GitHub access tokens at rest and
	// derives the OAuth state signing key.
	EncryptionKey []byte

	// Now overrides the clock in tests.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Configured reports whether GitHub OAuth/API features are available.
func (s *Service) Configured() bool {
	return s.Client != nil && len(s.EncryptionKey) == TokenKeySize
}

func (s *Service) requireConfigured() error {
	if !s.Configured() {
		return ErrNotConfigured
	}
	return nil
}

// stateKey derives a signing key distinct from the token encryption key.
func (s *Service) stateKey() []byte {
	mac := hmac.New(sha256.New, s.EncryptionKey)
	mac.Write([]byte("kmjg-github-oauth-state-key"))
	return mac.Sum(nil)
}

// GetIdentity returns userID's connected GitHub account, or ErrNotConnected.
func (s *Service) GetIdentity(ctx context.Context, userID string) (*Identity, error) {
	return s.Store.GetIdentity(ctx, userID)
}

// StartOAuth begins connecting userID's GitHub account, returning the
// GitHub authorization URL and the signed state it embeds. The state binds
// the eventual callback to userID and expires after OAuthStateTTL, so no
// server-side storage is needed.
func (s *Service) StartOAuth(ctx context.Context, userID string) (string, string, error) {
	if err := s.requireConfigured(); err != nil {
		return "", "", err
	}
	state, err := newOAuthState(s.stateKey(), userID, s.now())
	if err != nil {
		return "", "", err
	}
	return s.Client.AuthURL(state), state, nil
}

// CompleteOAuth finishes the OAuth flow started by StartOAuth. The KMJG Hub
// user is taken from the verified state (GitHub's browser redirect carries
// no KMJG Hub session); the GitHub account is then linked to that existing
// KMJG Hub account — it never replaces it (docs/PRD.md "GitHub
// Authentication").
func (s *Service) CompleteOAuth(ctx context.Context, code, state string) (*Identity, error) {
	if err := s.requireConfigured(); err != nil {
		return nil, err
	}
	userID, err := validateOAuthState(s.stateKey(), state, s.now())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(code) == "" {
		return nil, &ValidationError{Field: "code", Message: "Authorization code is required"}
	}
	token, githubUserID, login, err := s.Client.ExchangeCode(ctx, code)
	if err != nil {
		return nil, err
	}
	encrypted, err := EncryptToken(token, s.EncryptionKey)
	if err != nil {
		return nil, err
	}
	if err := s.Store.SaveIdentity(ctx, userID, githubUserID, login, encrypted); err != nil {
		return nil, err
	}
	return s.Store.GetIdentity(ctx, userID)
}

// DisconnectAccount unlinks userID's GitHub account and discards its token.
// Project repository connections the user made are unaffected.
func (s *Service) DisconnectAccount(ctx context.Context, userID string) error {
	return s.Store.DisconnectIdentity(ctx, userID)
}

func (s *Service) accessToken(ctx context.Context, userID string) (string, error) {
	encrypted, err := s.Store.GetEncryptedAccessToken(ctx, userID)
	if err != nil {
		return "", err
	}
	token, err := DecryptToken(encrypted, s.EncryptionKey)
	if err != nil {
		// Typically the encryption key changed (e.g. the dev-only random
		// key after a restart): the user must reconnect.
		slog.Warn("github: stored access token cannot be decrypted", "user_id", userID, "error", err)
		return "", ErrNotConnected
	}
	return token, nil
}

// ListAvailableRepositories returns the repositories userID's connected
// GitHub account can access. Access is decided entirely by GitHub.
func (s *Service) ListAvailableRepositories(ctx context.Context, userID string) ([]AvailableRepository, error) {
	if err := s.requireConfigured(); err != nil {
		return nil, err
	}
	token, err := s.accessToken(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.Client.ListRepositories(ctx, token)
}

func (s *Service) viewerRole(ctx context.Context, projectID, userID string) (string, error) {
	role, err := s.Membership.ViewerRole(ctx, projectID, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	return role, nil
}

func canManageRepository(role string) bool {
	return role == RoleOwner || role == RoleAdmin
}

// ConnectRepository connects the GitHub repository externalID to
// projectID. Only the Project Owner or an Admin may do so, the Project must
// not already have a repository (v1: at most one), and the repository must
// be accessible to the acting user's own GitHub account — KMJG Hub never
// grants repository access GitHub has not.
func (s *Service) ConnectRepository(ctx context.Context, userID, projectID string, externalID int64) (*Repository, error) {
	role, err := s.viewerRole(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	if !canManageRepository(role) {
		return nil, ErrForbidden
	}
	if externalID <= 0 {
		return nil, &ValidationError{Field: "external_repo_id", Message: "A repository must be selected"}
	}
	if err := s.requireConfigured(); err != nil {
		return nil, err
	}
	if _, err := s.Store.GetRepository(ctx, projectID); err == nil {
		return nil, ErrAlreadyConnected
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	available, err := s.ListAvailableRepositories(ctx, userID)
	if err != nil {
		return nil, err
	}
	var chosen *AvailableRepository
	for i := range available {
		if available[i].ExternalID == externalID {
			chosen = &available[i]
			break
		}
	}
	if chosen == nil {
		return nil, ErrRepositoryUnavailable
	}
	if chosen.DefaultBranch == "" {
		chosen.DefaultBranch = "main"
	}

	repo, err := s.Store.ConnectRepository(ctx, projectID, userID, *chosen)
	if err != nil {
		return nil, err
	}
	s.forEachMember(ctx, projectID, func(memberID string) {
		if s.Publisher != nil {
			s.Publisher.PublishRepositoryConnected(memberID, *repo)
		}
	})
	return repo, nil
}

// GetRepository returns projectID's connected repository to any Project
// member, or ErrNotFound if there is none (or userID is not a member).
func (s *Service) GetRepository(ctx context.Context, userID, projectID string) (*Repository, error) {
	if _, err := s.viewerRole(ctx, projectID, userID); err != nil {
		return nil, err
	}
	return s.Store.GetRepository(ctx, projectID)
}

// FindRepository is GetRepository for callers that treat "no repository
// connected" as a normal state: it returns (nil, nil) in that case, while a
// non-member still gets ErrNotFound.
func (s *Service) FindRepository(ctx context.Context, userID, projectID string) (*Repository, error) {
	if _, err := s.viewerRole(ctx, projectID, userID); err != nil {
		return nil, err
	}
	repo, err := s.Store.GetRepository(ctx, projectID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return repo, err
}

// DisconnectRepository removes projectID's repository connection (Owner or
// Admin only). It does not touch the repository on GitHub.
func (s *Service) DisconnectRepository(ctx context.Context, userID, projectID string) error {
	role, err := s.viewerRole(ctx, projectID, userID)
	if err != nil {
		return err
	}
	if !canManageRepository(role) {
		return ErrForbidden
	}
	if err := s.Store.DisconnectRepository(ctx, projectID); err != nil {
		return err
	}
	s.forEachMember(ctx, projectID, func(memberID string) {
		if s.Publisher != nil {
			s.Publisher.PublishRepositoryDisconnected(memberID, projectID)
		}
	})
	return nil
}

// UpdateNotificationConfig sets projectID's Git push notification options.
// docs/PRD.md "Git Push Notifications": "The Project Owner may configure"
// these, so this is Owner-only (Admins are refused).
func (s *Service) UpdateNotificationConfig(ctx context.Context, userID, projectID string, postToChat, allMembers, allBranches bool) (*Repository, error) {
	role, err := s.viewerRole(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	if role != RoleOwner {
		return nil, ErrForbidden
	}
	if err := s.Store.UpdateNotificationConfig(ctx, projectID, postToChat, allMembers, allBranches); err != nil {
		return nil, err
	}
	return s.Store.GetRepository(ctx, projectID)
}

// pushPayload is the subset of GitHub's push webhook payload KMJG Hub uses.
type pushPayload struct {
	Ref     string `json:"ref"`
	Deleted bool   `json:"deleted"`
	Pusher  struct {
		Name string `json:"name"`
	} `json:"pusher"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
	Repository struct {
		ID int64 `json:"id"`
	} `json:"repository"`
	Commits []struct {
		Message string `json:"message"`
	} `json:"commits"`
}

// HandleWebhook processes one GitHub webhook delivery. The signature is
// verified before the payload is parsed at all. Only "push" events to
// branches are acted on; everything else (e.g. "ping", tag pushes, branch
// deletions, repositories no Project has connected) is accepted and
// ignored so GitHub does not keep retrying it.
func (s *Service) HandleWebhook(ctx context.Context, eventType string, payload []byte, signatureHeader string) error {
	if err := s.requireConfigured(); err != nil {
		return err
	}
	if !s.Client.VerifyWebhookSignature(payload, signatureHeader) {
		return ErrInvalidSignature
	}
	if eventType != "push" {
		return nil
	}
	event, ok, err := parsePushEvent(payload)
	if err != nil || !ok {
		return err
	}

	repos, err := s.Store.ListRepositoriesByExternalID(ctx, event.RepositoryExternalID)
	if err != nil {
		return err
	}
	for _, repo := range repos {
		if !repo.NotifyAllBranches && event.Branch != repo.DefaultBranch {
			continue
		}
		s.deliverPush(ctx, repo, event)
	}
	return nil
}

func parsePushEvent(payload []byte) (PushEvent, bool, error) {
	var p pushPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return PushEvent{}, false, &ValidationError{Field: "payload", Message: "Malformed push payload"}
	}
	branch, isBranch := strings.CutPrefix(p.Ref, "refs/heads/")
	if !isBranch || branch == "" || p.Deleted || p.Repository.ID == 0 {
		return PushEvent{}, false, nil
	}
	pusher := p.Sender.Login
	if pusher == "" {
		pusher = p.Pusher.Name
	}
	event := PushEvent{
		RepositoryExternalID: p.Repository.ID,
		Branch:               branch,
		PusherLogin:          pusher,
		CommitCount:          len(p.Commits),
		CommitSummaries:      make([]string, 0, min(len(p.Commits), maxCommitSummaries)),
	}
	for i, c := range p.Commits {
		if i == maxCommitSummaries {
			break
		}
		event.CommitSummaries = append(event.CommitSummaries, commitSummary(c.Message))
	}
	return event, true, nil
}

func commitSummary(message string) string {
	line, _, _ := strings.Cut(message, "\n")
	line = strings.TrimSpace(line)
	if utf8.RuneCountInString(line) > maxCommitSummaryRune {
		runes := []rune(line)
		line = string(runes[:maxCommitSummaryRune-1]) + "…"
	}
	return line
}

// deliverPush fans a push out to repo's Project members: every member gets
// the real-time Git activity event ("View Git activity" is a Member
// capability); persistent notifications go to all members only when the
// Owner has enabled NotifyAllMembers; and when PostPushesToChat is enabled
// the push is posted once to Project Chat as Git activity (a system
// message, not a user-authored one). Branch filtering (NotifyAllBranches)
// has already been applied by the caller.
func (s *Service) deliverPush(ctx context.Context, repo Repository, event PushEvent) {
	if repo.PostPushesToChat && s.ChatPoster != nil {
		if err := s.ChatPoster.PostGitActivity(ctx, repo.ProjectID, FormatPushMessage(repo, event)); err != nil {
			slog.Error("github: post push to project chat failed", "project_id", repo.ProjectID, "error", err)
		}
	}
	payload := map[string]any{
		"project_id":       repo.ProjectID,
		"repository":       repo.OwnerLogin + "/" + repo.Name,
		"branch":           event.Branch,
		"pusher_login":     event.PusherLogin,
		"commit_count":     event.CommitCount,
		"commit_summaries": event.CommitSummaries,
	}
	s.forEachMember(ctx, repo.ProjectID, func(memberID string) {
		if s.Publisher != nil {
			s.Publisher.PublishPush(memberID, repo.ProjectID, event)
		}
		if s.Notifier != nil && repo.NotifyAllMembers {
			if err := s.Notifier.Notify(ctx, memberID, EventGitPush, payload); err != nil {
				slog.Error("github: git push notification failed", "user_id", memberID, "error", err)
			}
		}
	})
}

// maxChatCommitSummaries bounds how many commit summaries a Project Chat
// Git activity message lists; the count line always reports the total.
const maxChatCommitSummaries = 10

// FormatPushMessage renders a push as a Project Chat Git activity message
// with the information docs/PRD.md "Git Push Notifications" lists: who
// pushed, the branch, the number of commits, and commit summaries, e.g.
//
//	alice-gh pushed 2 commits → main (alice-gh/hub)
//	• Add feature
//	• Fix bug
func FormatPushMessage(repo Repository, event PushEvent) string {
	pusher := event.PusherLogin
	if pusher == "" {
		pusher = "Someone"
	}
	var b strings.Builder
	switch event.CommitCount {
	case 0:
		fmt.Fprintf(&b, "%s pushed → %s", pusher, event.Branch)
	case 1:
		fmt.Fprintf(&b, "%s pushed 1 commit → %s", pusher, event.Branch)
	default:
		fmt.Fprintf(&b, "%s pushed %d commits → %s", pusher, event.CommitCount, event.Branch)
	}
	if repo.OwnerLogin != "" && repo.Name != "" {
		fmt.Fprintf(&b, " (%s/%s)", repo.OwnerLogin, repo.Name)
	}
	shown := 0
	for _, summary := range event.CommitSummaries {
		if shown == maxChatCommitSummaries {
			break
		}
		if summary == "" {
			continue
		}
		b.WriteString("\n• ")
		b.WriteString(summary)
		shown++
	}
	if rest := event.CommitCount - shown; shown > 0 && rest > 0 {
		fmt.Fprintf(&b, "\n…and %d more", rest)
	}
	return b.String()
}

// forEachMember runs fn for each current member of projectID. Delivery is
// best-effort: a membership lookup failure is logged, not returned, since
// the triggering change has already been committed.
func (s *Service) forEachMember(ctx context.Context, projectID string, fn func(userID string)) {
	if s.Membership == nil {
		return
	}
	members, err := s.Membership.MemberUserIDs(ctx, projectID)
	if err != nil {
		slog.Error("github: list project members", "project_id", projectID, "error", err)
		return
	}
	for _, id := range members {
		fn(id)
	}
}
