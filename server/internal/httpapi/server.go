package httpapi

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/itswskkk/KMJG-Hub/server/internal/auth"
	"github.com/itswskkk/KMJG-Hub/server/internal/presence"
	"github.com/itswskkk/KMJG-Hub/server/internal/project"
	"github.com/itswskkk/KMJG-Hub/server/internal/realtime"
)

// Handlers holds the application services the HTTP layer dispatches to.
// Handlers translate network requests into calls against these services
// rather than containing business rules themselves, per
// docs/ARCHITECTURE.md "Server Internal Architecture".
type Handlers struct {
	Auth     *auth.Service
	Projects *project.Service
	Realtime *realtime.Hub
	Presence *presence.Service

	// AuthTimeout overrides how long a newly upgraded WebSocket connection
	// has to send its auth message (see defaultAuthTimeout). Zero means use
	// the default; tests set this to a short value instead of sleeping
	// through the production timeout.
	AuthTimeout time.Duration

	wsUpgrader websocket.Upgrader
}

// NewRouter builds the KMJG Hub HTTP API, mounted under the versioned
// /api/v1 prefix per docs/ARCHITECTURE.md "API Versioning".
func NewRouter(h *Handlers, allowedOrigins []string) http.Handler {
	h.wsUpgrader = newWebSocketUpgrader(allowedOrigins)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", handleHealth)

	mux.HandleFunc("POST /api/v1/auth/register", h.handleRegister)
	mux.HandleFunc("POST /api/v1/auth/login", h.handleLogin)

	authed := requireAuth(h.Auth)
	mux.Handle("POST /api/v1/auth/logout", authed(http.HandlerFunc(h.handleLogout)))
	mux.Handle("GET /api/v1/auth/session", authed(http.HandlerFunc(h.handleCurrentSession)))

	mux.Handle("POST /api/v1/projects", authed(http.HandlerFunc(h.handleCreateProject)))
	mux.Handle("GET /api/v1/projects", authed(http.HandlerFunc(h.handleListProjects)))
	mux.Handle("GET /api/v1/projects/{id}", authed(http.HandlerFunc(h.handleGetProject)))

	// Not wrapped in requireAuth: a browser's native WebSocket API cannot
	// set an Authorization header on the upgrade request, so this
	// connection authenticates itself via its first application message
	// instead (see handleWebSocket). The HTTP upgrade succeeding grants no
	// access by itself.
	mux.HandleFunc("GET /api/v1/ws", h.handleWebSocket)

	return corsMiddleware(allowedOrigins)(mux)
}
