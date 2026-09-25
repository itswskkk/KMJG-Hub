package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/invitation"
	"github.com/itswskkk/KMJG-Hub/server/internal/project"
)

type fakeInvitationRepo struct {
	mu          sync.Mutex
	nextID      int
	items       []invitation.Direct
	credentials []fakeCredential
	users       *fakeUserRepo
	projects    *fakeProjectRepo
}

type fakeCredential struct {
	item    invitation.Credential
	hash    string
	revoked bool
}

func newFakeInvitationRepo(users *fakeUserRepo, projects *fakeProjectRepo) *fakeInvitationRepo {
	return &fakeInvitationRepo{users: users, projects: projects}
}

func (f *fakeInvitationRepo) CreateDirect(_ context.Context, projectID, inviterID, recipientIdentifier string, expiresAt *time.Time) (*invitation.Direct, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	detail, err := f.projects.GetDetailForUser(context.Background(), projectID, inviterID)
	if err != nil {
		return nil, invitation.ErrNotFound
	}
	if detail.ViewerRole != project.RoleOwner && detail.ViewerRole != project.RoleAdmin {
		return nil, invitation.ErrForbidden
	}
	recipient, err := f.users.GetByUsernameOrEmail(context.Background(), recipientIdentifier)
	if err != nil {
		return nil, invitation.ErrRecipientNotFound
	}
	if recipient.ID == inviterID {
		return nil, invitation.ErrConflict
	}
	for _, member := range detail.Members {
		if member.UserID == recipient.ID {
			return nil, invitation.ErrConflict
		}
	}
	for _, item := range f.items {
		if item.ProjectID == projectID && item.RecipientUserID == recipient.ID && (item.ExpiresAt == nil || item.ExpiresAt.After(time.Now())) {
			return nil, invitation.ErrConflict
		}
	}
	f.nextID++
	inviter, _ := f.users.GetByID(context.Background(), inviterID)
	item := invitation.Direct{ID: fmt.Sprintf("invitation-%d", f.nextID), ProjectID: projectID, ProjectName: detail.Name, InviterUserID: inviterID, InviterUsername: inviter.Username, RecipientUserID: recipient.ID, RecipientUsername: recipient.Username, CreatedAt: time.Now().UTC(), ExpiresAt: expiresAt}
	f.items = append(f.items, item)
	return &item, nil
}

func (f *fakeInvitationRepo) ListReceived(_ context.Context, recipientID string) ([]invitation.Direct, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.list(func(i invitation.Direct) bool { return i.RecipientUserID == recipientID }), nil
}
func (f *fakeInvitationRepo) ListForProject(_ context.Context, projectID, actorID string) ([]invitation.Direct, error) {
	detail, err := f.projects.GetDetailForUser(context.Background(), projectID, actorID)
	if err != nil {
		return nil, invitation.ErrForbidden
	}
	if detail.ViewerRole != project.RoleOwner && detail.ViewerRole != project.RoleAdmin {
		return nil, invitation.ErrForbidden
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.list(func(i invitation.Direct) bool { return i.ProjectID == projectID }), nil
}
func (f *fakeInvitationRepo) list(match func(invitation.Direct) bool) []invitation.Direct {
	var out []invitation.Direct
	now := time.Now()
	for _, i := range f.items {
		if match(i) && (i.ExpiresAt == nil || i.ExpiresAt.After(now)) {
			out = append(out, i)
		}
	}
	return out
}
func (f *fakeInvitationRepo) take(id, user string, now time.Time) (invitation.Direct, error) {
	for idx, i := range f.items {
		if i.ID == id && i.RecipientUserID == user && (i.ExpiresAt == nil || i.ExpiresAt.After(now)) {
			f.items = append(f.items[:idx], f.items[idx+1:]...)
			return i, nil
		}
	}
	return invitation.Direct{}, invitation.ErrNotFound
}
func (f *fakeInvitationRepo) Accept(_ context.Context, id, user string, now time.Time) (string, error) {
	f.mu.Lock()
	item, err := f.take(id, user, now)
	f.mu.Unlock()
	if err != nil {
		return "", err
	}
	f.projects.addMember(item.ProjectID, user, project.RoleMember)
	return item.ProjectID, nil
}
func (f *fakeInvitationRepo) Decline(_ context.Context, id, user string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, err := f.take(id, user, now)
	return err
}
func (f *fakeInvitationRepo) Cancel(_ context.Context, projectID, id, inviter string, now time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for idx, i := range f.items {
		if i.ID == id && i.ProjectID == projectID && i.InviterUserID == inviter && (i.ExpiresAt == nil || i.ExpiresAt.After(now)) {
			f.items = append(f.items[:idx], f.items[idx+1:]...)
			return nil
		}
	}
	return invitation.ErrNotFound
}

func (f *fakeInvitationRepo) CreateCredential(_ context.Context, projectID, creatorID, hash string, expiresAt *time.Time, maxUses *int) (*invitation.Credential, error) {
	detail, err := f.projects.GetDetailForUser(context.Background(), projectID, creatorID)
	if err != nil {
		return nil, invitation.ErrNotFound
	}
	if detail.ViewerRole != project.RoleOwner && detail.ViewerRole != project.RoleAdmin {
		return nil, invitation.ErrForbidden
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	creator, _ := f.users.GetByID(context.Background(), creatorID)
	item := invitation.Credential{ID: fmt.Sprintf("credential-%d", f.nextID), ProjectID: projectID, ProjectName: detail.Name, CreatorUserID: creatorID, CreatorUsername: creator.Username, CreatedAt: time.Now().UTC(), ExpiresAt: expiresAt, MaxUses: maxUses}
	f.credentials = append(f.credentials, fakeCredential{item: item, hash: hash})
	return &item, nil
}

func (f *fakeInvitationRepo) ListCredentials(_ context.Context, projectID, actorID string) ([]invitation.Credential, error) {
	detail, err := f.projects.GetDetailForUser(context.Background(), projectID, actorID)
	if err != nil {
		return nil, invitation.ErrNotFound
	}
	if detail.ViewerRole != project.RoleOwner && detail.ViewerRole != project.RoleAdmin {
		return nil, invitation.ErrForbidden
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []invitation.Credential
	now := time.Now()
	for _, c := range f.credentials {
		if c.item.ProjectID == projectID && !c.revoked && (c.item.ExpiresAt == nil || c.item.ExpiresAt.After(now)) && (c.item.MaxUses == nil || c.item.Uses < *c.item.MaxUses) {
			out = append(out, c.item)
		}
	}
	return out, nil
}

func (f *fakeInvitationRepo) RevokeCredential(_ context.Context, projectID, id, actorID string, _ time.Time) error {
	detail, err := f.projects.GetDetailForUser(context.Background(), projectID, actorID)
	if err != nil {
		return invitation.ErrNotFound
	}
	if detail.ViewerRole != project.RoleOwner && detail.ViewerRole != project.RoleAdmin {
		return invitation.ErrForbidden
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.credentials {
		if f.credentials[i].item.ID == id && f.credentials[i].item.ProjectID == projectID && !f.credentials[i].revoked {
			f.credentials[i].revoked = true
			return nil
		}
	}
	return invitation.ErrNotFound
}

func (f *fakeInvitationRepo) ConsumeCredential(_ context.Context, hash, userID string, now time.Time) (string, error) {
	f.mu.Lock()
	for i := range f.credentials {
		c := &f.credentials[i]
		if c.hash != hash {
			continue
		}
		if c.revoked {
			f.mu.Unlock()
			return "", invitation.ErrRevoked
		}
		if c.item.ExpiresAt != nil && !c.item.ExpiresAt.After(now) {
			f.mu.Unlock()
			return "", invitation.ErrExpired
		}
		if c.item.MaxUses != nil && c.item.Uses >= *c.item.MaxUses {
			f.mu.Unlock()
			return "", invitation.ErrUsesExhausted
		}
		projectID := c.item.ProjectID
		if detail, err := f.projects.GetDetailForUser(context.Background(), projectID, userID); err == nil && detail != nil {
			f.mu.Unlock()
			return "", invitation.ErrAlreadyMember
		}
		c.item.Uses++
		f.mu.Unlock()
		f.projects.addMember(projectID, userID, project.RoleMember)
		return projectID, nil
	}
	f.mu.Unlock()
	return "", invitation.ErrNotFound
}

func TestInviteCredentialJoinUseLimitAndRevoke(t *testing.T) {
	router, _, projects := newTestRouterWithHandlers()
	register := func(name string) (string, string) {
		rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register", map[string]string{"username": name, "email": name + "@example.com", "password": "hunter22222"}, "")
		var body struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		}
		decodeJSON(t, rec, &body)
		return body.User.ID, extractToken(t, rec)
	}
	_, ownerToken := register("linkowner")
	memberID, memberToken := register("linkmember")
	_, firstToken := register("linkfirst")
	_, secondToken := register("linksecond")
	created := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Link Project"}, ownerToken)
	var p struct {
		ID string `json:"id"`
	}
	decodeJSON(t, created, &p)
	projects.addMember(p.ID, memberID, project.RoleMember)
	if rec := doJSON(t, router, http.MethodPost, "/api/v1/projects/"+p.ID+"/invite-credentials", map[string]any{"expires_in": "7d", "max_uses": 1}, memberToken); rec.Code != http.StatusForbidden {
		t.Fatalf("member create: expected 403, got %d", rec.Code)
	}
	if rec := doJSON(t, router, http.MethodPost, "/api/v1/projects/"+p.ID+"/invite-credentials", map[string]any{"expires_in": "7d", "max_uses": 0}, ownerToken); rec.Code != http.StatusBadRequest {
		t.Fatalf("zero uses: expected 400, got %d", rec.Code)
	}
	create := doJSON(t, router, http.MethodPost, "/api/v1/projects/"+p.ID+"/invite-credentials", map[string]any{"expires_in": "7d", "max_uses": 1}, ownerToken)
	if create.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", create.Code, create.Body.String())
	}
	var credential invitation.Credential
	decodeJSON(t, create, &credential)
	if credential.Secret == "" {
		t.Fatal("create response omitted code")
	}
	listed := doJSON(t, router, http.MethodGet, "/api/v1/projects/"+p.ID+"/invite-credentials", nil, ownerToken)
	if listed.Code != http.StatusOK {
		t.Fatalf("list credentials: %d %s", listed.Code, listed.Body.String())
	}
	if strings.Contains(listed.Body.String(), credential.Secret) || strings.Contains(listed.Body.String(), `"code"`) {
		t.Fatal("credential listing exposed one-time secret")
	}
	already := doJSON(t, router, http.MethodPost, "/api/v1/invitations/join", map[string]string{"invite": credential.Secret}, ownerToken)
	if already.Code != http.StatusConflict {
		t.Fatalf("existing member: expected 409, got %d %s", already.Code, already.Body.String())
	}
	join := doJSON(t, router, http.MethodPost, "/api/v1/invitations/join", map[string]string{"invite": "https://hub.example/invite/" + credential.Secret}, firstToken)
	if join.Code != http.StatusOK {
		t.Fatalf("join: %d %s", join.Code, join.Body.String())
	}
	exhausted := doJSON(t, router, http.MethodPost, "/api/v1/invitations/join", map[string]string{"invite": credential.Secret}, secondToken)
	if exhausted.Code != http.StatusGone {
		t.Fatalf("exhausted: expected 410, got %d %s", exhausted.Code, exhausted.Body.String())
	}
	unlimited := doJSON(t, router, http.MethodPost, "/api/v1/projects/"+p.ID+"/invite-credentials", map[string]any{"expires_in": "never", "max_uses": nil}, ownerToken)
	var revoked invitation.Credential
	decodeJSON(t, unlimited, &revoked)
	if rec := doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+p.ID+"/invite-credentials/"+revoked.ID, nil, ownerToken); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, router, http.MethodPost, "/api/v1/invitations/join", map[string]string{"invite": revoked.Secret}, secondToken); rec.Code != http.StatusGone {
		t.Fatalf("revoked join: expected 410, got %d", rec.Code)
	}
}

func TestDirectInvitationAcceptFlowAndAuthorization(t *testing.T) {
	router, _, projects := newTestRouterWithHandlers()
	register := func(name string) (string, string) {
		rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register", map[string]string{"username": name, "email": name + "@example.com", "password": "hunter22222"}, "")
		var b struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		}
		decodeJSON(t, rec, &b)
		return b.User.ID, extractToken(t, rec)
	}
	_, ownerToken := register("ownerinvite")
	memberID, memberToken := register("memberinvite")
	recipientID, recipientToken := register("recipientinvite")
	_, secondRecipientToken := register("recipienttwo")
	created := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Invite Project"}, ownerToken)
	var p struct {
		ID string `json:"id"`
	}
	decodeJSON(t, created, &p)
	projects.addMember(p.ID, memberID, project.RoleMember)
	forbidden := doJSON(t, router, http.MethodPost, "/api/v1/projects/"+p.ID+"/invitations", map[string]string{"recipient": "recipientinvite", "expires_in": "7d"}, memberToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("member invite: expected 403, got %d: %s", forbidden.Code, forbidden.Body.String())
	}
	invalid := doJSON(t, router, http.MethodPost, "/api/v1/projects/"+p.ID+"/invitations", map[string]string{"recipient": "recipientinvite", "expires_in": "someday"}, ownerToken)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid expiration: expected 400, got %d", invalid.Code)
	}
	invite := doJSON(t, router, http.MethodPost, "/api/v1/projects/"+p.ID+"/invitations", map[string]string{"recipient": "recipientinvite", "expires_in": "7d"}, ownerToken)
	if invite.Code != http.StatusCreated {
		t.Fatalf("create invite: %d %s", invite.Code, invite.Body.String())
	}
	var item invitation.Direct
	decodeJSON(t, invite, &item)
	if item.RecipientUserID != recipientID {
		t.Fatalf("wrong recipient: %+v", item)
	}
	list := doJSON(t, router, http.MethodGet, "/api/v1/invitations", nil, recipientToken)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), item.ID) {
		t.Fatalf("list: %d %s", list.Code, list.Body.String())
	}
	wrongRecipient := doJSON(t, router, http.MethodPost, "/api/v1/invitations/"+item.ID+"/accept", nil, secondRecipientToken)
	if wrongRecipient.Code != http.StatusNotFound {
		t.Fatalf("wrong recipient: expected 404, got %d", wrongRecipient.Code)
	}
	accept := doJSON(t, router, http.MethodPost, "/api/v1/invitations/"+item.ID+"/accept", nil, recipientToken)
	if accept.Code != http.StatusOK {
		t.Fatalf("accept: %d %s", accept.Code, accept.Body.String())
	}
	get := doJSON(t, router, http.MethodGet, "/api/v1/projects/"+p.ID, nil, recipientToken)
	if get.Code != http.StatusOK {
		t.Fatalf("accepted recipient cannot open project: %d %s", get.Code, get.Body.String())
	}
}

func TestDirectInvitationDeclineAndInviterCancellation(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	register := func(name string) string {
		rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register", map[string]string{"username": name, "email": name + "@example.com", "password": "hunter22222"}, "")
		return extractToken(t, rec)
	}
	ownerToken := register("owneractions")
	recipientToken := register("recipientactions")
	otherToken := register("otheractions")
	created := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Actions Project"}, ownerToken)
	var p struct {
		ID string `json:"id"`
	}
	decodeJSON(t, created, &p)
	makeInvite := func(recipient string) invitation.Direct {
		rec := doJSON(t, router, http.MethodPost, "/api/v1/projects/"+p.ID+"/invitations", map[string]string{"recipient": recipient, "expires_in": "never"}, ownerToken)
		var item invitation.Direct
		decodeJSON(t, rec, &item)
		return item
	}
	declined := makeInvite("recipientactions")
	if rec := doJSON(t, router, http.MethodPost, "/api/v1/invitations/"+declined.ID+"/decline", nil, recipientToken); rec.Code != http.StatusNoContent {
		t.Fatalf("decline: %d %s", rec.Code, rec.Body.String())
	}
	cancelled := makeInvite("recipientactions")
	if rec := doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+p.ID+"/invitations/"+cancelled.ID, nil, otherToken); rec.Code != http.StatusNotFound {
		t.Fatalf("non-inviter cancel: expected 404, got %d", rec.Code)
	}
	if rec := doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+p.ID+"/invitations/"+cancelled.ID, nil, ownerToken); rec.Code != http.StatusNoContent {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body.String())
	}
}
