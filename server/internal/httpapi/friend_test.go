package httpapi_test

import (
	"context"
	"net/http"
	"testing"
)

// fakeUserDirectory adapts fakeUserRepo to friendtest.UserDirectory so the
// in-memory friend repository sees the same users the auth fakes register.
type fakeUserDirectory struct{ users *fakeUserRepo }

func (d fakeUserDirectory) Username(id string) (string, bool) {
	u, err := d.users.GetByID(context.Background(), id)
	if err != nil {
		return "", false
	}
	return u.Username, true
}

func (d fakeUserDirectory) Resolve(identifier string) (string, bool) {
	u, err := d.users.GetByUsernameOrEmail(context.Background(), identifier)
	if err != nil {
		return "", false
	}
	return u.ID, true
}

type friendUser struct {
	token, id string
}

// newTestRouterWithFriends returns a router plus registered users by name.
func newTestRouterWithFriends(t *testing.T, names ...string) (http.Handler, map[string]friendUser) {
	t.Helper()
	router, _, _ := newTestRouterWithHandlers()
	users := make(map[string]friendUser, len(names))
	for _, name := range names {
		token := registerAndToken(t, router, name)
		users[name] = friendUser{token: token, id: currentUserID(t, router, token)}
	}
	return router, users
}

type friendRequestBody struct {
	ID                string `json:"id"`
	SenderID          string `json:"sender_id"`
	SenderUsername    string `json:"sender_username"`
	RecipientID       string `json:"recipient_id"`
	RecipientUsername string `json:"recipient_username"`
	Status            string `json:"status"`
}

func expectStatus(t *testing.T, rec interface {
	Result() *http.Response
}, want int, what string) {
	t.Helper()
	if got := rec.Result().StatusCode; got != want {
		t.Fatalf("%s: expected %d, got %d", what, want, got)
	}
}

func sendFriendRequest(t *testing.T, router http.Handler, from friendUser, to friendUser) friendRequestBody {
	t.Helper()
	rec := doJSON(t, router, http.MethodPost, "/api/v1/friends/requests", map[string]string{"recipient_id": to.id}, from.token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("send request: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body friendRequestBody
	decodeJSON(t, rec, &body)
	return body
}

func listFriendIDs(t *testing.T, router http.Handler, u friendUser) []string {
	t.Helper()
	rec := doJSON(t, router, http.MethodGet, "/api/v1/friends", nil, u.token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list friends: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Friends []struct {
			UserID   string `json:"user_id"`
			Username string `json:"username"`
		} `json:"friends"`
	}
	decodeJSON(t, rec, &body)
	ids := make([]string, 0, len(body.Friends))
	for _, f := range body.Friends {
		ids = append(ids, f.UserID)
	}
	return ids
}

func TestFriendEndpointsRequireAuth(t *testing.T) {
	router := newTestRouter()
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/friends/requests"},
		{http.MethodGet, "/api/v1/friends/requests/incoming"},
		{http.MethodGet, "/api/v1/friends/requests/outgoing"},
		{http.MethodPost, "/api/v1/friends/requests/x/accept"},
		{http.MethodPost, "/api/v1/friends/requests/x/decline"},
		{http.MethodDelete, "/api/v1/friends/requests/x"},
		{http.MethodGet, "/api/v1/friends"},
		{http.MethodDelete, "/api/v1/friends/x"},
		{http.MethodPost, "/api/v1/blocked"},
		{http.MethodDelete, "/api/v1/blocked/x"},
		{http.MethodGet, "/api/v1/blocked"},
	} {
		rec := doJSON(t, router, tc.method, tc.path, nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401, got %d", tc.method, tc.path, rec.Code)
		}
	}
}

func TestSendAndAcceptFriendRequest(t *testing.T) {
	router, u := newTestRouterWithFriends(t, "alice", "bob")

	// By username, to exercise recipient resolution.
	rec := doJSON(t, router, http.MethodPost, "/api/v1/friends/requests", map[string]string{"recipient": "bob"}, u["alice"].token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
	var req friendRequestBody
	decodeJSON(t, rec, &req)
	if req.SenderUsername != "alice" || req.RecipientID != u["bob"].id || req.Status != "pending" {
		t.Fatalf("unexpected request: %+v", req)
	}

	incoming := doJSON(t, router, http.MethodGet, "/api/v1/friends/requests/incoming", nil, u["bob"].token)
	var list struct {
		Requests []friendRequestBody `json:"requests"`
	}
	decodeJSON(t, incoming, &list)
	if len(list.Requests) != 1 || list.Requests[0].ID != req.ID {
		t.Fatalf("incoming: %s", incoming.Body.String())
	}
	outgoing := doJSON(t, router, http.MethodGet, "/api/v1/friends/requests/outgoing", nil, u["alice"].token)
	decodeJSON(t, outgoing, &list)
	if len(list.Requests) != 1 {
		t.Fatalf("outgoing: %s", outgoing.Body.String())
	}

	// Duplicate is rejected.
	dup := doJSON(t, router, http.MethodPost, "/api/v1/friends/requests", map[string]string{"recipient_id": u["bob"].id}, u["alice"].token)
	expectStatus(t, dup, http.StatusConflict, "duplicate request")

	// Only the recipient may accept.
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/friends/requests/"+req.ID+"/accept", nil, u["alice"].token), http.StatusForbidden, "sender accept")

	acc := doJSON(t, router, http.MethodPost, "/api/v1/friends/requests/"+req.ID+"/accept", nil, u["bob"].token)
	if acc.Code != http.StatusOK {
		t.Fatalf("accept: %d %s", acc.Code, acc.Body.String())
	}
	if ids := listFriendIDs(t, router, u["alice"]); len(ids) != 1 || ids[0] != u["bob"].id {
		t.Fatalf("alice friends=%v", ids)
	}
	if ids := listFriendIDs(t, router, u["bob"]); len(ids) != 1 || ids[0] != u["alice"].id {
		t.Fatalf("bob friends=%v", ids)
	}
}

func TestSelfAndUnknownFriendRequests(t *testing.T) {
	router, u := newTestRouterWithFriends(t, "solo")
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/friends/requests", map[string]string{"recipient_id": u["solo"].id}, u["solo"].token), http.StatusBadRequest, "self request")
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/friends/requests", map[string]string{"recipient": "ghost"}, u["solo"].token), http.StatusNotFound, "unknown recipient")
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/friends/requests", map[string]string{}, u["solo"].token), http.StatusBadRequest, "missing recipient")
}

func TestDeclineOnlyByRecipient(t *testing.T) {
	router, u := newTestRouterWithFriends(t, "alice", "bob", "carol")
	req := sendFriendRequest(t, router, u["alice"], u["bob"])

	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/friends/requests/"+req.ID+"/decline", nil, u["alice"].token), http.StatusForbidden, "sender decline")
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/friends/requests/"+req.ID+"/decline", nil, u["carol"].token), http.StatusNotFound, "outsider decline")
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/friends/requests/"+req.ID+"/decline", nil, u["bob"].token), http.StatusOK, "recipient decline")
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/friends/requests/"+req.ID+"/accept", nil, u["bob"].token), http.StatusConflict, "accept after decline")
	if ids := listFriendIDs(t, router, u["alice"]); len(ids) != 0 {
		t.Fatalf("declined request made friends: %v", ids)
	}
}

func TestCancelOnlyBySender(t *testing.T) {
	router, u := newTestRouterWithFriends(t, "alice", "bob")
	req := sendFriendRequest(t, router, u["alice"], u["bob"])

	expectStatus(t, doJSON(t, router, http.MethodDelete, "/api/v1/friends/requests/"+req.ID, nil, u["bob"].token), http.StatusForbidden, "recipient cancel")
	expectStatus(t, doJSON(t, router, http.MethodDelete, "/api/v1/friends/requests/"+req.ID, nil, u["alice"].token), http.StatusNoContent, "sender cancel")
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/friends/requests/"+req.ID+"/accept", nil, u["bob"].token), http.StatusNotFound, "accept cancelled")
}

func TestBlockedUserCannotSendRequest(t *testing.T) {
	router, u := newTestRouterWithFriends(t, "alice", "bob")
	rec := doJSON(t, router, http.MethodPost, "/api/v1/blocked", map[string]string{"user_id": u["bob"].id}, u["alice"].token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("block: %d %s", rec.Code, rec.Body.String())
	}
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/friends/requests", map[string]string{"recipient_id": u["alice"].id}, u["bob"].token), http.StatusForbidden, "blocked sender")
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/friends/requests", map[string]string{"recipient_id": u["bob"].id}, u["alice"].token), http.StatusForbidden, "blocker sender")
}

func TestBlockRemovesFriendshipAndUnblock(t *testing.T) {
	router, u := newTestRouterWithFriends(t, "alice", "bob")
	req := sendFriendRequest(t, router, u["alice"], u["bob"])
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/friends/requests/"+req.ID+"/accept", nil, u["bob"].token), http.StatusOK, "accept")

	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/blocked", map[string]string{"username": "bob"}, u["alice"].token), http.StatusCreated, "block by username")
	if ids := listFriendIDs(t, router, u["bob"]); len(ids) != 0 {
		t.Fatalf("friendship survived block: %v", ids)
	}

	rec := doJSON(t, router, http.MethodGet, "/api/v1/blocked", nil, u["alice"].token)
	var blocked struct {
		Blocked []struct {
			UserID   string `json:"user_id"`
			Username string `json:"username"`
		} `json:"blocked"`
	}
	decodeJSON(t, rec, &blocked)
	if len(blocked.Blocked) != 1 || blocked.Blocked[0].UserID != u["bob"].id || blocked.Blocked[0].Username != "bob" {
		t.Fatalf("blocked list: %s", rec.Body.String())
	}
	// Bob's own list is unaffected by being blocked.
	rec = doJSON(t, router, http.MethodGet, "/api/v1/blocked", nil, u["bob"].token)
	decodeJSON(t, rec, &blocked)
	if len(blocked.Blocked) != 0 {
		t.Fatalf("bob's blocked list: %s", rec.Body.String())
	}

	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/blocked", map[string]string{"user_id": u["alice"].id}, u["alice"].token), http.StatusBadRequest, "self block")
	expectStatus(t, doJSON(t, router, http.MethodDelete, "/api/v1/blocked/"+u["bob"].id, nil, u["alice"].token), http.StatusNoContent, "unblock")
	expectStatus(t, doJSON(t, router, http.MethodDelete, "/api/v1/blocked/"+u["bob"].id, nil, u["alice"].token), http.StatusNotFound, "unblock again")

	// Unblocking does not restore the friendship, but allows a new request.
	if ids := listFriendIDs(t, router, u["alice"]); len(ids) != 0 {
		t.Fatalf("unblock restored friendship: %v", ids)
	}
	sendFriendRequest(t, router, u["bob"], u["alice"])
}

func TestRemoveFriend(t *testing.T) {
	router, u := newTestRouterWithFriends(t, "alice", "bob")
	req := sendFriendRequest(t, router, u["alice"], u["bob"])
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/friends/requests/"+req.ID+"/accept", nil, u["bob"].token), http.StatusOK, "accept")

	expectStatus(t, doJSON(t, router, http.MethodDelete, "/api/v1/friends/"+u["alice"].id, nil, u["bob"].token), http.StatusNoContent, "remove")
	if ids := listFriendIDs(t, router, u["alice"]); len(ids) != 0 {
		t.Fatalf("friendship survived removal: %v", ids)
	}
	expectStatus(t, doJSON(t, router, http.MethodDelete, "/api/v1/friends/"+u["alice"].id, nil, u["bob"].token), http.StatusNotFound, "remove again")
}
