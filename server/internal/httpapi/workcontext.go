package httpapi

import (
	"errors"
	"net/http"

	"github.com/itswskkk/KMJG-Hub/server/internal/workcontext"
)

func (h *Handlers) handleListWorkContexts(w http.ResponseWriter, r *http.Request) {
	values, err := h.WorkContexts.List(r.Context(), currentAuth(r).User.ID, r.PathValue("id"))
	if err != nil {
		writeWorkContextError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"contexts": values})
}

type updateWorkContextRequest struct {
	Working       bool   `json:"working"`
	StatusMode    string `json:"status_mode"`
	CurrentBranch string `json:"current_branch"`
}

func (h *Handlers) handleUpdateWorkContext(w http.ResponseWriter, r *http.Request) {
	var req updateWorkContextRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}
	value, err := h.WorkContexts.Update(r.Context(), currentAuth(r).User.ID, r.PathValue("id"), req.Working, req.StatusMode, req.CurrentBranch)
	if err != nil {
		writeWorkContextError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func writeWorkContextError(w http.ResponseWriter, err error) {
	var validationErr *workcontext.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeFieldError(w, http.StatusBadRequest, "validation_error", validationErr.Message, validationErr.Field)
	case errors.Is(err, workcontext.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Project not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong, please try again")
	}
}
