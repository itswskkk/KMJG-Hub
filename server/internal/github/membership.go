package github

import (
	"context"
	"errors"

	"github.com/itswskkk/KMJG-Hub/server/internal/project"
)

// ProjectMembership adapts *project.Service to Membership, translating
// project.ErrNotFound (not a member, or no such Project) into ErrNotFound.
type ProjectMembership struct {
	Projects *project.Service
}

func (m ProjectMembership) ViewerRole(ctx context.Context, projectID, userID string) (string, error) {
	detail, err := m.Projects.GetDetail(ctx, userID, projectID)
	if err != nil {
		if errors.Is(err, project.ErrNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	return string(detail.ViewerRole), nil
}

func (m ProjectMembership) MemberUserIDs(ctx context.Context, projectID string) ([]string, error) {
	return m.Projects.MemberUserIDs(ctx, projectID)
}
