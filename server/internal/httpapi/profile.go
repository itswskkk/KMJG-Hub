package httpapi

import (
	"errors"
	"net/http"
	"sort"

	"github.com/itswskkk/KMJG-Hub/server/internal/profile"
)

// profilePrivacySettingDTO is one field's privacy configuration, as used in
// both a profileDTO's Privacy list (own profile only) and
// updateProfilePrivacyRequest's Settings list.
type profilePrivacySettingDTO struct {
	Field    string `json:"field"`
	Audience string `json:"audience"`
}

// profileDTO is a user profile as returned by the profile endpoints.
// Email and Privacy are only ever populated for the viewer's own profile
// (see internal/profile.Service.GetPublicProfile, which always clears
// them); omitempty keeps them out of a public profile's JSON entirely
// rather than serializing them as empty/null.
type profileDTO struct {
	UserID         string                     `json:"user_id"`
	Username       string                     `json:"username"`
	Email          string                     `json:"email,omitempty"`
	DisplayName    *string                    `json:"display_name,omitempty"`
	Avatar         *string                    `json:"avatar,omitempty"`
	Bio            *string                    `json:"bio,omitempty"`
	Presence       *string                    `json:"presence,omitempty"`
	CurrentProject *string                    `json:"current_project,omitempty"`
	CurrentTask    *string                    `json:"current_task,omitempty"`
	CurrentBranch  *string                    `json:"current_branch,omitempty"`
	Repositories   []string                   `json:"repositories,omitempty"`
	Privacy        []profilePrivacySettingDTO `json:"privacy,omitempty"`
}

func toProfileDTO(p *profile.Profile) profileDTO {
	dto := profileDTO{
		UserID:         p.UserID,
		Username:       p.Username,
		Email:          p.Email,
		DisplayName:    p.DisplayName,
		Avatar:         p.Avatar,
		Bio:            p.Bio,
		Presence:       p.Presence,
		CurrentProject: p.CurrentProject,
		CurrentTask:    p.CurrentTask,
		CurrentBranch:  p.CurrentBranch,
		Repositories:   p.Repositories,
	}
	if p.Privacy != nil {
		dto.Privacy = make([]profilePrivacySettingDTO, 0, len(p.Privacy))
		for field, audience := range p.Privacy {
			dto.Privacy = append(dto.Privacy, profilePrivacySettingDTO{Field: string(field), Audience: string(audience)})
		}
		// Map iteration order is random; sort so responses (and tests
		// asserting on them) are deterministic.
		sort.Slice(dto.Privacy, func(i, j int) bool { return dto.Privacy[i].Field < dto.Privacy[j].Field })
	}
	return dto
}

// handleGetOwnProfile serves GET /api/v1/users/me: the authenticated
// user's own full profile, including email and privacy settings, with no
// privacy filtering applied.
func (h *Handlers) handleGetOwnProfile(w http.ResponseWriter, r *http.Request) {
	p, err := h.Profiles.GetOwnProfile(r.Context(), currentAuth(r).User.ID)
	if err != nil {
		writeProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProfileDTO(p))
}

// handleGetPublicProfile serves GET /api/v1/users/{id}/profile: another
// user's profile as seen by the authenticated viewer, filtered by that
// user's privacy settings and their relationship to the viewer.
func (h *Handlers) handleGetPublicProfile(w http.ResponseWriter, r *http.Request) {
	p, err := h.Profiles.GetPublicProfile(r.Context(), r.PathValue("id"), currentAuth(r).User.ID)
	if err != nil {
		writeProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProfileDTO(p))
}

// updateProfileRequest carries only the fields being changed:
// internal/profile.Service.UpdateProfile treats a nil field as "leave
// unchanged" and a non-nil (possibly empty, to clear) field as "set this",
// so an omitted JSON key must decode to nil rather than the zero string.
type updateProfileRequest struct {
	DisplayName *string `json:"display_name"`
	Avatar      *string `json:"avatar"`
	Bio         *string `json:"bio"`
}

// handleUpdateProfile serves PUT /api/v1/users/profile: updates the
// authenticated user's own display name, avatar, and/or bio.
func (h *Handlers) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	var req updateProfileRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}

	userID := currentAuth(r).User.ID
	if err := h.Profiles.UpdateProfile(r.Context(), userID, req.DisplayName, req.Avatar, req.Bio); err != nil {
		writeProfileError(w, err)
		return
	}

	p, err := h.Profiles.GetOwnProfile(r.Context(), userID)
	if err != nil {
		writeProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProfileDTO(p))
}

// updateProfilePrivacyRequest carries the field-audience pairs to set. Any
// field not included is left at its current setting (or, if never set, the
// Service's own default), per docs/PRD.md "Field-level privacy" letting a
// user change one field at a time without resending every setting.
type updateProfilePrivacyRequest struct {
	Settings []profilePrivacySettingDTO `json:"settings"`
}

// handleSetPrivacy serves PUT /api/v1/users/profile/privacy: sets the
// visibility audience for one or more profile fields on the authenticated
// user's own profile.
func (h *Handlers) handleSetPrivacy(w http.ResponseWriter, r *http.Request) {
	var req updateProfilePrivacyRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}

	userID := currentAuth(r).User.ID
	for _, setting := range req.Settings {
		if err := h.Profiles.SetPrivacy(r.Context(), userID, profile.Field(setting.Field), profile.Audience(setting.Audience)); err != nil {
			writeProfileError(w, err)
			return
		}
	}

	p, err := h.Profiles.GetOwnProfile(r.Context(), userID)
	if err != nil {
		writeProfileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProfileDTO(p))
}

func writeProfileError(w http.ResponseWriter, err error) {
	var validationErr *profile.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeFieldError(w, http.StatusBadRequest, "validation_error", validationErr.Message, validationErr.Field)
	case errors.Is(err, profile.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "User not found")
	case errors.Is(err, profile.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to view this profile")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong, please try again")
	}
}
