package project

import (
	"context"
	"strings"
)

// Service implements Project creation and retrieval.
type Service struct {
	Repo Repository
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
