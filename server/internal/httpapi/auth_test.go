package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/auth"
	"github.com/itswskkk/KMJG-Hub/server/internal/chat"
	"github.com/itswskkk/KMJG-Hub/server/internal/directmessage"
	"github.com/itswskkk/KMJG-Hub/server/internal/friend"
	"github.com/itswskkk/KMJG-Hub/server/internal/friend/friendtest"
	"github.com/itswskkk/KMJG-Hub/server/internal/github"
	"github.com/itswskkk/KMJG-Hub/server/internal/github/githubtest"
	"github.com/itswskkk/KMJG-Hub/server/internal/httpapi"
	"github.com/itswskkk/KMJG-Hub/server/internal/invitation"
	"github.com/itswskkk/KMJG-Hub/server/internal/notification"
	"github.com/itswskkk/KMJG-Hub/server/internal/notification/notificationtest"
	"github.com/itswskkk/KMJG-Hub/server/internal/presence"
	"github.com/itswskkk/KMJG-Hub/server/internal/profile"
	"github.com/itswskkk/KMJG-Hub/server/internal/project"
	"github.com/itswskkk/KMJG-Hub/server/internal/realtime"
	"github.com/itswskkk/KMJG-Hub/server/internal/session"
	"github.com/itswskkk/KMJG-Hub/server/internal/user"
)

// In-memory fakes mirroring internal/auth's, kept local to this package
// because the real fakes live in an internal _test.go file and are not
// exported. This lets handler tests exercise the full HTTP contract
// (status codes, JSON shape, headers) without a live PostgreSQL instance.

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

// usernameFor lets fakeProjectRepo resolve a member's username the way the
// real ProjectRepository joins against the users table.
func (f *fakeUserRepo) usernameFor(id string) string {
	u, err := f.GetByID(context.Background(), id)
	if err != nil {
		return ""
	}
	return u.Username
}

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

// newTestRouterWithHandlers is like newTestRouter but also returns the
// wired Handlers, for tests (e.g. WebSocket tests) that need direct access
// to the Realtime hub or Presence service alongside the HTTP router.
func newTestRouterWithHandlers() (http.Handler, *httpapi.Handlers, *fakeProjectRepo) {
	users := &fakeUserRepo{}
	authSvc := &auth.Service{
		Users:      users,
		Sessions:   &fakeSessionRepo{},
		SessionTTL: time.Hour,
	}
	projectRepo := newFakeProjectRepo(users)
	projectSvc := &project.Service{Repo: projectRepo}
	hub := realtime.NewHub(context.Background())
	notificationSvc := &notification.Service{Repo: notificationtest.NewMemory(), Publisher: &notification.RealtimePublisher{Hub: hub}}
	invitationSvc := &invitation.Service{Repo: newFakeInvitationRepo(users, projectRepo), Notifier: notificationSvc}
	presenceSvc := &presence.Service{Membership: projectSvc, Hub: hub}
	chatSvc := &chat.Service{Repo: newFakeChatRepo(users, projectRepo), Membership: projectSvc, Publisher: &chat.RealtimePublisher{Hub: hub}}
	hub.OnUserOnline = presenceSvc.HandleUserOnline
	hub.OnUserOffline = presenceSvc.HandleUserOffline

	profileRepo := newFakeProfileRepo(users)
	profileMembership := newFakeProfileMembership(projectRepo)
	profileSvc := &profile.Service{Repo: profileRepo, Membership: profileMembership}

	friendRepo := friendtest.NewMemory(fakeUserDirectory{users})
	friendSvc := &friend.Service{Repo: friendRepo, Publisher: &friend.RealtimePublisher{Hub: hub}, Notifier: notificationSvc}

	dmSvc := &directmessage.Service{Repo: newFakeDMRepo(users, projectRepo, friendRepo), Publisher: &directmessage.RealtimePublisher{Hub: hub}, Notifier: notificationSvc}

	githubSvc := &github.Service{
		Store: githubtest.NewMemory(), Client: githubtest.NewClient(testWebhookSecret),
		Membership: github.ProjectMembership{Projects: projectSvc},
		Publisher:  &github.RealtimePublisher{Hub: hub}, Notifier: notificationSvc,
		EncryptionKey: make([]byte, github.TokenKeySize),
	}

	handlers := &httpapi.Handlers{Auth: authSvc, Projects: projectSvc, Invitations: invitationSvc, Chat: chatSvc, Profiles: profileSvc, Friends: friendSvc, DirectMessages: dmSvc, Notifications: notificationSvc, GitHub: githubSvc, Realtime: hub, Presence: presenceSvc}
	router := httpapi.NewRouter(handlers, []string{"http://localhost:1420"})
	return router, handlers, projectRepo
}

func newTestRouter() http.Handler {
	router, _, _ := newTestRouterWithHandlers()
	return router
}

// extractToken pulls the session token out of a register/login response
// recorded by doJSON.
func extractToken(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Session struct {
			Token string `json:"token"`
		} `json:"session"`
	}
	decodeJSON(t, rec, &body)
	if body.Session.Token == "" {
		t.Fatalf("expected a session token in response, got: %s", rec.Body.String())
	}
	return body.Session.Token
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, rec.Body.String())
	}
}

func doJSON(t *testing.T, router http.Handler, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestHealthEndpoint(t *testing.T) {
	router := newTestRouter()
	rec := doJSON(t, router, http.MethodGet, "/api/v1/health", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", body["status"])
	}
}

func TestRegisterLoginLogoutFlow(t *testing.T) {
	router := newTestRouter()

	registerBody := map[string]string{
		"username": "korn",
		"email":    "korn@example.com",
		"password": "hunter22222",
	}
	rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register", registerBody, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var registerResp struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
		Session struct {
			Token string `json:"token"`
		} `json:"session"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &registerResp); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	if registerResp.User.Username != "korn" {
		t.Fatalf("expected username korn, got %s", registerResp.User.Username)
	}
	if registerResp.Session.Token == "" {
		t.Fatal("expected non-empty session token")
	}

	// Duplicate registration should be rejected.
	rec = doJSON(t, router, http.MethodPost, "/api/v1/auth/register", registerBody, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate register: expected 409, got %d", rec.Code)
	}

	// Login with correct credentials.
	loginBody := map[string]string{"identifier": "korn", "password": "hunter22222"}
	rec = doJSON(t, router, http.MethodPost, "/api/v1/auth/login", loginBody, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var loginResp struct {
		Session struct {
			Token string `json:"token"`
		} `json:"session"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	token := loginResp.Session.Token

	// Wrong password.
	rec = doJSON(t, router, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"identifier": "korn", "password": "wrongpassword",
	}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: expected 401, got %d", rec.Code)
	}

	// Session check succeeds with a valid token.
	rec = doJSON(t, router, http.MethodGet, "/api/v1/auth/session", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("session check: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Session check fails without a token.
	rec = doJSON(t, router, http.MethodGet, "/api/v1/auth/session", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("session check without token: expected 401, got %d", rec.Code)
	}

	// Logout revokes the session.
	rec = doJSON(t, router, http.MethodPost, "/api/v1/auth/logout", nil, token)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: expected 204, got %d", rec.Code)
	}

	rec = doJSON(t, router, http.MethodGet, "/api/v1/auth/session", nil, token)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("session check after logout: expected 401, got %d", rec.Code)
	}
}

func TestRegisterValidationError(t *testing.T) {
	router := newTestRouter()
	rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"username": "ab",
		"email":    "korn@example.com",
		"password": "hunter22222",
	}, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCORSHeadersForAllowedOrigin(t *testing.T) {
	router := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Origin", "http://localhost:1420")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:1420" {
		t.Fatalf("expected CORS header to echo allowed origin, got %q", got)
	}
}
