package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/project"
)

type projectDTO struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type projectSummaryDTO struct {
	projectDTO
	MemberCount int    `json:"member_count"`
	Role        string `json:"role"`
}

type projectMemberDTO struct {
	ID               string  `json:"id"`
	Username         string  `json:"username"`
	Role             string  `json:"role"`
	CurrentTaskTitle *string `json:"current_task_title"`
}

type projectDetailDTO struct {
	projectDTO
	Role    string             `json:"role"`
	Members []projectMemberDTO `json:"members"`
}

func toProjectDTO(p project.Project) projectDTO {
	return projectDTO{ID: p.ID, Name: p.Name, Description: p.Description, CreatedAt: p.CreatedAt}
}

func toProjectSummaryDTO(s project.Summary) projectSummaryDTO {
	return projectSummaryDTO{
		projectDTO:  toProjectDTO(s.Project),
		MemberCount: s.MemberCount,
		Role:        string(s.ViewerRole),
	}
}

func toProjectDetailDTO(d *project.Detail) projectDetailDTO {
	members := make([]projectMemberDTO, len(d.Members))
	for i, m := range d.Members {
		members[i] = projectMemberDTO{ID: m.UserID, Username: m.Username, Role: string(m.Role), CurrentTaskTitle: m.CurrentTaskTitle}
	}
	return projectDetailDTO{
		projectDTO: toProjectDTO(d.Project),
		Role:       string(d.ViewerRole),
		Members:    members,
	}
}

type createProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *Handlers) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	authed := currentAuth(r)

	var req createProjectRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}

	p, err := h.Projects.Create(r.Context(), authed.User.ID, project.CreateInput{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		writeProjectError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toProjectSummaryDTO(project.Summary{
		Project:     *p,
		MemberCount: 1,
		ViewerRole:  project.RoleOwner,
	}))
}

func (h *Handlers) handleListProjects(w http.ResponseWriter, r *http.Request) {
	authed := currentAuth(r)

	summaries, err := h.Projects.List(r.Context(), authed.User.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list projects")
		return
	}

	dtos := make([]projectSummaryDTO, len(summaries))
	for i, s := range summaries {
		dtos[i] = toProjectSummaryDTO(s)
	}

	writeJSON(w, http.StatusOK, map[string]any{"projects": dtos})
}

func (h *Handlers) handleGetProject(w http.ResponseWriter, r *http.Request) {
	authed := currentAuth(r)
	id := r.PathValue("id")

	detail, err := h.Projects.GetDetail(r.Context(), authed.User.ID, id)
	if err != nil {
		if errors.Is(err, project.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "Project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to load project")
		return
	}

	writeJSON(w, http.StatusOK, toProjectDetailDTO(detail))
}

func (h *Handlers) handleRemoveProjectMember(w http.ResponseWriter, r *http.Request) {
	authed := currentAuth(r)
	err := h.Projects.RemoveMember(r.Context(), authed.User.ID, r.PathValue("id"), r.PathValue("userID"))
	if err == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch {
	case errors.Is(err, project.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Project or member not found")
	case errors.Is(err, project.ErrCannotRemoveSelf):
		writeError(w, http.StatusForbidden, "cannot_remove_self", "Leave the project using the leave-project flow")
	case errors.Is(err, project.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to remove this member")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Could not remove the project member")
	}
}

type transferOwnershipRequest struct {
	NewOwnerID        string `json:"new_owner_id"`
	PreviousOwnerRole string `json:"previous_owner_role"`
}

func (h *Handlers) handleTransferProjectOwnership(w http.ResponseWriter, r *http.Request) {
	authed := currentAuth(r)
	var req transferOwnershipRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}
	err := h.Projects.TransferOwnership(r.Context(), authed.User.ID, r.PathValue("id"), req.NewOwnerID, project.Role(req.PreviousOwnerRole))
	if err != nil {
		writeProjectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type updateMemberRoleRequest struct {
	Role string `json:"role"`
}

func (h *Handlers) handleUpdateProjectMemberRole(w http.ResponseWriter, r *http.Request) {
	authed := currentAuth(r)
	var req updateMemberRoleRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}
	err := h.Projects.UpdateMemberRole(r.Context(), authed.User.ID, r.PathValue("id"), r.PathValue("userID"), project.Role(req.Role))
	if err != nil {
		writeProjectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleLeaveProject(w http.ResponseWriter, r *http.Request) {
	authed := currentAuth(r)
	if err := h.Projects.Leave(r.Context(), authed.User.ID, r.PathValue("id")); err != nil {
		writeProjectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	authed := currentAuth(r)
	if err := h.Projects.Delete(r.Context(), authed.User.ID, r.PathValue("id")); err != nil {
		writeProjectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleRestoreProject(w http.ResponseWriter, r *http.Request) {
	authed := currentAuth(r)
	if err := h.Projects.Restore(r.Context(), authed.User.ID, r.PathValue("id")); err != nil {
		writeProjectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type deletedProjectDTO struct {
	projectDTO
	MemberCount     int       `json:"member_count"`
	DeletedAt       time.Time `json:"deleted_at"`
	RestoreDeadline time.Time `json:"restore_deadline"`
}

func toDeletedProjectDTO(d project.DeletedSummary) deletedProjectDTO {
	return deletedProjectDTO{
		projectDTO:      toProjectDTO(d.Project),
		MemberCount:     d.MemberCount,
		DeletedAt:       d.DeletedAt,
		RestoreDeadline: d.RestoreDeadline,
	}
}

func (h *Handlers) handleListDeletedProjects(w http.ResponseWriter, r *http.Request) {
	authed := currentAuth(r)
	deleted, err := h.Projects.ListDeleted(r.Context(), authed.User.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list deleted projects")
		return
	}
	dtos := make([]deletedProjectDTO, len(deleted))
	for i, d := range deleted {
		dtos[i] = toDeletedProjectDTO(d)
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": dtos})
}

func writeProjectError(w http.ResponseWriter, err error) {
	var validationErr *project.ValidationError
	if errors.As(err, &validationErr) {
		writeFieldError(w, http.StatusBadRequest, "validation_error", validationErr.Message, validationErr.Field)
		return
	}
	switch {
	case errors.Is(err, project.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Project or member not found")
	case errors.Is(err, project.ErrNotOwner):
		writeError(w, http.StatusForbidden, "not_owner", "Only the project owner can do this")
	case errors.Is(err, project.ErrOwnerCannotLeave):
		writeError(w, http.StatusConflict, "owner_cannot_leave", "Transfer ownership to another member before leaving the project")
	case errors.Is(err, project.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to do this")
	case errors.Is(err, project.ErrCannotTransferToSelf):
		writeError(w, http.StatusBadRequest, "cannot_transfer_to_self", "Choose another member to become the owner")
	case errors.Is(err, project.ErrCannotDemoteOwner):
		writeError(w, http.StatusConflict, "cannot_change_owner_role", "The owner's role changes only through ownership transfer")
	case errors.Is(err, project.ErrRestoreWindowExpired):
		writeError(w, http.StatusGone, "restore_window_expired", "This project can no longer be restored")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong, please try again")
	}
}
