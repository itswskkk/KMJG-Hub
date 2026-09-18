package project_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/project"
)

// fakeRepo is a minimal in-memory project.Repository for exercising
// project.Service without a real PostgreSQL instance.
type fakeRepo struct {
	mu       sync.Mutex
	nextID   int
	projects map[string]*project.Project
	members  map[string][]project.Member
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		projects: make(map[string]*project.Project),
		members:  make(map[string][]project.Member),
	}
}

func (f *fakeRepo) CreateWithOwner(_ context.Context, p *project.Project, ownerID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	p.ID = fmt.Sprintf("project-%d", f.nextID)
	p.CreatedAt = time.Now().UTC()
	stored := *p
	f.projects[p.ID] = &stored
	f.members[p.ID] = []project.Member{{UserID: ownerID, Username: "owner", Role: project.RoleOwner, JoinedAt: p.CreatedAt}}
	return nil
}

func (f *fakeRepo) ListForUser(_ context.Context, userID string) ([]project.Summary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []project.Summary
	for id, p := range f.projects {
		for _, m := range f.members[id] {
			if m.UserID == userID {
				out = append(out, project.Summary{Project: *p, MemberCount: len(f.members[id]), ViewerRole: m.Role})
				break
			}
		}
	}
	return out, nil
}

func (f *fakeRepo) GetDetailForUser(_ context.Context, projectID, userID string) (*project.Detail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.projects[projectID]
	if !ok {
		return nil, project.ErrNotFound
	}
	for _, m := range f.members[projectID] {
		if m.UserID == userID {
			return &project.Detail{Project: *p, ViewerRole: m.Role, Members: f.members[projectID]}, nil
		}
	}
	return nil, project.ErrNotFound
}

func (f *fakeRepo) ListMemberUserIDs(_ context.Context, projectID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]string, len(f.members[projectID]))
	for i, m := range f.members[projectID] {
		ids[i] = m.UserID
	}
	return ids, nil
}

func TestProjectIDsForUserAndMemberUserIDs(t *testing.T) {
	svc := &project.Service{Repo: newFakeRepo()}
	ctx := context.Background()

	p, err := svc.Create(ctx, "user-1", project.CreateInput{Name: "KMJG Hub Development"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ids, err := svc.ProjectIDsForUser(ctx, "user-1")
	if err != nil {
		t.Fatalf("ProjectIDsForUser: %v", err)
	}
	if len(ids) != 1 || ids[0] != p.ID {
		t.Fatalf("expected [%s], got %v", p.ID, ids)
	}

	if ids, err := svc.ProjectIDsForUser(ctx, "user-2"); err != nil || len(ids) != 0 {
		t.Fatalf("expected no projects for a non-member, got %v (err=%v)", ids, err)
	}

	members, err := svc.MemberUserIDs(ctx, p.ID)
	if err != nil {
		t.Fatalf("MemberUserIDs: %v", err)
	}
	if len(members) != 1 || members[0] != "user-1" {
		t.Fatalf("expected [user-1], got %v", members)
	}
}

func TestCreateProjectMakesCreatorOwner(t *testing.T) {
	svc := &project.Service{Repo: newFakeRepo()}
	p, err := svc.Create(context.Background(), "user-1", project.CreateInput{Name: "KMJG Hub Development"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID == "" {
		t.Fatal("expected a generated project ID")
	}

	detail, err := svc.GetDetail(context.Background(), "user-1", p.ID)
	if err != nil {
		t.Fatalf("GetDetail: %v", err)
	}
	if detail.ViewerRole != project.RoleOwner {
		t.Fatalf("expected creator to be owner, got role %q", detail.ViewerRole)
	}
	if len(detail.Members) != 1 {
		t.Fatalf("expected exactly one member, got %d", len(detail.Members))
	}
}

func TestCreateProjectRejectsInvalidInput(t *testing.T) {
	svc := &project.Service{Repo: newFakeRepo()}

	cases := []struct {
		name  string
		input project.CreateInput
	}{
		{"empty name", project.CreateInput{Name: ""}},
		{"whitespace-only name", project.CreateInput{Name: "   "}},
		{"name too long", project.CreateInput{Name: strings.Repeat("a", 101)}},
		{"description too long", project.CreateInput{Name: "Valid Name", Description: strings.Repeat("a", 501)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var validationErr *project.ValidationError
			_, err := svc.Create(context.Background(), "user-1", tc.input)
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected ValidationError, got %v", err)
			}
		})
	}
}

func TestListProjectsOnlyReturnsMemberProjects(t *testing.T) {
	svc := &project.Service{Repo: newFakeRepo()}
	ctx := context.Background()

	if _, err := svc.Create(ctx, "user-1", project.CreateInput{Name: "Project A"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Create(ctx, "user-2", project.CreateInput{Name: "Project B"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err := svc.List(ctx, "user-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Name != "Project A" {
		t.Fatalf("expected only Project A for user-1, got %+v", list)
	}
}

func TestGetDetailRejectsNonMember(t *testing.T) {
	svc := &project.Service{Repo: newFakeRepo()}
	ctx := context.Background()

	p, err := svc.Create(ctx, "user-1", project.CreateInput{Name: "Private Project"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.GetDetail(ctx, "user-2", p.ID); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a non-member, got %v", err)
	}
}
