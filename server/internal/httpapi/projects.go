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

func writeProjectError(w http.ResponseWriter, err error) {
	var validationErr *project.ValidationError
	if errors.As(err, &validationErr) {
		writeFieldError(w, http.StatusBadRequest, "validation_error", validationErr.Message, validationErr.Field)
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong, please try again")
}
