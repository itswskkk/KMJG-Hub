package github_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/github"
	"github.com/itswskkk/KMJG-Hub/server/internal/github/githubtest"
)

const webhookSecret = "whsec-test"

type fakeMembership struct {
	roles map[string]map[string]string // projectID -> userID -> role
}

func (f *fakeMembership) ViewerRole(_ context.Context, projectID, userID string) (string, error) {
	role, ok := f.roles[projectID][userID]
	if !ok {
		return "", github.ErrNotFound
	}
	return role, nil
}

func (f *fakeMembership) MemberUserIDs(_ context.Context, projectID string) ([]string, error) {
	ids := make([]string, 0)
	for id := range f.roles[projectID] {
		ids = append(ids, id)
	}
	return ids, nil
}

type recorded struct {
	mu            sync.Mutex
	connected     []string
	disconnected  []string
	pushes        []string // userID:branch
	notifications []string // userID:eventType
	lastPayload   any
}

func (r *recorded) PublishRepositoryConnected(userID string, _ github.Repository) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connected = append(r.connected, userID)
}
func (r *recorded) PublishRepositoryDisconnected(userID, _ string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.disconnected = append(r.disconnected, userID)
}
func (r *recorded) PublishPush(userID, _ string, e github.PushEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pushes = append(r.pushes, userID+":"+e.Branch)
}
func (r *recorded) Notify(_ context.Context, userID, eventType string, payload any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notifications = append(r.notifications, userID+":"+eventType)
	r.lastPayload = payload
	return nil
}

type fixture struct {
	svc    *github.Service
	store  *githubtest.Memory
	client *githubtest.Client
	rec    *recorded
}

var aliceRepo = github.AvailableRepository{ExternalID: 42, OwnerLogin: "alice-gh", Name: "hub", HTMLURL: "https://github.com/alice-gh/hub", DefaultBranch: "main"}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	key := make([]byte, github.TokenKeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	store := githubtest.NewMemory()
	client := githubtest.NewClient(webhookSecret)
	client.AddAccount("code-owner", githubtest.Account{ID: 1, Login: "alice-gh", Repos: []github.AvailableRepository{aliceRepo}})
	client.AddAccount("code-admin", githubtest.Account{ID: 2, Login: "adam-gh", Repos: []github.AvailableRepository{aliceRepo}})
	client.AddAccount("code-member", githubtest.Account{ID: 3, Login: "mia-gh", Repos: []github.AvailableRepository{aliceRepo}})
	rec := &recorded{}
	svc := &github.Service{
		Store: store, Client: client, Publisher: rec, Notifier: rec, EncryptionKey: key,
		Membership: &fakeMembership{roles: map[string]map[string]string{
			"p1": {"owner": github.RoleOwner, "admin": github.RoleAdmin, "member": github.RoleMember},
			"p2": {"owner": github.RoleOwner},
		}},
	}
	return &fixture{svc: svc, store: store, client: client, rec: rec}
}

func (f *fixture) connectAccount(t *testing.T, userID, code string) {
	t.Helper()
	_, state, err := f.svc.StartOAuth(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.CompleteOAuth(context.Background(), code, state); err != nil {
		t.Fatalf("complete oauth for %s: %v", userID, err)
	}
}

func TestOAuthFlowLinksIdentityAndEncryptsToken(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	authURL, state, err := f.svc.StartOAuth(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(authURL)
	if u.Query().Get("state") != state {
		t.Fatalf("auth URL does not carry the state: %s", authURL)
	}
	identity, err := f.svc.CompleteOAuth(ctx, "code-owner", state)
	if err != nil {
		t.Fatal(err)
	}
	if identity.UserID != "owner" || identity.GitHubLogin != "alice-gh" {
		t.Fatalf("identity: %+v", identity)
	}
	stored := f.store.StoredToken("owner")
	if len(stored) == 0 || bytes.Contains(stored, []byte("token-for-alice-gh")) {
		t.Fatalf("token must be stored encrypted, got %q", stored)
	}
	repos, err := f.svc.ListAvailableRepositories(ctx, "owner")
	if err != nil || len(repos) != 1 || repos[0].ExternalID != 42 {
		t.Fatalf("list repos after connect: %+v %v", repos, err)
	}

	if err := f.svc.DisconnectAccount(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ListAvailableRepositories(ctx, "owner"); !errors.Is(err, github.ErrNotConnected) {
		t.Fatalf("after disconnect: %v", err)
	}
}

func TestOAuthStateTamperingRejected(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, state, err := f.svc.StartOAuth(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	encoded, sig, _ := strings.Cut(state, ".")
	// Swap the payload for one naming another user, keeping the signature.
	_, otherState, _ := f.svc.StartOAuth(ctx, "member")
	otherEncoded, _, _ := strings.Cut(otherState, ".")

	flipped := []byte(sig)
	if flipped[0] == 'a' {
		flipped[0] = 'b'
	} else {
		flipped[0] = 'a'
	}
	for name, bad := range map[string]string{
		"empty":            "",
		"garbage":          "not-a-state",
		"no signature":     encoded,
		"bad signature":    encoded + "." + string(flipped),
		"swapped payload":  otherEncoded + "." + sig,
		"other deployment": signedByOtherKey(t),
	} {
		if _, err := f.svc.CompleteOAuth(ctx, "code-owner", bad); !errors.Is(err, github.ErrInvalidOAuthState) {
			t.Errorf("%s: expected ErrInvalidOAuthState, got %v", name, err)
		}
	}
	if _, err := f.store.GetIdentity(ctx, "owner"); !errors.Is(err, github.ErrNotConnected) {
		t.Fatal("identity linked despite tampered state")
	}
}

func signedByOtherKey(t *testing.T) string {
	other := newFixture(t)
	_, state, err := other.svc.StartOAuth(context.Background(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestOAuthStateExpires(t *testing.T) {
	f := newFixture(t)
	start := time.Now()
	f.svc.Now = func() time.Time { return start }
	_, state, err := f.svc.StartOAuth(context.Background(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	f.svc.Now = func() time.Time { return start.Add(github.OAuthStateTTL + time.Second) }
	if _, err := f.svc.CompleteOAuth(context.Background(), "code-owner", state); !errors.Is(err, github.ErrInvalidOAuthState) {
		t.Fatalf("expired state: %v", err)
	}
}

func TestGitHubAccountCannotLinkToTwoUsers(t *testing.T) {
	f := newFixture(t)
	f.connectAccount(t, "owner", "code-owner")
	_, state, _ := f.svc.StartOAuth(context.Background(), "member")
	if _, err := f.svc.CompleteOAuth(context.Background(), "code-owner", state); !errors.Is(err, github.ErrIdentityInUse) {
		t.Fatalf("expected ErrIdentityInUse, got %v", err)
	}
}

func TestNotConfigured(t *testing.T) {
	f := newFixture(t)
	f.svc.Client = nil
	ctx := context.Background()
	if f.svc.Configured() {
		t.Fatal("service without client reports configured")
	}
	if _, _, err := f.svc.StartOAuth(ctx, "owner"); !errors.Is(err, github.ErrNotConfigured) {
		t.Fatalf("start oauth: %v", err)
	}
	if _, err := f.svc.ListAvailableRepositories(ctx, "owner"); !errors.Is(err, github.ErrNotConfigured) {
		t.Fatalf("list: %v", err)
	}
	if _, err := f.svc.ConnectRepository(ctx, "owner", "p1", 42); !errors.Is(err, github.ErrNotConfigured) {
		t.Fatalf("connect: %v", err)
	}
	if err := f.svc.HandleWebhook(ctx, "push", []byte("{}"), ""); !errors.Is(err, github.ErrNotConfigured) {
		t.Fatalf("webhook: %v", err)
	}
	if repo, err := f.svc.FindRepository(ctx, "member", "p1"); err != nil || repo != nil {
		t.Fatalf("viewing stays available: %v %v", repo, err)
	}
}

func TestConnectRepositoryAuthorization(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, u := range []struct{ id, code string }{{"owner", "code-owner"}, {"admin", "code-admin"}, {"member", "code-member"}} {
		f.connectAccount(t, u.id, u.code)
	}

	if _, err := f.svc.ConnectRepository(ctx, "member", "p1", 42); !errors.Is(err, github.ErrForbidden) {
		t.Fatalf("member connect: %v", err)
	}
	if _, err := f.svc.ConnectRepository(ctx, "outsider", "p1", 42); !errors.Is(err, github.ErrNotFound) {
		t.Fatalf("non-member connect: %v", err)
	}
	if _, err := f.svc.ConnectRepository(ctx, "admin", "p1", 999); !errors.Is(err, github.ErrRepositoryUnavailable) {
		t.Fatalf("inaccessible repo: %v", err)
	}

	repo, err := f.svc.ConnectRepository(ctx, "admin", "p1", 42)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	if repo.ProjectID != "p1" || repo.Name != "hub" || repo.ConnectedByUserID != "admin" || repo.DefaultBranch != "main" {
		t.Fatalf("repo: %+v", repo)
	}
	if len(f.rec.connected) != 3 {
		t.Fatalf("expected connected event to 3 members, got %v", f.rec.connected)
	}

	if _, err := f.svc.ConnectRepository(ctx, "owner", "p1", 42); !errors.Is(err, github.ErrAlreadyConnected) {
		t.Fatalf("second connect: %v", err)
	}

	got, err := f.svc.GetRepository(ctx, "member", "p1")
	if err != nil || got.ExternalID != 42 {
		t.Fatalf("member view: %+v %v", got, err)
	}
	if _, err := f.svc.GetRepository(ctx, "outsider", "p1"); !errors.Is(err, github.ErrNotFound) {
		t.Fatalf("outsider view: %v", err)
	}

	if err := f.svc.DisconnectRepository(ctx, "member", "p1"); !errors.Is(err, github.ErrForbidden) {
		t.Fatalf("member disconnect: %v", err)
	}
	if err := f.svc.DisconnectRepository(ctx, "owner", "p1"); err != nil {
		t.Fatalf("owner disconnect: %v", err)
	}
	if repo, err := f.svc.FindRepository(ctx, "member", "p1"); err != nil || repo != nil {
		t.Fatalf("after disconnect: %+v %v", repo, err)
	}
	if err := f.svc.DisconnectRepository(ctx, "owner", "p1"); !errors.Is(err, github.ErrNotFound) {
		t.Fatalf("repeat disconnect: %v", err)
	}
}

func TestConnectRepositoryRequiresConnectedAccount(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.ConnectRepository(context.Background(), "owner", "p1", 42); !errors.Is(err, github.ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got %v", err)
	}
}

func TestUpdateNotificationConfigOwnerOnly(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.connectAccount(t, "owner", "code-owner")
	if _, err := f.svc.UpdateNotificationConfig(ctx, "owner", "p1", true, false, false); !errors.Is(err, github.ErrNotFound) {
		t.Fatalf("no repo yet: %v", err)
	}
	if _, err := f.svc.ConnectRepository(ctx, "owner", "p1", 42); err != nil {
		t.Fatal(err)
	}
	for _, who := range []string{"admin", "member"} {
		if _, err := f.svc.UpdateNotificationConfig(ctx, who, "p1", true, false, false); !errors.Is(err, github.ErrForbidden) {
			t.Fatalf("%s update: %v", who, err)
		}
	}
	repo, err := f.svc.UpdateNotificationConfig(ctx, "owner", "p1", true, false, false)
	if err != nil || !repo.PostPushesToChat || repo.NotifyAllMembers || repo.NotifyAllBranches {
		t.Fatalf("owner update: %+v %v", repo, err)
	}
}

func pushPayload(t *testing.T, repoID int64, ref string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"ref":        ref,
		"repository": map[string]any{"id": repoID},
		"sender":     map[string]any{"login": "alice-gh"},
		"pusher":     map[string]any{"name": "alice-gh"},
		"commits": []map[string]any{
			{"message": "Add feature\n\nLong body"},
			{"message": "Fix bug"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestWebhookSignatureVerification(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	payload := pushPayload(t, 42, "refs/heads/main")
	valid := githubtest.Sign(webhookSecret, payload)

	tampered := bytes.Replace(payload, []byte("Fix bug"), []byte("Fix bux"), 1)
	for name, tc := range map[string]struct {
		payload []byte
		sig     string
	}{
		"missing":          {payload, ""},
		"no prefix":        {payload, strings.TrimPrefix(valid, "sha256=")},
		"sha1 header":      {payload, "sha1=" + strings.TrimPrefix(valid, "sha256=")},
		"not hex":          {payload, "sha256=zzzz"},
		"wrong secret":     {payload, githubtest.Sign("other-secret", payload)},
		"tampered payload": {tampered, valid},
	} {
		if err := f.svc.HandleWebhook(ctx, "push", tc.payload, tc.sig); !errors.Is(err, github.ErrInvalidSignature) {
			t.Errorf("%s: expected ErrInvalidSignature, got %v", name, err)
		}
	}
	if err := f.svc.HandleWebhook(ctx, "push", payload, valid); err != nil {
		t.Fatalf("valid signature: %v", err)
	}
	if !github.VerifySignature(webhookSecret, payload, valid) || github.VerifySignature("", payload, githubtest.Sign("", payload)) {
		t.Fatal("VerifySignature: valid must pass, empty secret must never verify")
	}
}

func TestWebhookPushDeliversToMembers(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.connectAccount(t, "owner", "code-owner")
	if _, err := f.svc.ConnectRepository(ctx, "owner", "p1", 42); err != nil {
		t.Fatal(err)
	}
	send := func(event string, payload []byte) {
		t.Helper()
		if err := f.svc.HandleWebhook(ctx, event, payload, githubtest.Sign(webhookSecret, payload)); err != nil {
			t.Fatalf("webhook %s: %v", event, err)
		}
	}

	send("ping", []byte(`{"zen":"hi"}`))
	send("push", pushPayload(t, 7777, "refs/heads/main")) // unconnected repo
	send("push", pushPayload(t, 42, "refs/tags/v1"))      // tag push
	if len(f.rec.pushes) != 0 || len(f.rec.notifications) != 0 {
		t.Fatalf("ignored events delivered: %v %v", f.rec.pushes, f.rec.notifications)
	}

	send("push", pushPayload(t, 42, "refs/heads/feature"))
	if len(f.rec.pushes) != 3 || len(f.rec.notifications) != 3 {
		t.Fatalf("expected push to all 3 members: %v %v", f.rec.pushes, f.rec.notifications)
	}
	p := f.rec.lastPayload.(map[string]any)
	if p["branch"] != "feature" || p["commit_count"] != 2 || p["pusher_login"] != "alice-gh" {
		t.Fatalf("payload: %v", p)
	}
	if s := p["commit_summaries"].([]string); len(s) != 2 || s[0] != "Add feature" {
		t.Fatalf("summaries: %v", s)
	}

	// Owner narrows to default branch only, no persistent notifications.
	if _, err := f.svc.UpdateNotificationConfig(ctx, "owner", "p1", false, false, false); err != nil {
		t.Fatal(err)
	}
	f.rec.pushes, f.rec.notifications = nil, nil
	send("push", pushPayload(t, 42, "refs/heads/feature"))
	if len(f.rec.pushes) != 0 {
		t.Fatalf("non-default branch delivered: %v", f.rec.pushes)
	}
	send("push", pushPayload(t, 42, "refs/heads/main"))
	if len(f.rec.pushes) != 3 || len(f.rec.notifications) != 0 {
		t.Fatalf("default branch: pushes=%v notifications=%v", f.rec.pushes, f.rec.notifications)
	}
}

func TestWebhookMalformedPayload(t *testing.T) {
	f := newFixture(t)
	payload := []byte("{not json")
	var v *github.ValidationError
	if err := f.svc.HandleWebhook(context.Background(), "push", payload, githubtest.Sign(webhookSecret, payload)); !errors.As(err, &v) {
		t.Fatalf("expected validation error, got %v", err)
	}
}
