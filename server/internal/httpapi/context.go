package httpapi

import (
	"context"
	"net/http"

	"github.com/itswskkk/KMJG-Hub/server/internal/auth"
	"github.com/itswskkk/KMJG-Hub/server/internal/user"
)

type contextKey int

const authContextKey contextKey = iota

// authContext is what requireAuth attaches to an authenticated request's
// context: the resolved user and the raw bearer token (still needed by
// handlers such as logout that revoke the session by its token).
type authContext struct {
	User  *user.User
	Token string
}

// requireAuth resolves the request's bearer token to its user before
// calling next, or responds 401 and short-circuits. docs/ARCHITECTURE.md
// "Authorization Architecture": every protected Server operation must
// verify the authenticated user's current permissions before performing
// the operation; this is the shared entry point for that check.
func requireAuth(authService *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				writeError(w, http.StatusUnauthorized, "unauthorized", "A valid session token is required")
				return
			}

			u, err := authService.CurrentUser(r.Context(), token)
			if err != nil {
				writeAuthError(w, err)
				return
			}

			ctx := context.WithValue(r.Context(), authContextKey, authContext{User: u, Token: token})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// currentAuth panics if called on a request that did not pass through
// requireAuth; every route it's used from must be wrapped accordingly.
func currentAuth(r *http.Request) authContext {
	return r.Context().Value(authContextKey).(authContext)
}
