package httpapi

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/itswskkk/KMJG-Hub/server/internal/auth"
	"github.com/itswskkk/KMJG-Hub/server/internal/chat"
	"github.com/itswskkk/KMJG-Hub/server/internal/invitation"
	"github.com/itswskkk/KMJG-Hub/server/internal/presence"
	"github.com/itswskkk/KMJG-Hub/server/internal/project"
	"github.com/itswskkk/KMJG-Hub/server/internal/realtime"
	"github.com/itswskkk/KMJG-Hub/server/internal/task"
)

// Handlers holds the application services the HTTP layer dispatches to.
// Handlers translate network requests into calls against these services
// rather than containing business rules themselves, per
// docs/ARCHITECTURE.md "Server Internal Architecture".
type Handlers struct {
	Auth        *auth.Service
	Projects    *project.Service
	Invitations *invitation.Service
	Chat        *chat.Service
	Tasks       *task.Service
	Realtime    *realtime.Hub
	Presence    *presence.Service

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
	mux.Handle("DELETE /api/v1/projects/{id}/members/{userID}", authed(http.HandlerFunc(h.handleRemoveProjectMember)))
	mux.Handle("POST /api/v1/projects/{id}/invitations", authed(http.HandlerFunc(h.handleCreateDirectInvitation)))
	mux.Handle("GET /api/v1/projects/{id}/invitations", authed(http.HandlerFunc(h.handleListProjectInvitations)))
	mux.Handle("DELETE /api/v1/projects/{id}/invitations/{invitationID}", authed(http.HandlerFunc(h.handleCancelInvitation)))
	mux.Handle("GET /api/v1/invitations", authed(http.HandlerFunc(h.handleListReceivedInvitations)))
	mux.Handle("POST /api/v1/invitations/{id}/accept", authed(http.HandlerFunc(h.handleAcceptInvitation)))
	mux.Handle("POST /api/v1/invitations/{id}/decline", authed(http.HandlerFunc(h.handleDeclineInvitation)))
	mux.Handle("POST /api/v1/projects/{id}/invite-credentials", authed(http.HandlerFunc(h.handleCreateInviteCredential)))
	mux.Handle("GET /api/v1/projects/{id}/invite-credentials", authed(http.HandlerFunc(h.handleListInviteCredentials)))
	mux.Handle("DELETE /api/v1/projects/{id}/invite-credentials/{credentialID}", authed(http.HandlerFunc(h.handleRevokeInviteCredential)))
	mux.Handle("POST /api/v1/invitations/join", authed(http.HandlerFunc(h.handleJoinProjectWithInvite)))
	mux.Handle("GET /api/v1/projects/{id}/chat/messages", authed(http.HandlerFunc(h.handleListProjectMessages)))
	mux.Handle("POST /api/v1/projects/{id}/chat/messages", authed(http.HandlerFunc(h.handleSendProjectMessage)))
	mux.Handle("DELETE /api/v1/projects/{id}/chat/messages/{messageID}", authed(http.HandlerFunc(h.handleDeleteProjectMessage)))
	mux.Handle("GET /api/v1/projects/{id}/tasks", authed(http.HandlerFunc(h.handleListTasks)))
	mux.Handle("POST /api/v1/projects/{id}/tasks", authed(http.HandlerFunc(h.handleCreateTask)))
	mux.Handle("GET /api/v1/projects/{id}/tasks/{taskID}", authed(http.HandlerFunc(h.handleGetTask)))
	mux.Handle("PATCH /api/v1/projects/{id}/tasks/{taskID}/status", authed(http.HandlerFunc(h.handleSetTaskStatus)))
	mux.Handle("POST /api/v1/projects/{id}/tasks/{taskID}/assign", authed(http.HandlerFunc(h.handleAssignTask)))
	mux.Handle("POST /api/v1/projects/{id}/tasks/{taskID}/current", authed(http.HandlerFunc(h.handleSetCurrentTask)))
	mux.Handle("GET /api/v1/projects/{id}/tasks/{taskID}/comments", authed(http.HandlerFunc(h.handleListTaskComments)))
	mux.Handle("POST /api/v1/projects/{id}/tasks/{taskID}/comments", authed(http.HandlerFunc(h.handleAddTaskComment)))
	mux.Handle("POST /api/v1/task-assignment-requests/{requestID}/accept", authed(http.HandlerFunc(h.handleAcceptTaskAssignment)))
	mux.Handle("POST /api/v1/task-assignment-requests/{requestID}/decline", authed(http.HandlerFunc(h.handleDeclineTaskAssignment)))

	// Not wrapped in requireAuth: a browser's native WebSocket API cannot
	// set an Authorization header on the upgrade request, so this
	// connection authenticates itself via its first application message
	// instead (see handleWebSocket). The HTTP upgrade succeeding grants no
	// access by itself.
	mux.HandleFunc("GET /api/v1/ws", h.handleWebSocket)

	return corsMiddleware(allowedOrigins)(mux)
}
