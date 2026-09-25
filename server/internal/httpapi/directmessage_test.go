package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/directmessage"
	"github.com/itswskkk/KMJG-Hub/server/internal/friend/friendtest"
	"github.com/itswskkk/KMJG-Hub/server/internal/project"
)

// fakeDMRepo is an in-memory directmessage.Repository mirroring the access
// rules internal/store/postgres.DirectMessageRepository enforces in SQL:
// friends OR shared Project, and neither user has blocked the other.
type fakeDMRepo struct {
	mu       sync.Mutex
	nextID   int
	messages []directmessage.Message
	deleted  map[string]bool
	users    *fakeUserRepo
	projects *fakeProjectRepo
	friends  *friendtest.Memory
}

func newFakeDMRepo(users *fakeUserRepo, projects *fakeProjectRepo, friends *friendtest.Memory) *fakeDMRepo {
	return &fakeDMRepo{deleted: map[string]bool{}, users: users, projects: projects, friends: friends}
}

func (f *fakeDMRepo) userExists(id string) bool {
	_, err := f.users.GetByID(context.Background(), id)
	return err == nil
}

func (f *fakeDMRepo) shareProject(a, b string) bool {
	f.projects.mu.Lock()
	defer f.projects.mu.Unlock()
	for _, members := range f.projects.members {
		var hasA, hasB bool
		for _, m := range members {
			hasA = hasA || m.UserID == a
			hasB = hasB || m.UserID == b
		}
		if hasA && hasB {
			return true
		}
	}
	return false
}

func (f *fakeDMRepo) Create(ctx context.Context, senderID, recipientID, body string) (*directmessage.Message, error) {
	if !f.userExists(senderID) || !f.userExists(recipientID) {
		return nil, directmessage.ErrNotFound
	}
	areFriends, _ := f.friends.AreFriends(ctx, senderID, recipientID)
	b1, _ := f.friends.IsBlocked(ctx, senderID, recipientID)
	b2, _ := f.friends.IsBlocked(ctx, recipientID, senderID)
	if b1 || b2 || !(areFriends || f.shareProject(senderID, recipientID)) {
		return nil, directmessage.ErrForbidden
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	m := directmessage.Message{
		ID: fmt.Sprintf("dm-%d", f.nextID), SenderID: senderID, SenderUsername: f.users.usernameFor(senderID),
		RecipientID: recipientID, RecipientUsername: f.users.usernameFor(recipientID), Body: body,
		CreatedAt: time.Date(2026, 1, 1, 0, 0, f.nextID, 0, time.UTC),
	}
	f.messages = append(f.messages, m)
	return &m, nil
}

func (f *fakeDMRepo) ListPage(_ context.Context, viewerID, otherUserID string, before *directmessage.Cursor, limit int) ([]directmessage.Message, error) {
	if !f.userExists(otherUserID) || otherUserID == viewerID {
		return nil, directmessage.ErrNotFound
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	matching := []directmessage.Message{}
	for _, m := range f.messages {
		if f.deleted[m.ID] {
			continue
		}
		if !((m.SenderID == viewerID && m.RecipientID == otherUserID) || (m.SenderID == otherUserID && m.RecipientID == viewerID)) {
			continue
		}
		if before != nil && !m.CreatedAt.Before(before.CreatedAt) {
			continue
		}
		matching = append(matching, m)
	}
	if len(matching) > limit {
		matching = matching[len(matching)-limit:]
	}
	return matching, nil
}

func (f *fakeDMRepo) ListConversations(_ context.Context, viewerID string) ([]directmessage.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	latest := map[string]directmessage.Message{}
	for _, m := range f.messages {
		if f.deleted[m.ID] {
			continue
		}
		var other string
		switch viewerID {
		case m.SenderID:
			other = m.RecipientID
		case m.RecipientID:
			other = m.SenderID
		default:
			continue
		}
		latest[other] = m
	}
	out := make([]directmessage.Conversation, 0, len(latest))
	for other, m := range latest {
		out = append(out, directmessage.Conversation{
			OtherUserID: other, OtherUsername: f.users.usernameFor(other),
			LastMessageBody: m.Body, LastMessageAt: m.CreatedAt, LastMessageFromMe: m.SenderID == viewerID,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastMessageAt.After(out[j].LastMessageAt) })
	return out, nil
}

func (f *fakeDMRepo) SoftDelete(_ context.Context, messageID, actorID string) (*directmessage.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.messages {
		if m.ID != messageID || f.deleted[m.ID] || (m.SenderID != actorID && m.RecipientID != actorID) {
			continue
		}
		if m.SenderID != actorID {
			return nil, directmessage.ErrForbidden
		}
		f.deleted[m.ID] = true
		return &m, nil
	}
	return nil, directmessage.ErrNotFound
}

func (f *fakeDMRepo) PurgeDeletedBefore(context.Context, time.Time) error { return nil }

type dmBody struct {
	ID          string `json:"id"`
	SenderID    string `json:"sender_id"`
	RecipientID string `json:"recipient_id"`
	Body        string `json:"body"`
}

func sendDM(t *testing.T, router http.Handler, from, to friendUser, body string) (int, dmBody) {
	t.Helper()
	rec := doJSON(t, router, http.MethodPost, "/api/v1/direct-messages/"+to.id, map[string]string{"body": body}, from.token)
	var out dmBody
	if rec.Code == http.StatusCreated {
		decodeJSON(t, rec, &out)
	}
	return rec.Code, out
}

func makeFriends(t *testing.T, router http.Handler, a, b friendUser) {
	t.Helper()
	req := sendFriendRequest(t, router, a, b)
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/friends/requests/"+req.ID+"/accept", nil, b.token), http.StatusOK, "accept friend request")
}

func newDMRouter(t *testing.T, names ...string) (http.Handler, *fakeProjectRepo, map[string]friendUser) {
	t.Helper()
	router, _, projects := newTestRouterWithHandlers()
	users := make(map[string]friendUser, len(names))
	for _, name := range names {
		token := registerAndToken(t, router, name)
		users[name] = friendUser{token: token, id: currentUserID(t, router, token)}
	}
	return router, projects, users
}

func TestDirectMessageEndpointsRequireAuth(t *testing.T) {
	router := newTestRouter()
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/direct-messages/conversations"},
		{http.MethodGet, "/api/v1/direct-messages/x"},
		{http.MethodPost, "/api/v1/direct-messages/x"},
		{http.MethodDelete, "/api/v1/direct-messages/x"},
	} {
		if rec := doJSON(t, router, tc.method, tc.path, nil, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401, got %d", tc.method, tc.path, rec.Code)
		}
	}
}

func TestDirectMessageAccessRules(t *testing.T) {
	router, projects, u := newDMRouter(t, "alice", "bob", "carol", "dave")

	// Unrelated users cannot DM.
	if code, _ := sendDM(t, router, u["alice"], u["bob"], "hi"); code != http.StatusForbidden {
		t.Fatalf("unrelated send: expected 403, got %d", code)
	}

	// Friends can DM.
	makeFriends(t, router, u["alice"], u["bob"])
	code, msg := sendDM(t, router, u["alice"], u["bob"], "  hi bob  ")
	if code != http.StatusCreated || msg.Body != "hi bob" || msg.SenderID != u["alice"].id || msg.RecipientID != u["bob"].id {
		t.Fatalf("friend send: %d %+v", code, msg)
	}

	// Shared Project membership suffices without friendship.
	created := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "DM Project"}, u["carol"].token)
	var p struct {
		ID string `json:"id"`
	}
	decodeJSON(t, created, &p)
	projects.addMember(p.ID, u["dave"].id, project.RoleMember)
	if code, _ := sendDM(t, router, u["dave"], u["carol"], "hello teammate"); code != http.StatusCreated {
		t.Fatalf("project member send: expected 201, got %d", code)
	}

	// Blocking (either direction) revokes access even for friends/teammates.
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/blocked", map[string]string{"user_id": u["dave"].id}, u["carol"].token), http.StatusCreated, "block")
	if code, _ := sendDM(t, router, u["dave"], u["carol"], "still there?"); code != http.StatusForbidden {
		t.Fatalf("blocked sender: expected 403, got %d", code)
	}
	if code, _ := sendDM(t, router, u["carol"], u["dave"], "bye"); code != http.StatusForbidden {
		t.Fatalf("blocker sender: expected 403, got %d", code)
	}

	// Validation and unknown recipients.
	if code, _ := sendDM(t, router, u["alice"], u["bob"], "   "); code != http.StatusBadRequest {
		t.Fatalf("empty body: expected 400, got %d", code)
	}
	if code, _ := sendDM(t, router, u["alice"], u["alice"], "me"); code != http.StatusBadRequest {
		t.Fatalf("self send: expected 400, got %d", code)
	}
	if code, _ := sendDM(t, router, u["alice"], friendUser{id: "ghost"}, "hi"); code != http.StatusNotFound {
		t.Fatalf("unknown recipient: expected 404, got %d", code)
	}
}

func TestDirectMessageDeleteOnlyBySender(t *testing.T) {
	router, _, u := newDMRouter(t, "alice", "bob", "eve")
	makeFriends(t, router, u["alice"], u["bob"])
	_, msg := sendDM(t, router, u["alice"], u["bob"], "delete me")

	expectStatus(t, doJSON(t, router, http.MethodDelete, "/api/v1/direct-messages/"+msg.ID, nil, u["bob"].token), http.StatusForbidden, "recipient delete")
	expectStatus(t, doJSON(t, router, http.MethodDelete, "/api/v1/direct-messages/"+msg.ID, nil, u["eve"].token), http.StatusNotFound, "outsider delete")
	expectStatus(t, doJSON(t, router, http.MethodDelete, "/api/v1/direct-messages/"+msg.ID, nil, u["alice"].token), http.StatusNoContent, "sender delete")
	expectStatus(t, doJSON(t, router, http.MethodDelete, "/api/v1/direct-messages/"+msg.ID, nil, u["alice"].token), http.StatusNotFound, "repeat delete")

	rec := doJSON(t, router, http.MethodGet, "/api/v1/direct-messages/"+u["alice"].id, nil, u["bob"].token)
	var list struct {
		Messages []dmBody `json:"messages"`
	}
	decodeJSON(t, rec, &list)
	if len(list.Messages) != 0 {
		t.Fatalf("deleted message still listed: %+v", list.Messages)
	}
}

func TestDirectMessageListAndConversations(t *testing.T) {
	router, _, u := newDMRouter(t, "alice", "bob", "carol")
	makeFriends(t, router, u["alice"], u["bob"])
	makeFriends(t, router, u["alice"], u["carol"])

	total := directmessage.RecentMessageLimit + 3
	for i := 0; i < total; i++ {
		from, to := u["alice"], u["bob"]
		if i%2 == 1 {
			from, to = to, from
		}
		if code, _ := sendDM(t, router, from, to, fmt.Sprintf("m%d", i)); code != http.StatusCreated {
			t.Fatalf("send %d: %d", i, code)
		}
	}
	if code, _ := sendDM(t, router, u["carol"], u["alice"], "latest"); code != http.StatusCreated {
		t.Fatalf("carol send: %d", code)
	}

	type page struct {
		Messages   []dmBody `json:"messages"`
		NextCursor string   `json:"next_cursor"`
	}
	rec := doJSON(t, router, http.MethodGet, "/api/v1/direct-messages/"+u["bob"].id, nil, u["alice"].token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var first page
	decodeJSON(t, rec, &first)
	if len(first.Messages) != directmessage.RecentMessageLimit || first.NextCursor == "" {
		t.Fatalf("first page: %d messages cursor=%q", len(first.Messages), first.NextCursor)
	}
	rec = doJSON(t, router, http.MethodGet, "/api/v1/direct-messages/"+u["bob"].id+"?cursor="+first.NextCursor, nil, u["alice"].token)
	var second page
	decodeJSON(t, rec, &second)
	if len(second.Messages) != 3 || second.NextCursor != "" || second.Messages[0].Body != "m0" {
		t.Fatalf("second page: %+v cursor=%q", second.Messages, second.NextCursor)
	}
	expectStatus(t, doJSON(t, router, http.MethodGet, "/api/v1/direct-messages/"+u["bob"].id+"?cursor=garbage!", nil, u["alice"].token), http.StatusBadRequest, "bad cursor")

	// Carol has no messages with bob, so she sees an empty thread.
	rec = doJSON(t, router, http.MethodGet, "/api/v1/direct-messages/"+u["bob"].id, nil, u["carol"].token)
	var empty page
	decodeJSON(t, rec, &empty)
	if len(empty.Messages) != 0 {
		t.Fatalf("carol sees alice/bob messages: %+v", empty.Messages)
	}

	rec = doJSON(t, router, http.MethodGet, "/api/v1/direct-messages/conversations", nil, u["alice"].token)
	if rec.Code != http.StatusOK {
		t.Fatalf("conversations: %d %s", rec.Code, rec.Body.String())
	}
	var convs struct {
		Conversations []struct {
			OtherUserID       string `json:"other_user_id"`
			OtherUsername     string `json:"other_username"`
			LastMessageBody   string `json:"last_message_body"`
			LastMessageFromMe bool   `json:"last_message_from_me"`
		} `json:"conversations"`
	}
	decodeJSON(t, rec, &convs)
	if len(convs.Conversations) != 2 {
		t.Fatalf("expected 2 conversations: %s", rec.Body.String())
	}
	if c := convs.Conversations[0]; c.OtherUserID != u["carol"].id || c.OtherUsername != "carol" || c.LastMessageBody != "latest" || c.LastMessageFromMe {
		t.Fatalf("first conversation: %+v", c)
	}
	if c := convs.Conversations[1]; c.OtherUserID != u["bob"].id || c.LastMessageBody != fmt.Sprintf("m%d", total-1) {
		t.Fatalf("second conversation: %+v", c)
	}
}
