// Package githubtest provides in-memory fakes of github.Store and
// github.Client for service and HTTP tests (no network, no database).
package githubtest

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/github"
)

type identity struct {
	githubUserID int64
	login        string
	token        []byte
	connectedAt  time.Time
}

// Memory is an in-memory github.Store.
type Memory struct {
	mu         sync.Mutex
	identities map[string]identity // by KMJG Hub user ID
	repos      map[string]github.Repository
	nextID     int
}

var _ github.Store = (*Memory)(nil)

func NewMemory() *Memory {
	return &Memory{identities: map[string]identity{}, repos: map[string]github.Repository{}}
}

func (m *Memory) SaveIdentity(_ context.Context, userID string, githubUserID int64, login string, encryptedToken []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for uid, id := range m.identities {
		if uid != userID && id.githubUserID == githubUserID {
			return github.ErrIdentityInUse
		}
	}
	m.identities[userID] = identity{githubUserID, login, append([]byte(nil), encryptedToken...), time.Now().UTC()}
	return nil
}

func (m *Memory) GetIdentity(_ context.Context, userID string) (*github.Identity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.identities[userID]
	if !ok {
		return nil, github.ErrNotConnected
	}
	return &github.Identity{UserID: userID, GitHubLogin: id.login, ConnectedAt: id.connectedAt}, nil
}

// StoredToken exposes the raw stored ciphertext for assertions.
func (m *Memory) StoredToken(userID string) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.identities[userID].token
}

func (m *Memory) GetEncryptedAccessToken(_ context.Context, userID string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.identities[userID]
	if !ok {
		return nil, github.ErrNotConnected
	}
	return id.token, nil
}

func (m *Memory) DisconnectIdentity(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.identities[userID]; !ok {
		return github.ErrNotConnected
	}
	delete(m.identities, userID)
	return nil
}

func (m *Memory) ConnectRepository(_ context.Context, projectID, connectedByUserID string, repo github.AvailableRepository) (*github.Repository, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.repos[projectID]; ok {
		return nil, github.ErrAlreadyConnected
	}
	m.nextID++
	r := github.Repository{
		ID: fmt.Sprintf("repo-%d", m.nextID), ProjectID: projectID, Provider: "github",
		ExternalID: repo.ExternalID, OwnerLogin: repo.OwnerLogin, Name: repo.Name,
		HTMLURL: repo.HTMLURL, DefaultBranch: repo.DefaultBranch,
		ConnectedByUserID: connectedByUserID, ConnectedAt: time.Now().UTC(),
		NotifyAllMembers: true, NotifyAllBranches: true,
	}
	m.repos[projectID] = r
	return &r, nil
}

func (m *Memory) GetRepository(_ context.Context, projectID string) (*github.Repository, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.repos[projectID]
	if !ok {
		return nil, github.ErrNotFound
	}
	return &r, nil
}

func (m *Memory) ListRepositoriesByExternalID(_ context.Context, externalID int64) ([]github.Repository, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]github.Repository, 0)
	for _, r := range m.repos {
		if r.ExternalID == externalID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *Memory) DisconnectRepository(_ context.Context, projectID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.repos[projectID]; !ok {
		return github.ErrNotFound
	}
	delete(m.repos, projectID)
	return nil
}

func (m *Memory) UpdateNotificationConfig(_ context.Context, projectID string, postToChat, allMembers, allBranches bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.repos[projectID]
	if !ok {
		return github.ErrNotFound
	}
	r.PostPushesToChat, r.NotifyAllMembers, r.NotifyAllBranches = postToChat, allMembers, allBranches
	m.repos[projectID] = r
	return nil
}

// Account is one fake GitHub account reachable through Client.
type Account struct {
	ID    int64
	Login string
	Repos []github.AvailableRepository
}

// Client is a fake github.Client. Codes map to accounts; the issued access
// token is "token-for-<login>". Webhook signatures are checked with the
// real HMAC verification against WebhookSecret.
type Client struct {
	mu            sync.Mutex
	Codes         map[string]Account
	WebhookSecret string
}

var _ github.Client = (*Client)(nil)

func NewClient(webhookSecret string) *Client {
	return &Client{Codes: map[string]Account{}, WebhookSecret: webhookSecret}
}

// AddAccount makes code exchangeable for account.
func (c *Client) AddAccount(code string, account Account) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Codes[code] = account
}

func (c *Client) AuthURL(state string) string {
	return "https://github.example/login/oauth/authorize?state=" + url.QueryEscape(state)
}

func (c *Client) ExchangeCode(_ context.Context, code string) (string, int64, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a, ok := c.Codes[code]
	if !ok {
		return "", 0, "", fmt.Errorf("%w: unknown code", github.ErrInvalidOAuthState)
	}
	return "token-for-" + a.Login, a.ID, a.Login, nil
}

func (c *Client) ListRepositories(_ context.Context, accessToken string) ([]github.AvailableRepository, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, a := range c.Codes {
		if accessToken == "token-for-"+a.Login {
			return append([]github.AvailableRepository(nil), a.Repos...), nil
		}
	}
	return nil, github.ErrTokenRejected
}

func (c *Client) VerifyWebhookSignature(payload []byte, signatureHeader string) bool {
	return github.VerifySignature(c.WebhookSecret, payload, signatureHeader)
}

// Sign returns the X-Hub-Signature-256 header value for payload.
func Sign(secret string, payload []byte) string {
	return github.SignPayload(secret, payload)
}
