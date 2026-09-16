// Package project implements KMJG Hub Projects: the project-first
// collaboration unit described throughout docs/PRD.md "Core Product Model"
// and docs/UX.md "Server Home" / "Project Workspace" / "Project Overview".
//
// This checkpoint implements only what the Load Projects -> Select Project
// -> Overview vertical slice needs: creating a project (the creator becomes
// its Owner, per docs/PRD.md "Project Creation"), listing a user's
// projects, and reading one project's detail (info + members) for a member
// of that project. Invitations, ownership transfer, deletion, and Git
// repository connection are separate, larger pieces of
// docs/ARCHITECTURE.md and are intentionally not implemented here.
package project

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned both when a project does not exist and when the
// requesting user is not one of its members. docs/ARCHITECTURE.md
// "Resource-Level Authorization": Project data must only be accessible to
// authorized members; collapsing "doesn't exist" and "not a member" into
// one response avoids revealing a private project's existence to
// non-members.
var ErrNotFound = errors.New("project: not found")

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
	UserID   string
	Username string
	Role     Role
	JoinedAt time.Time
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
}
