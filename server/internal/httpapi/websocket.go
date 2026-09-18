package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/gorilla/websocket"

	"github.com/itswskkk/KMJG-Hub/server/internal/auth"
	"github.com/itswskkk/KMJG-Hub/server/internal/realtime"
)

// Heartbeat/timeout constants for the authenticated WebSocket connection,
// per docs/ARCHITECTURE.md "Presence": "The real-time connection mechanism
// may use heartbeat, timeout, or equivalent connection-liveness detection
// to determine when a connection is no longer active. The exact heartbeat
// interval and timeout values will be selected during implementation."
//
// This follows the standard gorilla/websocket heartbeat pattern: the
// Server pings every pingPeriod; a Client (browser or otherwise) that
// answers control-frame pings keeps resetting its own read deadline via
// pongHandler, so a connection that stops responding — crash, network
// failure, unplugged cable — is detected and torn down within pongWait of
// its last successful pong, without requiring any application-level
// heartbeat message from the Client.
const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10

	// maxMessageBytes bounds every frame the Server will read from a
	// WebSocket connection, including the initial auth message. This is
	// deliberately small: the only Client-originated message this
	// checkpoint's protocol defines is `{"type":"auth","data":{"token":...}}`.
	maxMessageBytes = 4096
)

// defaultAuthTimeout bounds how long a newly upgraded connection has to
// send its auth message before the Server gives up and closes it, so an
// unauthenticated half-open connection cannot be held indefinitely.
// NewRouter uses this unless Handlers.AuthTimeout is set explicitly (tests
// shorten it there instead of sleeping through the real production value —
// a Handlers field rather than a shared package-level var, so concurrent
// requests never race over it).
const defaultAuthTimeout = 10 * time.Second

// newWebSocketUpgrader builds a gorilla/websocket Upgrader whose Origin
// check mirrors the HTTP API's CORS allow-list, per
// docs/ARCHITECTURE.md "Security Architecture" -> "Native Client Security"
// and this checkpoint's requirement for "origin handling appropriate to the
// actual deployment model." A request with no Origin header (a non-browser
// Client, e.g. a future Tauri-native connection) is allowed through, since
// Origin is a browser-enforced concept and the real authorization boundary
// here is the session token presented in the first message, not Origin.
func newWebSocketUpgrader(allowedOrigins []string) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			return slices.Contains(allowedOrigins, origin)
		},
	}
}

// handleWebSocket upgrades the connection, then requires the Client's first
// message to be an auth message carrying its existing Server-managed
// session token, per docs/ARCHITECTURE.md "Authentication Boundary": "The
// authenticated identity is used by both HTTP API requests and the
// real-time WebSocket connection." The token travels as the first
// application message rather than a URL query parameter: browsers' native
// WebSocket API cannot set an Authorization header on the upgrade request,
// and a query parameter would otherwise land in Server access logs and
// proxy logs, which docs/ARCHITECTURE.md "Session Security" rules out
// ("never be written to application logs").
//
// Once authenticated, the connection is registered with the Hub (making the
// user Online if this is their first live connection) and immediately sent
// its authorized initial presence snapshot, before the connection settles
// into its normal read/write pumps for the rest of its lifetime.
func (h *Handlers) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := h.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade already wrote an HTTP error response; nothing else to do.
		return
	}

	conn.SetReadLimit(maxMessageBytes)
	_ = conn.SetReadDeadline(time.Now().Add(h.authTimeout()))

	userID, tokenHash, ok := authenticateWebSocket(h.Auth, conn)
	if !ok {
		_ = conn.Close()
		return
	}

	client := realtime.NewClient(userID, tokenHash, func() { _ = conn.Close() })
	h.Realtime.Register(client)

	go writeWebSocketPump(conn, client)

	if connected, err := realtime.NewEnvelope("connected", struct{}{}); err == nil {
		client.Enqueue(connected)
	}

	snapshotCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := h.Presence.SendSnapshot(snapshotCtx, client); err != nil {
		slog.Warn("websocket: presence snapshot failed", "error", err)
	}
	cancel()

	readWebSocketPump(conn, client, h.Realtime)
}

func (h *Handlers) authTimeout() time.Duration {
	if h.AuthTimeout > 0 {
		return h.AuthTimeout
	}
	return defaultAuthTimeout
}

type wsClientMessage struct {
	Type string `json:"type"`
	Data struct {
		Token string `json:"token"`
	} `json:"data"`
}

// authenticateWebSocket reads exactly one message and requires it to be a
// valid auth message naming an active session, validated the same way as
// every HTTP request (auth.Service.CurrentUser against authoritative
// Server-side session state) — the WebSocket handshake succeeding is never
// by itself treated as proof of identity.
func authenticateWebSocket(authService *auth.Service, conn *websocket.Conn) (userID, tokenHash string, ok bool) {
	var msg wsClientMessage
	if err := conn.ReadJSON(&msg); err != nil {
		return "", "", false
	}
	if msg.Type != "auth" || msg.Data.Token == "" {
		writeWebSocketError(conn, "unauthorized", "Expected an auth message with a session token")
		return "", "", false
	}

	u, err := authService.CurrentUser(context.Background(), msg.Data.Token)
	if err != nil {
		writeWebSocketError(conn, "unauthorized", "Invalid or expired session")
		return "", "", false
	}

	return u.ID, auth.HashSessionToken(msg.Data.Token), true
}

func writeWebSocketError(conn *websocket.Conn, code, message string) {
	env, err := realtime.NewEnvelope("error", map[string]string{"code": code, "message": message})
	if err != nil {
		return
	}
	body, err := json.Marshal(env)
	if err != nil {
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
	_ = conn.WriteMessage(websocket.TextMessage, body)
}

// writeWebSocketPump owns all writes to conn for its lifetime: it relays
// client.Outbound() events and sends periodic protocol-level pings for
// liveness detection. Exits (and lets the deferred conn.Close() run) as
// soon as a write fails or client.StopSignal() closes, so a connection
// being torn down elsewhere (session sweep, logout, Server shutdown) does
// not leave this goroutine blocked forever — avoiding a goroutine leak per
// connection.
func writeWebSocketPump(conn *websocket.Conn, client *realtime.Client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()

	for {
		select {
		case <-client.StopSignal():
			return
		case env := <-client.Outbound():
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteJSON(env); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readWebSocketPump owns all reads from conn for its lifetime. This
// checkpoint's protocol defines no Client-originated messages after auth
// (docs/ARCHITECTURE.md "Event Authorization": authorized Project context
// is derived server-side, never a Client subscription), so every frame
// read here past the auth handshake is discarded — its only purpose is to
// drive the pong handler that keeps the read deadline (and therefore the
// connection) alive, and to detect the connection closing. Blocks until the
// connection ends for any reason, then unregisters from the Hub exactly
// once, from this single call site, so Hub bookkeeping never double-fires.
func readWebSocketPump(conn *websocket.Conn, client *realtime.Client, hub *realtime.Hub) {
	defer func() {
		hub.Unregister(client)
		client.Close()
	}()

	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
