package httpapi

import (
	"errors"
	"html"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/github"
)

// maxWebhookBytes bounds a webhook delivery body (GitHub caps payloads at
// 25 MB; push payloads are far smaller).
const maxWebhookBytes = 25 << 20

type githubStatusDTO struct {
	Configured  bool       `json:"configured"`
	Connected   bool       `json:"connected"`
	Login       string     `json:"login,omitempty"`
	ConnectedAt *time.Time `json:"connected_at,omitempty"`
}

type githubAvailableRepoDTO struct {
	ExternalID    int64  `json:"external_repo_id"`
	OwnerLogin    string `json:"owner_login"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
}

type githubRepositoryDTO struct {
	ID                string    `json:"id"`
	ProjectID         string    `json:"project_id"`
	Provider          string    `json:"provider"`
	ExternalID        int64     `json:"external_repo_id"`
	OwnerLogin        string    `json:"owner_login"`
	Name              string    `json:"name"`
	FullName          string    `json:"full_name"`
	HTMLURL           string    `json:"html_url"`
	DefaultBranch     string    `json:"default_branch"`
	ConnectedByUserID string    `json:"connected_by_user_id"`
	ConnectedAt       time.Time `json:"connected_at"`
	PostPushesToChat  bool      `json:"post_pushes_to_chat"`
	NotifyAllMembers  bool      `json:"notify_all_members"`
	NotifyAllBranches bool      `json:"notify_all_branches"`
}

type projectRepositoryInfoDTO struct {
	Configured bool                 `json:"configured"`
	Repository *githubRepositoryDTO `json:"repository"`
}

func toGitHubRepositoryDTO(r *github.Repository) *githubRepositoryDTO {
	return &githubRepositoryDTO{
		ID: r.ID, ProjectID: r.ProjectID, Provider: r.Provider, ExternalID: r.ExternalID,
		OwnerLogin: r.OwnerLogin, Name: r.Name, FullName: r.OwnerLogin + "/" + r.Name,
		HTMLURL: r.HTMLURL, DefaultBranch: r.DefaultBranch,
		ConnectedByUserID: r.ConnectedByUserID, ConnectedAt: r.ConnectedAt,
		PostPushesToChat: r.PostPushesToChat, NotifyAllMembers: r.NotifyAllMembers,
		NotifyAllBranches: r.NotifyAllBranches,
	}
}

func (h *Handlers) handleGitHubStatus(w http.ResponseWriter, r *http.Request) {
	status := githubStatusDTO{Configured: h.GitHub.Configured()}
	identity, err := h.GitHub.GetIdentity(r.Context(), currentAuth(r).User.ID)
	switch {
	case err == nil:
		status.Connected, status.Login, status.ConnectedAt = true, identity.GitHubLogin, &identity.ConnectedAt
	case !errors.Is(err, github.ErrNotConnected):
		writeGitHubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleGitHubAuthorize returns (rather than redirects to) the GitHub
// authorization URL: the Desktop Client calls this with its bearer token
// and then opens the URL in the system browser.
func (h *Handlers) handleGitHubAuthorize(w http.ResponseWriter, r *http.Request) {
	authURL, state, err := h.GitHub.StartOAuth(r.Context(), currentAuth(r).User.ID)
	if err != nil {
		writeGitHubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"authorize_url": authURL, "state": state})
}

// handleGitHubCallback is GitHub's OAuth redirect target, reached in the
// user's system browser, which carries no KMJG Hub session. It is therefore
// not wrapped in requireAuth: the signed, expiring state issued by
// handleGitHubAuthorize to an authenticated user identifies who is linking
// their account. It responds with a small HTML page for the browser.
func (h *Handlers) handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if denied := q.Get("error"); denied != "" {
		writeCallbackPage(w, http.StatusBadRequest, "GitHub was not connected", "GitHub reported: "+denied+". You can close this window.")
		return
	}
	identity, err := h.GitHub.CompleteOAuth(r.Context(), q.Get("code"), q.Get("state"))
	if err != nil {
		status, _, message := githubErrorInfo(err)
		if status == http.StatusInternalServerError {
			slog.Error("httpapi: github oauth callback", "error", err)
		}
		writeCallbackPage(w, status, "GitHub was not connected", message)
		return
	}
	writeCallbackPage(w, http.StatusOK, "GitHub connected",
		"Your KMJG Hub account is now linked to GitHub account "+identity.GitHubLogin+". You can close this window and return to KMJG Hub.")
}

func writeCallbackPage(w http.ResponseWriter, status int, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, "<!doctype html><html><head><meta charset=\"utf-8\"><title>"+html.EscapeString(title)+
		"</title></head><body style=\"font-family:sans-serif;max-width:32rem;margin:4rem auto\"><h1>"+
		html.EscapeString(title)+"</h1><p>"+html.EscapeString(message)+"</p></body></html>")
}

func (h *Handlers) handleGitHubDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := h.GitHub.DisconnectAccount(r.Context(), currentAuth(r).User.ID); err != nil {
		writeGitHubError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleListGitHubRepositories(w http.ResponseWriter, r *http.Request) {
	repos, err := h.GitHub.ListAvailableRepositories(r.Context(), currentAuth(r).User.ID)
	if err != nil {
		writeGitHubError(w, err)
		return
	}
	dtos := make([]githubAvailableRepoDTO, len(repos))
	for i, repo := range repos {
		dtos[i] = githubAvailableRepoDTO{
			ExternalID: repo.ExternalID, OwnerLogin: repo.OwnerLogin, Name: repo.Name,
			FullName: repo.OwnerLogin + "/" + repo.Name, HTMLURL: repo.HTMLURL,
			Private: repo.Private, DefaultBranch: repo.DefaultBranch,
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"repositories": dtos})
}

func (h *Handlers) handleConnectProjectRepository(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExternalRepoID int64 `json:"external_repo_id"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}
	repo, err := h.GitHub.ConnectRepository(r.Context(), currentAuth(r).User.ID, r.PathValue("id"), body.ExternalRepoID)
	if err != nil {
		writeGitHubError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toGitHubRepositoryDTO(repo))
}

// handleGetProjectRepository returns 200 with a null repository when the
// Project has none, so the Client need not treat "no repository" as an error.
func (h *Handlers) handleGetProjectRepository(w http.ResponseWriter, r *http.Request) {
	info := projectRepositoryInfoDTO{Configured: h.GitHub.Configured()}
	repo, err := h.GitHub.FindRepository(r.Context(), currentAuth(r).User.ID, r.PathValue("id"))
	if err != nil {
		writeGitHubError(w, err)
		return
	}
	if repo != nil {
		info.Repository = toGitHubRepositoryDTO(repo)
	}
	writeJSON(w, http.StatusOK, info)
}

func (h *Handlers) handleDisconnectProjectRepository(w http.ResponseWriter, r *http.Request) {
	if err := h.GitHub.DisconnectRepository(r.Context(), currentAuth(r).User.ID, r.PathValue("id")); err != nil {
		writeGitHubError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleUpdateGitHubNotifications(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PostToChat  *bool `json:"post_to_chat"`
		AllMembers  *bool `json:"all_members"`
		AllBranches *bool `json:"all_branches"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}
	if body.PostToChat == nil || body.AllMembers == nil || body.AllBranches == nil {
		writeError(w, http.StatusBadRequest, "validation_error", "post_to_chat, all_members and all_branches are required")
		return
	}
	repo, err := h.GitHub.UpdateNotificationConfig(r.Context(), currentAuth(r).User.ID, r.PathValue("id"),
		*body.PostToChat, *body.AllMembers, *body.AllBranches)
	if err != nil {
		writeGitHubError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toGitHubRepositoryDTO(repo))
}

// handleGitHubWebhook receives GitHub webhook deliveries. It is not wrapped
// in requireAuth: GitHub authenticates each delivery with an HMAC-SHA256
// signature over the body using the shared webhook secret, which the
// service verifies before parsing anything.
func (h *Handlers) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Webhook payload is too large")
		return
	}
	err = h.GitHub.HandleWebhook(r.Context(), r.Header.Get("X-GitHub-Event"), payload, r.Header.Get("X-Hub-Signature-256"))
	if err != nil {
		writeGitHubError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func githubErrorInfo(err error) (int, string, string) {
	var validationErr *github.ValidationError
	switch {
	case errors.As(err, &validationErr):
		return http.StatusBadRequest, "validation_error", validationErr.Message
	case errors.Is(err, github.ErrNotConfigured):
		return http.StatusServiceUnavailable, "github_not_configured", "GitHub integration is not configured on this server"
	case errors.Is(err, github.ErrNotConnected):
		return http.StatusConflict, "github_not_connected", "Connect your GitHub account first"
	case errors.Is(err, github.ErrTokenRejected):
		return http.StatusConflict, "github_token_rejected", "GitHub rejected your access token; reconnect your GitHub account"
	case errors.Is(err, github.ErrInvalidOAuthState):
		return http.StatusBadRequest, "invalid_oauth_state", "The GitHub authorization request is invalid or has expired; please try again"
	case errors.Is(err, github.ErrIdentityInUse):
		return http.StatusConflict, "github_identity_in_use", "This GitHub account is already linked to another KMJG Hub user"
	case errors.Is(err, github.ErrAlreadyConnected):
		return http.StatusConflict, "repository_already_connected", "This Project already has a connected repository"
	case errors.Is(err, github.ErrRepositoryUnavailable):
		return http.StatusForbidden, "repository_unavailable", "That repository is not accessible to your GitHub account"
	case errors.Is(err, github.ErrForbidden):
		return http.StatusForbidden, "forbidden", "You do not have permission to manage this Project's repository"
	case errors.Is(err, github.ErrNotFound):
		return http.StatusNotFound, "not_found", "Not found"
	case errors.Is(err, github.ErrInvalidSignature):
		return http.StatusUnauthorized, "invalid_signature", "Webhook signature verification failed"
	default:
		return http.StatusInternalServerError, "internal_error", "Something went wrong, please try again"
	}
}

func writeGitHubError(w http.ResponseWriter, err error) {
	status, code, message := githubErrorInfo(err)
	if status == http.StatusInternalServerError {
		slog.Error("httpapi: github", "error", err)
	}
	var validationErr *github.ValidationError
	if errors.As(err, &validationErr) {
		writeFieldError(w, status, code, message, validationErr.Field)
		return
	}
	writeError(w, status, code, message)
}
