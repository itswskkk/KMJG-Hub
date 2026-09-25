package project

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/storage"
)

// Service implements Project creation, retrieval and lifecycle.
type Service struct {
	Repo Repository

	// Lifecycle backs TransferOwnership, UpdateMemberRole, Leave, Delete,
	// Restore, ListDeleted and PurgeExpiredDeleted. It is separate from
	// Repo (see LifecycleRepository); callers that only need creation and
	// retrieval may leave it nil.
	Lifecycle LifecycleRepository

	// Storage, when set, lets PurgeExpiredDeleted remove the stored files
	// of purged Projects' chat attachments.
	Storage ObjectDeleter
}

// ObjectDeleter is the narrow slice of internal/storage.Store that the
// retention sweep needs.
type ObjectDeleter interface {
	Delete(ctx context.Context, id string) error
}

// CreateInput carries the minimum information required to create a
// Project, per docs/UX.md "Project Creation Experience": "The creation flow
// should request only the information necessary to start the Project."
// Repository setup (Create New GitHub Repository / Connect Existing /
// Set Up Later) is part of that same UX flow, but GitHub integration is out
// of scope for this checkpoint (see docs/ARCHITECTURE.md "GitHub
// Authentication" deferral note in internal/auth) — every Project created
// here is created without a repository, which is the "Set Up Later" path
// and a fully valid v1 Project state per docs/PRD.md "Repository Setup
// During Project Creation".
type CreateInput struct {
	Name        string
	Description string
}

// Create creates a new Project owned by userID.
func (s *Service) Create(ctx context.Context, userID string, in CreateInput) (*Project, error) {
	name := strings.TrimSpace(in.Name)
	if err := validateName(name); err != nil {
		return nil, err
	}

	description := strings.TrimSpace(in.Description)
	if err := validateDescription(description); err != nil {
		return nil, err
	}

	p := &Project{
		Name:        name,
		Description: description,
		CreatedBy:   userID,
	}
	if err := s.Repo.CreateWithOwner(ctx, p, userID); err != nil {
		return nil, err
	}
	return p, nil
}

// List returns the Projects userID belongs to.
func (s *Service) List(ctx context.Context, userID string) ([]Summary, error) {
	return s.Repo.ListForUser(ctx, userID)
}

// GetDetail returns projectID's detail if userID is a member.
func (s *Service) GetDetail(ctx context.Context, userID, projectID string) (*Detail, error) {
	return s.Repo.GetDetailForUser(ctx, projectID, userID)
}

// RemoveMember removes targetUserID from projectID when userID's Project
// role permits it. Leaving a Project is deliberately a separate flow: in
// particular, an Owner must transfer ownership before leaving.
func (s *Service) RemoveMember(ctx context.Context, userID, projectID, targetUserID string) error {
	if userID == targetUserID {
		return ErrCannotRemoveSelf
	}

	detail, err := s.Repo.GetDetailForUser(ctx, projectID, userID)
	if err != nil {
		return err
	}

	var targetRole Role
	found := false
	for _, member := range detail.Members {
		if member.UserID == targetUserID {
			targetRole = member.Role
			found = true
			break
		}
	}
	if !found {
		return ErrNotFound
	}

	allowed := (detail.ViewerRole == RoleOwner && targetRole != RoleOwner) ||
		(detail.ViewerRole == RoleAdmin && targetRole == RoleMember)
	if !allowed {
		return ErrForbidden
	}

	return s.Repo.RemoveMember(ctx, projectID, userID, targetUserID)
}

// ProjectIDsForUser returns the IDs of Projects userID currently belongs
// to. Satisfies internal/presence.ProjectMembership so the presence system
// can derive which Projects' member lists should include userID's
// connection state, entirely from Server-known membership.
func (s *Service) ProjectIDsForUser(ctx context.Context, userID string) ([]string, error) {
	summaries, err := s.Repo.ListForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(summaries))
	for i, summary := range summaries {
		ids[i] = summary.ID
	}
	return ids, nil
}

// MemberUserIDs returns the user IDs of projectID's current members.
// Satisfies internal/presence.ProjectMembership.
func (s *Service) MemberUserIDs(ctx context.Context, projectID string) ([]string, error) {
	return s.Repo.ListMemberUserIDs(ctx, projectID)
}

// TransferOwnership makes newOwnerID the Owner of projectID. userID must be
// the current Owner and keeps membership with previousOwnerRole (Admin or
// Member), per docs/PRD.md "Ownership Transfer".
func (s *Service) TransferOwnership(ctx context.Context, userID, projectID, newOwnerID string, previousOwnerRole Role) error {
	if err := validateNonOwnerRole("previous_owner_role", previousOwnerRole); err != nil {
		return err
	}
	if userID == newOwnerID {
		return ErrCannotTransferToSelf
	}
	detail, err := s.Repo.GetDetailForUser(ctx, projectID, userID)
	if err != nil {
		return err
	}
	if detail.ViewerRole != RoleOwner {
		return ErrNotOwner
	}
	if _, ok := findMember(detail, newOwnerID); !ok {
		return ErrNotFound
	}
	return s.Lifecycle.TransferOwnership(ctx, projectID, userID, newOwnerID, previousOwnerRole)
}

// UpdateMemberRole promotes a Member to Admin or demotes an Admin to
// Member. docs/PRD.md "Roles and Permissions" grants this only to the
// Owner; the Owner's own role changes only through TransferOwnership.
func (s *Service) UpdateMemberRole(ctx context.Context, userID, projectID, targetUserID string, newRole Role) error {
	if err := validateNonOwnerRole("role", newRole); err != nil {
		return err
	}
	detail, err := s.Repo.GetDetailForUser(ctx, projectID, userID)
	if err != nil {
		return err
	}
	if detail.ViewerRole != RoleOwner {
		return ErrNotOwner
	}
	target, ok := findMember(detail, targetUserID)
	if !ok {
		return ErrNotFound
	}
	if target.Role == RoleOwner {
		return ErrCannotDemoteOwner
	}
	return s.Lifecycle.UpdateMemberRole(ctx, projectID, userID, targetUserID, newRole)
}

// Leave removes userID from projectID. Members and Admins may leave; the
// Owner gets ErrOwnerCannotLeave until ownership has been transferred.
func (s *Service) Leave(ctx context.Context, userID, projectID string) error {
	return s.Lifecycle.Leave(ctx, projectID, userID)
}

// Delete soft-deletes projectID. Only the Owner may delete a Project.
func (s *Service) Delete(ctx context.Context, userID, projectID string) error {
	detail, err := s.Repo.GetDetailForUser(ctx, projectID, userID)
	if err != nil {
		return err
	}
	if detail.ViewerRole != RoleOwner {
		return ErrNotOwner
	}
	return s.Lifecycle.Delete(ctx, projectID, userID)
}

// Restore restores a soft-deleted Project for the Owner who deleted it,
// within RestoreWindow.
func (s *Service) Restore(ctx context.Context, userID, projectID string) error {
	return s.Lifecycle.Restore(ctx, projectID, userID)
}

// ListDeleted returns userID's restorable deleted Projects.
func (s *Service) ListDeleted(ctx context.Context, userID string) ([]DeletedSummary, error) {
	return s.Lifecycle.ListDeleted(ctx, userID)
}

// PurgeExpiredDeleted permanently deletes Projects whose RestoreWindow
// ended before now, along with their stored chat attachment files. The
// database rows go first: a file left behind by a failed file delete is
// unreachable, whereas deleting files first could break a Project whose
// row delete then failed.
func (s *Service) PurgeExpiredDeleted(ctx context.Context, now time.Time) error {
	storageIDs, err := s.Lifecycle.PurgeDeletedBefore(ctx, now.Add(-RestoreWindow))
	if err != nil {
		return err
	}
	if s.Storage == nil {
		return nil
	}
	var errs []error
	for _, id := range storageIDs {
		if err := s.Storage.Delete(ctx, id); err != nil && !errors.Is(err, storage.ErrNotFound) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func findMember(detail *Detail, userID string) (Member, bool) {
	for _, m := range detail.Members {
		if m.UserID == userID {
			return m, true
		}
	}
	return Member{}, false
}

func validateNonOwnerRole(field string, role Role) error {
	if role != RoleAdmin && role != RoleMember {
		return &ValidationError{Field: field, Message: "must be admin or member"}
	}
	return nil
}
