// Package project implements KMJG Hub Projects: the project-first
// collaboration unit described throughout docs/PRD.md "Core Product Model"
// and docs/UX.md "Server Home" / "Project Workspace" / "Project Overview".
//
// This checkpoint implements only what the Load Projects -> Select Project
// -> Overview vertical slice needs: creating a project (the creator becomes
// its Owner, per docs/PRD.md "Project Creation"), listing a user's
// projects, and reading one project's detail (info + members) for a member
// of that project. The Project lifecycle (ownership transfer, role
// changes, leaving, soft deletion and restore) lives behind the separate
// LifecycleRepository interface. Invitations and Git repository connection
// are implemented in their own packages.
package project

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned both when a project does not exist and when the
// requesting user is not one of its members. docs/ARCHITECTURE.md
// "Resource-Level Authorization": Project data must only be accessible to
// authorized members; collapsing "doesn't exist" and "not a member" into
// one response avoids revealing a private project's existence to
// non-members.
var ErrNotFound = errors.New("project: not found")

// ErrForbidden is returned when a project member is not allowed to perform
// a project-level operation.
var ErrForbidden = errors.New("project: forbidden")

// ErrCannotRemoveSelf keeps member removal separate from the Leave Project
// flow, whose Owner-specific ownership-transfer rule is different.
var ErrCannotRemoveSelf = errors.New("project: cannot remove self")

// Role is a Project-level role, per docs/PRD.md "Roles and Permissions".
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// Project is a KMJG Hub Project.
type Project struct {
	ID          string
	Name        string
	Description string // empty string means no description
	CreatedBy   string
	CreatedAt   time.Time
}

// Summary is a Project as shown in a list (docs/UX.md "Server Home"):
// enough to render "Your Projects" without fetching each project's full
// member list.
type Summary struct {
	Project
	MemberCount int
	ViewerRole  Role
}

// Member is a Project member as shown on the Overview / Members experience.
type Member struct {
	UserID           string
	Username         string
	Role             Role
	JoinedAt         time.Time
	CurrentTaskTitle *string
}

// Detail is a single Project's full data for the Overview screen.
type Detail struct {
	Project
	ViewerRole Role
	Members    []Member
}

// Repository persists and retrieves Projects and their membership.
type Repository interface {
	// CreateWithOwner creates p and adds ownerID as its Owner in the same
	// atomic operation, per docs/PRD.md "Project Creation": "the user who
	// creates the Project automatically becomes the initial Project
	// Owner." On success p.ID and p.CreatedAt are populated.
	CreateWithOwner(ctx context.Context, p *Project, ownerID string) error

	// ListForUser returns the Projects userID is a member of.
	ListForUser(ctx context.Context, userID string) ([]Summary, error)

	// GetDetailForUser returns projectID's detail if userID is one of its
	// members, or ErrNotFound otherwise.
	GetDetailForUser(ctx context.Context, projectID, userID string) (*Detail, error)

	// ListMemberUserIDs returns the user IDs of projectID's current
	// members, with no other membership data. Added for the presence
	// system (internal/presence.ProjectMembership): real-time presence
	// recipients must be derived from Server-known membership, per
	// docs/ARCHITECTURE.md "Event Authorization", never from a
	// Client-supplied Project ID alone.
	ListMemberUserIDs(ctx context.Context, projectID string) ([]string, error)

	// RemoveMember removes targetUserID only when actorUserID still has the
	// necessary role at the time of deletion. The repository must make this
	// authorization check part of the authoritative delete, not rely solely
	// on an earlier read in the service layer.
	RemoveMember(ctx context.Context, projectID, actorUserID, targetUserID string) error
}

// Lifecycle errors, per docs/PRD.md "Project Management". ErrNotOwner and
// ErrOwnerCannotLeave wrap ErrForbidden so callers that only distinguish
// "allowed / not allowed" keep working.
var (
	// ErrNotOwner is returned when an Owner-only operation (ownership
	// transfer, role changes, deletion) is attempted by an Admin or Member.
	ErrNotOwner = fmt.Errorf("%w: only the project owner may do this", ErrForbidden)

	// ErrOwnerCannotLeave: "The Owner cannot leave the Project until
	// ownership has been transferred to another member."
	ErrOwnerCannotLeave = fmt.Errorf("%w: owner must transfer ownership before leaving", ErrForbidden)

	// ErrCannotTransferToSelf is returned when the Owner names themselves
	// as the new Owner.
	ErrCannotTransferToSelf = errors.New("project: cannot transfer ownership to self")

	// ErrCannotDemoteOwner is returned when a role change targets the
	// current Owner; ownership only moves via TransferOwnership so the
	// Project always has exactly one Owner.
	ErrCannotDemoteOwner = errors.New("project: owner role can only change through ownership transfer")

	// ErrRestoreWindowExpired is returned when the deleting Owner tries to
	// restore a Project after its RestoreWindow has ended.
	ErrRestoreWindowExpired = errors.New("project: restore window expired")
)

// RestoreWindow is how long a soft-deleted Project stays restorable before
// the retention sweep permanently deletes it (docs/PRD.md "Project
// Deletion": "Its KMJG Hub project data is retained for 30 days").
const RestoreWindow = 30 * 24 * time.Hour

// DeletedSummary is a soft-deleted Project as listed in the "Recently
// Deleted Projects" recovery area.
type DeletedSummary struct {
	Project
	MemberCount     int
	DeletedAt       time.Time
	RestoreDeadline time.Time // DeletedAt + RestoreWindow
}

// LifecycleRepository persists Project lifecycle changes: ownership
// transfer, role changes, leaving, soft deletion and restore.
//
// It is deliberately separate from Repository: Repository has many
// in-memory fakes across the codebase's tests, and the lifecycle
// operations are only needed by project.Service's lifecycle methods.
// postgres.ProjectRepository implements both.
//
// Like Repository.RemoveMember, every mutating method must enforce its
// authorization rule inside the authoritative write (same statement or
// transaction), never relying only on an earlier read by Service. None of
// them may act on a soft-deleted Project, except Restore.
type LifecycleRepository interface {
	// TransferOwnership atomically makes newOwnerID the Owner and demotes
	// actorUserID (who must be the current Owner) to previousOwnerRole
	// (RoleAdmin or RoleMember). newOwnerID must be an existing non-Owner
	// member. Returns ErrForbidden if actorUserID isn't the Owner and
	// ErrNotFound if the Project or newOwnerID's membership doesn't exist.
	TransferOwnership(ctx context.Context, projectID, actorUserID, newOwnerID string, previousOwnerRole Role) error

	// UpdateMemberRole sets targetUserID's role to newRole (RoleAdmin or
	// RoleMember). Only the Owner may change roles, and the Owner's own
	// role cannot be changed this way. Returns ErrForbidden otherwise.
	UpdateMemberRole(ctx context.Context, projectID, actorUserID, targetUserID string, newRole Role) error

	// Leave removes userID from projectID. Returns ErrOwnerCannotLeave for
	// the Owner and ErrNotFound for a non-member.
	Leave(ctx context.Context, projectID, userID string) error

	// Delete soft-deletes projectID, recording actorUserID (who must be the
	// current Owner) as the deleting Owner. Returns ErrForbidden otherwise.
	Delete(ctx context.Context, projectID, actorUserID string) error

	// Restore un-deletes projectID and makes actorUserID its Owner again.
	// Only the user recorded as the deleting Owner may restore, and only
	// within RestoreWindow of deletion. Returns ErrRestoreWindowExpired
	// when the window has ended, and ErrNotFound when the Project is not
	// deleted or actorUserID is not its deleting Owner (not revealing the
	// Project to anyone else).
	Restore(ctx context.Context, projectID, actorUserID string) error

	// ListDeleted returns the Projects userID deleted as Owner that are
	// still within RestoreWindow, most recently deleted first.
	ListDeleted(ctx context.Context, userID string) ([]DeletedSummary, error)

	// PurgeDeletedBefore permanently deletes Projects soft-deleted before
	// cutoff (their dependent rows cascade) and returns the storage IDs of
	// the Project Chat attachments that belonged to them, so the caller can
	// remove the stored files.
	PurgeDeletedBefore(ctx context.Context, cutoff time.Time) ([]string, error)
}
