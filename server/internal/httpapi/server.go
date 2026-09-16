package httpapi

import (
	"net/http"

	"github.com/itswskkk/KMJG-Hub/server/internal/auth"
	"github.com/itswskkk/KMJG-Hub/server/internal/project"
)

// Handlers holds the application services the HTTP layer dispatches to.
// Handlers translate network requests into calls against these services
// rather than containing business rules themselves, per
// docs/ARCHITECTURE.md "Server Internal Architecture".
type Handlers struct {
	Auth     *auth.Service
	Projects *project.Service
}

// NewRouter builds the KMJG Hub HTTP API, mounted under the versioned
// /api/v1 prefix per docs/ARCHITECTURE.md "API Versioning".
func NewRouter(h *Handlers, allowedOrigins []string) http.Handler {
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

	return corsMiddleware(allowedOrigins)(mux)
}
