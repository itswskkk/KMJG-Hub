package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/itswskkk/KMJG-Hub/server/internal/notification"
)

type notificationBody struct {
	ID        string          `json:"id"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
	ReadAt    *string         `json:"read_at"`
}

type notificationPageBody struct {
	Notifications []notificationBody `json:"notifications"`
	NextCursor    string             `json:"next_cursor"`
	UnreadCount   int                `json:"unread_count"`
}

func listNotifications(t *testing.T, router http.Handler, u friendUser, cursor string) notificationPageBody {
	t.Helper()
	path := "/api/v1/notifications"
	if cursor != "" {
		path += "?cursor=" + url.QueryEscape(cursor)
	}
	rec := doJSON(t, router, http.MethodGet, path, nil, u.token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list notifications: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var page notificationPageBody
	decodeJSON(t, rec, &page)
	return page
}

func newNotificationRouter(t *testing.T, names ...string) (http.Handler, *notification.Service, map[string]friendUser) {
	t.Helper()
	router, handlers, _ := newTestRouterWithHandlers()
	users := make(map[string]friendUser, len(names))
	for _, name := range names {
		token := registerAndToken(t, router, name)
		users[name] = friendUser{token: token, id: currentUserID(t, router, token)}
	}
	return router, handlers.Notifications, users
}

func TestNotificationEndpointsRequireAuth(t *testing.T) {
	router := newTestRouter()
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/notifications"},
		{http.MethodPost, "/api/v1/notifications/x/read"},
		{http.MethodDelete, "/api/v1/notifications/x"},
	} {
		if rec := doJSON(t, router, tc.method, tc.path, nil, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401, got %d", tc.method, tc.path, rec.Code)
		}
	}
}

func TestNotificationListPagination(t *testing.T) {
	router, svc, u := newNotificationRouter(t, "alice", "bob")
	ctx := context.Background()
	total := notification.RecentLimit + 3
	for i := 0; i < total; i++ {
		if _, err := svc.Create(ctx, u["alice"].id, notification.EventTaskComment, map[string]int{"i": i}); err != nil {
			t.Fatal(err)
		}
	}

	first := listNotifications(t, router, u["alice"], "")
	if len(first.Notifications) != notification.RecentLimit || first.NextCursor == "" || first.UnreadCount != total {
		t.Fatalf("first page: len=%d cursor=%q unread=%d", len(first.Notifications), first.NextCursor, first.UnreadCount)
	}
	if first.Notifications[0].EventType != "task_comment" || string(first.Notifications[0].Payload) == "" {
		t.Fatalf("first item: %+v", first.Notifications[0])
	}
	second := listNotifications(t, router, u["alice"], first.NextCursor)
	if len(second.Notifications) != 3 || second.NextCursor != "" {
		t.Fatalf("second page: len=%d cursor=%q", len(second.Notifications), second.NextCursor)
	}

	if bob := listNotifications(t, router, u["bob"], ""); len(bob.Notifications) != 0 || bob.UnreadCount != 0 {
		t.Fatalf("bob sees others' notifications: %+v", bob)
	}

	expectStatus(t, doJSON(t, router, http.MethodGet, "/api/v1/notifications?cursor=%21%21bad", nil, u["alice"].token), http.StatusBadRequest, "bad cursor")
}

func TestNotificationMarkReadAndDeleteOwnerOnly(t *testing.T) {
	router, svc, u := newNotificationRouter(t, "alice", "mallory")
	n, err := svc.Create(context.Background(), u["alice"].id, notification.EventDirectMessage, map[string]string{"body_preview": "hi"})
	if err != nil {
		t.Fatal(err)
	}

	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/notifications/"+n.ID+"/read", nil, u["mallory"].token), http.StatusNotFound, "non-owner mark read")
	expectStatus(t, doJSON(t, router, http.MethodDelete, "/api/v1/notifications/"+n.ID, nil, u["mallory"].token), http.StatusNotFound, "non-owner delete")
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/notifications/missing/read", nil, u["alice"].token), http.StatusNotFound, "missing mark read")

	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/notifications/"+n.ID+"/read", nil, u["alice"].token), http.StatusNoContent, "owner mark read")
	page := listNotifications(t, router, u["alice"], "")
	if page.UnreadCount != 0 || len(page.Notifications) != 1 || page.Notifications[0].ReadAt == nil {
		t.Fatalf("after read: %+v", page)
	}

	expectStatus(t, doJSON(t, router, http.MethodDelete, "/api/v1/notifications/"+n.ID, nil, u["alice"].token), http.StatusNoContent, "owner delete")
	expectStatus(t, doJSON(t, router, http.MethodDelete, "/api/v1/notifications/"+n.ID, nil, u["alice"].token), http.StatusNotFound, "repeat delete")
	if page := listNotifications(t, router, u["alice"], ""); len(page.Notifications) != 0 {
		t.Fatalf("after delete: %+v", page)
	}
}

func TestFriendRequestAndDirectMessageCreateNotifications(t *testing.T) {
	router, _, u := newNotificationRouter(t, "alice", "bob")

	req := sendFriendRequest(t, router, u["alice"], u["bob"])
	page := listNotifications(t, router, u["bob"], "")
	if len(page.Notifications) != 1 || page.Notifications[0].EventType != "friend_request" || page.UnreadCount != 1 {
		t.Fatalf("bob after friend request: %+v", page)
	}
	var fr map[string]string
	_ = json.Unmarshal(page.Notifications[0].Payload, &fr)
	if fr["sender_id"] != u["alice"].id || fr["request_id"] != req.ID || fr["sender_username"] != "alice" {
		t.Fatalf("friend_request payload: %v", fr)
	}
	if alice := listNotifications(t, router, u["alice"], ""); len(alice.Notifications) != 0 {
		t.Fatalf("sender should not be notified: %+v", alice)
	}

	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/friends/requests/"+req.ID+"/accept", nil, u["bob"].token), http.StatusOK, "accept")
	if code, _ := sendDM(t, router, u["bob"], u["alice"], "hello alice"); code != http.StatusCreated {
		t.Fatalf("dm send: %d", code)
	}
	page = listNotifications(t, router, u["alice"], "")
	if len(page.Notifications) != 1 || page.Notifications[0].EventType != "direct_message" {
		t.Fatalf("alice after DM: %+v", page)
	}
	var dm map[string]string
	_ = json.Unmarshal(page.Notifications[0].Payload, &dm)
	if dm["sender_id"] != u["bob"].id || dm["body_preview"] != "hello alice" || dm["message_id"] == "" {
		t.Fatalf("direct_message payload: %v", dm)
	}
}

func TestDirectInvitationCreatesNotification(t *testing.T) {
	router, _, u := newNotificationRouter(t, "owner", "invitee")
	created := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Notify Project"}, u["owner"].token)
	var p struct {
		ID string `json:"id"`
	}
	decodeJSON(t, created, &p)
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/projects/"+p.ID+"/invitations", map[string]string{"recipient": "invitee", "expires_in": "7d"}, u["owner"].token), http.StatusCreated, "invite")

	page := listNotifications(t, router, u["invitee"], "")
	if len(page.Notifications) != 1 || page.Notifications[0].EventType != "project_invitation" {
		t.Fatalf("invitee: %+v", page)
	}
	var payload map[string]string
	_ = json.Unmarshal(page.Notifications[0].Payload, &payload)
	if payload["project_id"] != p.ID || payload["project_name"] != "Notify Project" || payload["inviter_username"] != "owner" || payload["invitation_id"] == "" {
		t.Fatalf("project_invitation payload: %v", payload)
	}
}
