package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/auth"
	"github.com/itswskkk/KMJG-Hub/server/internal/user"
)

type userDTO struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type sessionDTO struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type authResponse struct {
	User    userDTO    `json:"user"`
	Session sessionDTO `json:"session"`
}

func toUserDTO(u *user.User) userDTO {
	return userDTO{ID: u.ID, Username: u.Username, Email: u.Email, CreatedAt: u.CreatedAt}
}

func toAuthResponse(res *auth.Result) authResponse {
	return authResponse{
		User:    toUserDTO(res.User),
		Session: sessionDTO{Token: res.Token, ExpiresAt: res.ExpiresAt},
	}
}

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handlers) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}

	res, err := h.Auth.Register(r.Context(), auth.RegisterInput{
		Username: req.Username,
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toAuthResponse(res))
}

type loginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

func (h *Handlers) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}

	res, err := h.Auth.Login(r.Context(), auth.LoginInput{
		Identifier: req.Identifier,
		Password:   req.Password,
	})
	if err != nil {
		writeAuthError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toAuthResponse(res))
}

func (h *Handlers) handleLogout(w http.ResponseWriter, r *http.Request) {
	authed := currentAuth(r)

	if err := h.Auth.Logout(r.Context(), authed.Token); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to revoke session")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleCurrentSession(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, toUserDTO(currentAuth(r).User))
}

func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return "", false
	}
	return token, true
}

func writeAuthError(w http.ResponseWriter, err error) {
	var validationErr *auth.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeFieldError(w, http.StatusBadRequest, "validation_error", validationErr.Message, validationErr.Field)
	case errors.Is(err, user.ErrDuplicate):
		writeError(w, http.StatusConflict, "already_exists", "Username or email is already registered")
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Incorrect username, email, or password")
	case errors.Is(err, auth.ErrSessionInvalid):
		writeError(w, http.StatusUnauthorized, "invalid_session", "Your session is invalid or has expired. Please log in again.")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong, please try again")
	}
}
