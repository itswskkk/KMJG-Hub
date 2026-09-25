package httpapi

import (
	"errors"
	"net/http"

	"github.com/itswskkk/KMJG-Hub/server/internal/invitation"
)

type createDirectInvitationRequest struct {
	Recipient string `json:"recipient"`
	ExpiresIn string `json:"expires_in"`
}

type createInviteCredentialRequest struct {
	ExpiresIn string `json:"expires_in"`
	MaxUses   *int   `json:"max_uses"`
}

type joinProjectRequest struct {
	Invite string `json:"invite"`
}

func (h *Handlers) handleCreateDirectInvitation(w http.ResponseWriter, r *http.Request) {
	var req createDirectInvitationRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be valid JSON")
		return
	}
	authed := currentAuth(r)
	item, err := h.Invitations.CreateDirect(r.Context(), r.PathValue("id"), authed.User.ID, req.Recipient, req.ExpiresIn)
	if err != nil {
		writeInvitationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handlers) handleListReceivedInvitations(w http.ResponseWriter, r *http.Request) {
	items, err := h.Invitations.ListReceived(r.Context(), currentAuth(r).User.ID)
	if err != nil {
		writeInvitationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invitations": items})
}

func (h *Handlers) handleListProjectInvitations(w http.ResponseWriter, r *http.Request) {
	items, err := h.Invitations.ListForProject(r.Context(), r.PathValue("id"), currentAuth(r).User.ID)
	if err != nil {
		writeInvitationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invitations": items})
}

func (h *Handlers) handleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	projectID, err := h.Invitations.Accept(r.Context(), r.PathValue("id"), currentAuth(r).User.ID)
	if err != nil {
		writeInvitationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"project_id": projectID})
}

func (h *Handlers) handleDeclineInvitation(w http.ResponseWriter, r *http.Request) {
	if err := h.Invitations.Decline(r.Context(), r.PathValue("id"), currentAuth(r).User.ID); err != nil {
		writeInvitationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleCancelInvitation(w http.ResponseWriter, r *http.Request) {
	if err := h.Invitations.Cancel(r.Context(), r.PathValue("id"), r.PathValue("invitationID"), currentAuth(r).User.ID); err != nil {
		writeInvitationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleCreateInviteCredential(w http.ResponseWriter, r *http.Request) {
	var req createInviteCredentialRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be valid JSON")
		return
	}
	item, err := h.Invitations.CreateCredential(r.Context(), r.PathValue("id"), currentAuth(r).User.ID, req.ExpiresIn, req.MaxUses)
	if err != nil {
		writeInvitationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handlers) handleListInviteCredentials(w http.ResponseWriter, r *http.Request) {
	items, err := h.Invitations.ListCredentials(r.Context(), r.PathValue("id"), currentAuth(r).User.ID)
	if err != nil {
		writeInvitationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"credentials": items})
}

func (h *Handlers) handleRevokeInviteCredential(w http.ResponseWriter, r *http.Request) {
	if err := h.Invitations.RevokeCredential(r.Context(), r.PathValue("id"), r.PathValue("credentialID"), currentAuth(r).User.ID); err != nil {
		writeInvitationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleJoinProjectWithInvite(w http.ResponseWriter, r *http.Request) {
	var req joinProjectRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be valid JSON")
		return
	}
	projectID, err := h.Invitations.JoinWithCredential(r.Context(), req.Invite, currentAuth(r).User.ID)
	if err != nil {
		writeInvitationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"project_id": projectID})
}

func writeInvitationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, invitation.ErrInvalidExpiration):
		writeError(w, http.StatusBadRequest, "invalid_expiration", "Expiration must be 1h, 1d, 7d, 30d, or never")
	case errors.Is(err, invitation.ErrRecipientNotFound):
		writeError(w, http.StatusNotFound, "recipient_not_found", "Recipient was not found on this server")
	case errors.Is(err, invitation.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Invitation or project not found")
	case errors.Is(err, invitation.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to manage this invitation")
	case errors.Is(err, invitation.ErrConflict):
		writeError(w, http.StatusConflict, "invitation_conflict", "User is already a member or has a pending invitation")
	case errors.Is(err, invitation.ErrInvalidUseLimit):
		writeError(w, http.StatusBadRequest, "invalid_use_limit", "Maximum uses must be a positive number or unlimited")
	case errors.Is(err, invitation.ErrExpired):
		writeError(w, http.StatusGone, "invite_expired", "This invite has expired")
	case errors.Is(err, invitation.ErrRevoked):
		writeError(w, http.StatusGone, "invite_revoked", "This invite has been revoked")
	case errors.Is(err, invitation.ErrUsesExhausted):
		writeError(w, http.StatusGone, "invite_uses_exhausted", "This invite has reached its maximum uses")
	case errors.Is(err, invitation.ErrAlreadyMember):
		writeError(w, http.StatusConflict, "already_member", "You are already a member of this Project")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Invitation operation failed")
	}
}
