package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/chat"
	"github.com/itswskkk/KMJG-Hub/server/internal/project"
)

type fakeChatRepo struct {
	mu       sync.Mutex
	nextID   int
	messages map[string][]chat.Message
	users    usernameLookup
	projects *fakeProjectRepo
}

func newFakeChatRepo(users usernameLookup, projects *fakeProjectRepo) *fakeChatRepo {
	return &fakeChatRepo{messages: make(map[string][]chat.Message), users: users, projects: projects}
}

func (f *fakeChatRepo) role(projectID, userID string) (project.Role, bool) {
	f.projects.mu.Lock()
	defer f.projects.mu.Unlock()
	for _, member := range f.projects.members[projectID] {
		if member.UserID == userID {
			return member.Role, true
		}
	}
	return "", false
}

func (f *fakeChatRepo) Create(_ context.Context, projectID, authorID, body string) (*chat.Message, error) {
	if _, ok := f.role(projectID, authorID); !ok {
		return nil, chat.ErrNotFound
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	message := chat.Message{ID: fmt.Sprintf("message-%d", f.nextID), ProjectID: projectID, AuthorID: authorID, AuthorUsername: f.users.usernameFor(authorID), Body: body, CreatedAt: time.Now().UTC()}
	f.messages[projectID] = append(f.messages[projectID], message)
	copy := message
	return &copy, nil
}

func (f *fakeChatRepo) ListRecent(_ context.Context, projectID, viewerID string, limit int) ([]chat.Message, error) {
	if _, ok := f.role(projectID, viewerID); !ok {
		return nil, chat.ErrNotFound
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	messages := f.messages[projectID]
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	return append([]chat.Message(nil), messages...), nil
}

func (f *fakeChatRepo) SoftDelete(_ context.Context, projectID, messageID, actorID string) (*chat.Message, error) {
	role, member := f.role(projectID, actorID)
	if !member {
		return nil, chat.ErrNotFound
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, message := range f.messages[projectID] {
		if message.ID != messageID {
			continue
		}
		if message.AuthorID != actorID && role != project.RoleOwner && role != project.RoleAdmin {
			return nil, chat.ErrForbidden
		}
		f.messages[projectID] = append(f.messages[projectID][:i], f.messages[projectID][i+1:]...)
		copy := message
		return &copy, nil
	}
	return nil, chat.ErrNotFound
}

func (f *fakeChatRepo) PurgeDeletedBefore(context.Context, time.Time) error { return nil }

func TestProjectChatSendListAndDeletePermissions(t *testing.T) {
	router, _, projects := newTestRouterWithHandlers()
	ownerToken := registerAndToken(t, router, "owner")
	memberToken := registerAndToken(t, router, "member")
	otherToken := registerAndToken(t, router, "other")

	created := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Chat Project"}, ownerToken)
	var p struct {
		ID string `json:"id"`
	}
	decodeJSON(t, created, &p)
	memberID := currentUserID(t, router, memberToken)
	otherID := currentUserID(t, router, otherToken)
	projects.addMember(p.ID, memberID, project.RoleMember)
	projects.addMember(p.ID, otherID, project.RoleMember)

	sent := doJSON(t, router, http.MethodPost, "/api/v1/projects/"+p.ID+"/chat/messages", map[string]string{"body": "  hello team  "}, memberToken)
	if sent.Code != http.StatusCreated {
		t.Fatalf("send: %d %s", sent.Code, sent.Body.String())
	}
	var message struct {
		ID       string `json:"id"`
		Body     string `json:"body"`
		AuthorID string `json:"author_id"`
	}
	decodeJSON(t, sent, &message)
	if message.Body != "hello team" || message.AuthorID != memberID {
		t.Fatalf("unexpected message: %+v (raw: %s)", message, sent.Body.String())
	}

	listed := doJSON(t, router, http.MethodGet, "/api/v1/projects/"+p.ID+"/chat/messages", nil, ownerToken)
	if listed.Code != http.StatusOK {
		t.Fatalf("list: %d %s", listed.Code, listed.Body.String())
	}
	var body struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	decodeJSON(t, listed, &body)
	if len(body.Messages) != 1 || body.Messages[0].ID != message.ID {
		t.Fatalf("unexpected history: %+v", body.Messages)
	}

	forbidden := doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+p.ID+"/chat/messages/"+message.ID, nil, otherToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("member deleting another message: %d %s", forbidden.Code, forbidden.Body.String())
	}
	deleted := doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+p.ID+"/chat/messages/"+message.ID, nil, ownerToken)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("owner moderation delete: %d %s", deleted.Code, deleted.Body.String())
	}

	listed = doJSON(t, router, http.MethodGet, "/api/v1/projects/"+p.ID+"/chat/messages", nil, memberToken)
	decodeJSON(t, listed, &body)
	if len(body.Messages) != 0 {
		t.Fatalf("deleted message still visible: %+v", body.Messages)
	}
}

func TestProjectChatRejectsNonMembersAndInvalidMessages(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	ownerToken := registerAndToken(t, router, "owner2")
	outsiderToken := registerAndToken(t, router, "outsider")
	created := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Private Chat"}, ownerToken)
	var p struct {
		ID string `json:"id"`
	}
	decodeJSON(t, created, &p)

	if rec := doJSON(t, router, http.MethodGet, "/api/v1/projects/"+p.ID+"/chat/messages", nil, outsiderToken); rec.Code != http.StatusNotFound {
		t.Fatalf("non-member list: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, router, http.MethodPost, "/api/v1/projects/"+p.ID+"/chat/messages", map[string]string{"body": "   "}, ownerToken); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty send: %d %s", rec.Code, rec.Body.String())
	}
}

func TestProjectChatRealtimeEventsReachCurrentMembersOnly(t *testing.T) {
	router, _, projects := newTestRouterWithHandlers()
	server := httptest.NewServer(router)
	defer server.Close()

	ownerToken := registerAndToken(t, router, "chatowner")
	memberToken := registerAndToken(t, router, "chatmember")
	outsiderToken := registerAndToken(t, router, "chatoutsider")
	created := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Realtime Chat"}, ownerToken)
	var p struct {
		ID string `json:"id"`
	}
	decodeJSON(t, created, &p)
	projects.addMember(p.ID, currentUserID(t, router, memberToken), project.RoleMember)

	memberConn := dial(t, server)
	if err := memberConn.WriteJSON(map[string]any{"type": "auth", "data": map[string]string{"token": memberToken}}); err != nil {
		t.Fatalf("member auth: %v", err)
	}
	_ = readEnvelope(t, memberConn) // connected
	_ = readEnvelope(t, memberConn) // presence snapshot

	outsiderConn := dial(t, server)
	if err := outsiderConn.WriteJSON(map[string]any{"type": "auth", "data": map[string]string{"token": outsiderToken}}); err != nil {
		t.Fatalf("outsider auth: %v", err)
	}
	_ = readEnvelope(t, outsiderConn) // connected; no Project snapshot

	sent := doJSON(t, router, http.MethodPost, "/api/v1/projects/"+p.ID+"/chat/messages", map[string]string{"body": "realtime"}, ownerToken)
	if sent.Code != http.StatusCreated {
		t.Fatalf("send: %d %s", sent.Code, sent.Body.String())
	}

	env := readEnvelope(t, memberConn)
	if env.Type != chat.MessageCreatedEvent {
		t.Fatalf("expected %q, got %q", chat.MessageCreatedEvent, env.Type)
	}
	expectNoMoreMessages(t, outsiderConn)

	var message struct {
		ID string `json:"id"`
	}
	decodeJSON(t, sent, &message)
	deleted := doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+p.ID+"/chat/messages/"+message.ID, nil, ownerToken)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", deleted.Code, deleted.Body.String())
	}
	deleteEnv := readEnvelope(t, memberConn)
	if deleteEnv.Type != chat.MessageDeletedEvent {
		t.Fatalf("expected %q, got %q", chat.MessageDeletedEvent, deleteEnv.Type)
	}
}

func currentUserID(t *testing.T, router http.Handler, token string) string {
	t.Helper()
	rec := doJSON(t, router, http.MethodGet, "/api/v1/auth/session", nil, token)
	var user struct {
		ID string `json:"id"`
	}
	decodeJSON(t, rec, &user)
	return user.ID
}
