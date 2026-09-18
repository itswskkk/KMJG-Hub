package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/itswskkk/KMJG-Hub/server/internal/httpapi"
	"github.com/itswskkk/KMJG-Hub/server/internal/presence"
)

// These are integration-style tests: a real net/http test server plus a
// real gorilla/websocket client, exercising the full WebSocket handshake,
// auth-as-first-message, and presence protocol end to end. Lower-level
// concurrency/lifecycle behavior of the connection registry itself is
// covered without any network server in internal/realtime and
// internal/presence's own test suites.

type wsEnvelope struct {
	V    int             `json:"v"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func wsURL(t *testing.T, server *httptest.Server, path string) string {
	t.Helper()
	return "ws" + strings.TrimPrefix(server.URL, "http") + path
}

func dial(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL(t, server, "/api/v1/ws"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func registerAndToken(t *testing.T, router http.Handler, username string) string {
	t.Helper()
	rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"username": username, "email": username + "@example.com", "password": "hunter22222",
	}, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s: expected 201, got %d: %s", username, rec.Code, rec.Body.String())
	}
	return extractToken(t, rec)
}

func readEnvelope(t *testing.T, conn *websocket.Conn) wsEnvelope {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var env wsEnvelope
	if err := conn.ReadJSON(&env); err != nil {
		t.Fatalf("read envelope: %v", err)
	}
	return env
}

func expectNoMoreMessages(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	var env wsEnvelope
	err := conn.ReadJSON(&env)
	if err == nil {
		t.Fatalf("expected no further messages, got %+v", env)
	}
}

func expectClosed(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return // any error (close frame, EOF, reset) counts as closed
		}
	}
}

func TestWebSocketRejectsConnectionWithoutAuthMessage(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	server := httptest.NewServer(router)
	defer server.Close()

	conn := dial(t, server)
	// Send something that is not an auth message at all.
	if err := conn.WriteJSON(map[string]string{"type": "hello"}); err != nil {
		t.Fatalf("write: %v", err)
	}

	expectClosed(t, conn)
}

// TestWebSocketClosesUnauthenticatedConnectionAfterTimeout verifies that a
// connection which never sends anything is not held open indefinitely — it
// must be closed once its auth timeout elapses. Handlers.AuthTimeout is set
// on this test's own Handlers instance (never a shared package-level var,
// which would race with other tests' concurrently running servers under
// -race) so this test doesn't need to sleep through the real production
// value.
func TestWebSocketClosesUnauthenticatedConnectionAfterTimeout(t *testing.T) {
	_, handlers, _ := newTestRouterWithHandlers()
	handlers.AuthTimeout = 100 * time.Millisecond
	router := httpapi.NewRouter(handlers, []string{"http://localhost:1420"})

	server := httptest.NewServer(router)
	defer server.Close()

	conn := dial(t, server)
	// Send nothing at all.
	expectClosed(t, conn)
}

func TestWebSocketRejectsInvalidToken(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	server := httptest.NewServer(router)
	defer server.Close()

	conn := dial(t, server)
	if err := conn.WriteJSON(map[string]any{
		"type": "auth",
		"data": map[string]string{"token": "not-a-real-token"},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	env := readEnvelope(t, conn)
	if env.Type != "error" {
		t.Fatalf("expected an error envelope, got %q", env.Type)
	}
	expectClosed(t, conn)
}

func TestWebSocketRejectsRevokedSession(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	server := httptest.NewServer(router)
	defer server.Close()

	token := registerAndToken(t, router, "korn")

	logoutRec := doJSON(t, router, http.MethodPost, "/api/v1/auth/logout", nil, token)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("logout: expected 204, got %d", logoutRec.Code)
	}

	conn := dial(t, server)
	if err := conn.WriteJSON(map[string]any{"type": "auth", "data": map[string]string{"token": token}}); err != nil {
		t.Fatalf("write: %v", err)
	}

	env := readEnvelope(t, conn)
	if env.Type != "error" {
		t.Fatalf("expected an error envelope for a revoked session, got %q", env.Type)
	}
	expectClosed(t, conn)
}

func TestWebSocketAcceptsValidSessionAndSendsConnectedAck(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	server := httptest.NewServer(router)
	defer server.Close()

	token := registerAndToken(t, router, "korn")

	conn := dial(t, server)
	if err := conn.WriteJSON(map[string]any{"type": "auth", "data": map[string]string{"token": token}}); err != nil {
		t.Fatalf("write: %v", err)
	}

	env := readEnvelope(t, conn)
	if env.Type != "connected" {
		t.Fatalf("expected a connected ack, got %q", env.Type)
	}
	if env.V != 1 {
		t.Fatalf("expected protocol version 1, got %d", env.V)
	}
}

func TestWebSocketSendsPresenceSnapshotForOwnProject(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	server := httptest.NewServer(router)
	defer server.Close()

	token := registerAndToken(t, router, "korn")
	createRec := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "KMJG Hub Development"}, token)
	var project struct {
		ID string `json:"id"`
	}
	decodeJSON(t, createRec, &project)

	conn := dial(t, server)
	if err := conn.WriteJSON(map[string]any{"type": "auth", "data": map[string]string{"token": token}}); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = readEnvelope(t, conn) // "connected"

	env := readEnvelope(t, conn)
	if env.Type != presence.EventSnapshot {
		t.Fatalf("expected %q, got %q", presence.EventSnapshot, env.Type)
	}
	var snapshot presence.SnapshotData
	if err := json.Unmarshal(env.Data, &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if snapshot.ProjectID != project.ID {
		t.Fatalf("expected snapshot for %s, got %s", project.ID, snapshot.ProjectID)
	}
	if len(snapshot.Members) != 1 || snapshot.Members[0].UserID == "" || !snapshot.Members[0].Online {
		t.Fatalf("expected exactly one online member (self), got %+v", snapshot.Members)
	}
}

func TestWebSocketBroadcastsPresenceToAuthorizedMemberOnly(t *testing.T) {
	router, _, projectRepo := newTestRouterWithHandlers()
	server := httptest.NewServer(router)
	defer server.Close()

	ownerToken := registerAndToken(t, router, "korn")
	createRec := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Shared Project"}, ownerToken)
	var project struct {
		ID string `json:"id"`
	}
	decodeJSON(t, createRec, &project)

	memberToken := registerAndToken(t, router, "meran")
	outsiderToken := registerAndToken(t, router, "jai") // never added to the project

	memberRec := doJSON(t, router, http.MethodGet, "/api/v1/auth/session", nil, memberToken)
	var member struct {
		ID string `json:"id"`
	}
	decodeJSON(t, memberRec, &member)
	projectRepo.addMember(project.ID, member.ID, "member")

	memberConn := dial(t, server)
	if err := memberConn.WriteJSON(map[string]any{"type": "auth", "data": map[string]string{"token": memberToken}}); err != nil {
		t.Fatalf("member write: %v", err)
	}
	_ = readEnvelope(t, memberConn) // connected
	_ = readEnvelope(t, memberConn) // presence.snapshot

	outsiderConn := dial(t, server)
	if err := outsiderConn.WriteJSON(map[string]any{"type": "auth", "data": map[string]string{"token": outsiderToken}}); err != nil {
		t.Fatalf("outsider write: %v", err)
	}
	_ = readEnvelope(t, outsiderConn) // connected (no snapshot: outsider belongs to no project)

	// Owner connects: the authorized member should learn about it; the
	// outsider (not a member of "Shared Project") must not.
	ownerConn := dial(t, server)
	if err := ownerConn.WriteJSON(map[string]any{"type": "auth", "data": map[string]string{"token": ownerToken}}); err != nil {
		t.Fatalf("owner write: %v", err)
	}
	_ = readEnvelope(t, ownerConn) // connected
	_ = readEnvelope(t, ownerConn) // presence.snapshot

	updateEnv := readEnvelope(t, memberConn)
	if updateEnv.Type != presence.EventUpdated {
		t.Fatalf("expected %q, got %q", presence.EventUpdated, updateEnv.Type)
	}
	var updated presence.UpdatedData
	if err := json.Unmarshal(updateEnv.Data, &updated); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	if updated.ProjectID != project.ID || !updated.Online {
		t.Fatalf("unexpected update for authorized member: %+v", updated)
	}

	expectNoMoreMessages(t, outsiderConn)
}

func TestWebSocketLogoutClosesConnection(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	server := httptest.NewServer(router)
	defer server.Close()

	token := registerAndToken(t, router, "korn")

	conn := dial(t, server)
	if err := conn.WriteJSON(map[string]any{"type": "auth", "data": map[string]string{"token": token}}); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = readEnvelope(t, conn) // connected

	logoutRec := doJSON(t, router, http.MethodPost, "/api/v1/auth/logout", nil, token)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("logout: expected 204, got %d", logoutRec.Code)
	}

	expectClosed(t, conn)
}

func TestWebSocketRejectsOversizedMessage(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	server := httptest.NewServer(router)
	defer server.Close()

	conn := dial(t, server)
	huge := strings.Repeat("a", 8192) // larger than the server's read limit
	if err := conn.WriteJSON(map[string]any{"type": "auth", "data": map[string]string{"token": huge}}); err != nil {
		t.Fatalf("write: %v", err)
	}

	expectClosed(t, conn)
}

func TestWebSocketIgnoresMalformedMessageAfterAuth(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	server := httptest.NewServer(router)
	defer server.Close()

	token := registerAndToken(t, router, "korn")

	conn := dial(t, server)
	if err := conn.WriteJSON(map[string]any{"type": "auth", "data": map[string]string{"token": token}}); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = readEnvelope(t, conn) // connected (this user belongs to no Project, so no snapshot follows)

	// Send garbage; the Server must not crash and the connection must stay
	// usable (still respond to a ping from the Server, i.e. not silently
	// dead) — verified by successfully writing another (ignored) message
	// afterwards without an error.
	if err := conn.WriteMessage(websocket.TextMessage, []byte("not json at all")); err != nil {
		t.Fatalf("write garbage: %v", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"unexpected"}`)); err != nil {
		t.Fatalf("write unexpected type: %v", err)
	}

	expectNoMoreMessages(t, conn) // no crash, no reply, connection simply idles
}
