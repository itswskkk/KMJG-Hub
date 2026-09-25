package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/project"
)

// usernameLookup is the sliver of fakeUserRepo that fakeProjectRepo needs to
// resolve a member's username at read time, mirroring how the real
// ProjectRepository joins project_members against the users table rather
// than duplicating usernames into project storage.
type usernameLookup interface {
	usernameFor(userID string) string
}

// fakeProjectRepo is a minimal in-memory project.Repository, mirroring the
// fakeUserRepo/fakeSessionRepo pattern in auth_test.go, so handler tests can
// exercise the full HTTP contract without a live PostgreSQL instance.
type fakeProjectRepo struct {
	mu       sync.Mutex
	nextID   int
	projects map[string]*project.Project
	members  map[string][]membership // projectID -> members, in join order
	users    usernameLookup
}

type membership struct {
	UserID   string
	Role     project.Role
	JoinedAt time.Time
}

func newFakeProjectRepo(users usernameLookup) *fakeProjectRepo {
	return &fakeProjectRepo{
		projects: make(map[string]*project.Project),
		members:  make(map[string][]membership),
		users:    users,
	}
}

func (f *fakeProjectRepo) CreateWithOwner(_ context.Context, p *project.Project, ownerID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nextID++
	p.ID = fmt.Sprintf("project-%d", f.nextID)
	p.CreatedAt = time.Now().UTC()
	stored := *p
	f.projects[p.ID] = &stored

	f.members[p.ID] = []membership{{
		UserID:   ownerID,
		Role:     project.RoleOwner,
		JoinedAt: p.CreatedAt,
	}}
	return nil
}

func (f *fakeProjectRepo) ListForUser(_ context.Context, userID string) ([]project.Summary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var out []project.Summary
	for id, p := range f.projects {
		for _, m := range f.members[id] {
			if m.UserID == userID {
				out = append(out, project.Summary{
					Project:     *p,
					MemberCount: len(f.members[id]),
					ViewerRole:  m.Role,
				})
				break
			}
		}
	}
	return out, nil
}

// addMember is a test-only helper for exercising multi-member scenarios
// (e.g. presence fan-out) that the product has no HTTP-facing way to reach
// yet, since Join/Invite is not implemented. It manipulates the fake
// repository's in-memory state directly and has no product-facing
// counterpart.
func (f *fakeProjectRepo) addMember(projectID, userID string, role project.Role) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.members[projectID] = append(f.members[projectID], membership{
		UserID:   userID,
		Role:     role,
		JoinedAt: time.Now().UTC(),
	})
}

func (f *fakeProjectRepo) ListMemberUserIDs(_ context.Context, projectID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]string, len(f.members[projectID]))
	for i, m := range f.members[projectID] {
		ids[i] = m.UserID
	}
	return ids, nil
}

func (f *fakeProjectRepo) RemoveMember(_ context.Context, projectID, actorUserID, targetUserID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	var actorRole, targetRole project.Role
	actorFound, targetFound := false, false
	for _, member := range f.members[projectID] {
		if member.UserID == actorUserID {
			actorRole, actorFound = member.Role, true
		}
		if member.UserID == targetUserID {
			targetRole, targetFound = member.Role, true
		}
	}
	if !actorFound || !targetFound {
		return project.ErrNotFound
	}
	if !((actorRole == project.RoleOwner && (targetRole == project.RoleAdmin || targetRole == project.RoleMember)) ||
		(actorRole == project.RoleAdmin && targetRole == project.RoleMember)) {
		return project.ErrForbidden
	}

	for i, member := range f.members[projectID] {
		if member.UserID == targetUserID {
			f.members[projectID] = append(f.members[projectID][:i], f.members[projectID][i+1:]...)
			return nil
		}
	}
	return project.ErrNotFound
}

func (f *fakeProjectRepo) GetDetailForUser(_ context.Context, projectID, userID string) (*project.Detail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	p, ok := f.projects[projectID]
	if !ok {
		return nil, project.ErrNotFound
	}

	var viewerRole project.Role
	found := false
	for _, m := range f.members[projectID] {
		if m.UserID == userID {
			viewerRole = m.Role
			found = true
			break
		}
	}
	if !found {
		return nil, project.ErrNotFound
	}

	members := make([]project.Member, len(f.members[projectID]))
	for i, m := range f.members[projectID] {
		members[i] = project.Member{
			UserID:   m.UserID,
			Username: f.users.usernameFor(m.UserID),
			Role:     m.Role,
			JoinedAt: m.JoinedAt,
		}
	}

	return &project.Detail{
		Project:    *p,
		ViewerRole: viewerRole,
		Members:    members,
	}, nil
}

func TestCreateAndListAndGetProject(t *testing.T) {
	router := newTestRouter()

	regRec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"username": "korn", "email": "korn@example.com", "password": "hunter22222",
	}, "")
	if regRec.Code != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d: %s", regRec.Code, regRec.Body.String())
	}
	token := extractToken(t, regRec)

	createRec := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{
		"name": "KMJG Hub Development", "description": "Core project",
	}, token)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create project: expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var created struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		MemberCount int    `json:"member_count"`
		Role        string `json:"role"`
	}
	decodeJSON(t, createRec, &created)
	if created.Role != "owner" {
		t.Fatalf("expected creator role owner, got %s", created.Role)
	}
	if created.MemberCount != 1 {
		t.Fatalf("expected member_count 1, got %d", created.MemberCount)
	}

	listRec := doJSON(t, router, http.MethodGet, "/api/v1/projects", nil, token)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list projects: expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	var listBody struct {
		Projects []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"projects"`
	}
	decodeJSON(t, listRec, &listBody)
	if len(listBody.Projects) != 1 || listBody.Projects[0].ID != created.ID {
		t.Fatalf("expected list to contain the created project, got %+v", listBody.Projects)
	}

	getRec := doJSON(t, router, http.MethodGet, "/api/v1/projects/"+created.ID, nil, token)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get project: expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}
	var detail struct {
		Name    string `json:"name"`
		Role    string `json:"role"`
		Members []struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"members"`
	}
	decodeJSON(t, getRec, &detail)
	if detail.Name != "KMJG Hub Development" {
		t.Fatalf("expected project name to round-trip, got %s", detail.Name)
	}
	if len(detail.Members) != 1 || detail.Members[0].Username != "korn" || detail.Members[0].Role != "owner" {
		t.Fatalf("expected one owner member named korn, got %+v", detail.Members)
	}
}

func TestGetProjectNotFoundForNonMember(t *testing.T) {
	router := newTestRouter()

	ownerRec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"username": "korn", "email": "korn@example.com", "password": "hunter22222",
	}, "")
	ownerToken := extractToken(t, ownerRec)

	createRec := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Private Project"}, ownerToken)
	var created struct {
		ID string `json:"id"`
	}
	decodeJSON(t, createRec, &created)

	outsiderRec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"username": "meran", "email": "meran@example.com", "password": "hunter22222",
	}, "")
	outsiderToken := extractToken(t, outsiderRec)

	getRec := doJSON(t, router, http.MethodGet, "/api/v1/projects/"+created.ID, nil, outsiderToken)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-member access, got %d: %s", getRec.Code, getRec.Body.String())
	}
}

func TestCreateProjectRequiresAuth(t *testing.T) {
	router := newTestRouter()
	rec := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "No Auth"}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a token, got %d", rec.Code)
	}
}

func TestCreateProjectValidatesName(t *testing.T) {
	router := newTestRouter()
	regRec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"username": "korn", "email": "korn@example.com", "password": "hunter22222",
	}, "")
	token := extractToken(t, regRec)

	rec := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": ""}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty name, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRemoveProjectMemberEnforcesRolesAndPreservesOwner(t *testing.T) {
	router, _, projectRepo := newTestRouterWithHandlers()

	register := func(username string) (string, string) {
		t.Helper()
		rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register", map[string]string{
			"username": username, "email": username + "@example.com", "password": "hunter22222",
		}, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("register %s: expected 201, got %d: %s", username, rec.Code, rec.Body.String())
		}
		var body struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		}
		decodeJSON(t, rec, &body)
		return body.User.ID, extractToken(t, rec)
	}

	ownerID, ownerToken := register("owner")
	adminID, adminToken := register("admin")
	memberID, memberToken := register("member")
	secondMemberID, _ := register("membertwo")

	createRec := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Team Project"}, ownerToken)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create project: expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	decodeJSON(t, createRec, &created)
	projectRepo.addMember(created.ID, adminID, project.RoleAdmin)
	projectRepo.addMember(created.ID, memberID, project.RoleMember)
	projectRepo.addMember(created.ID, secondMemberID, project.RoleMember)

	adminRemoveOwner := doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+created.ID+"/members/"+ownerID, nil, adminToken)
	if adminRemoveOwner.Code != http.StatusForbidden {
		t.Fatalf("admin removing owner: expected 403, got %d: %s", adminRemoveOwner.Code, adminRemoveOwner.Body.String())
	}
	adminRemoveMember := doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+created.ID+"/members/"+secondMemberID, nil, adminToken)
	if adminRemoveMember.Code != http.StatusNoContent {
		t.Fatalf("admin removing member: expected 204, got %d: %s", adminRemoveMember.Code, adminRemoveMember.Body.String())
	}
	adminRemoveAdmin := doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+created.ID+"/members/"+adminID, nil, ownerToken)
	if adminRemoveAdmin.Code != http.StatusNoContent {
		t.Fatalf("owner removing admin: expected 204, got %d: %s", adminRemoveAdmin.Code, adminRemoveAdmin.Body.String())
	}

	memberRemoveSelf := doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+created.ID+"/members/"+memberID, nil, memberToken)
	if memberRemoveSelf.Code != http.StatusForbidden {
		t.Fatalf("member removing self: expected 403, got %d: %s", memberRemoveSelf.Code, memberRemoveSelf.Body.String())
	}

	ownerRemoveMember := doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+created.ID+"/members/"+memberID, nil, ownerToken)
	if ownerRemoveMember.Code != http.StatusNoContent {
		t.Fatalf("owner removing member: expected 204, got %d: %s", ownerRemoveMember.Code, ownerRemoveMember.Body.String())
	}
	removedMemberGet := doJSON(t, router, http.MethodGet, "/api/v1/projects/"+created.ID, nil, memberToken)
	if removedMemberGet.Code != http.StatusNotFound {
		t.Fatalf("removed member reading project: expected 404, got %d: %s", removedMemberGet.Code, removedMemberGet.Body.String())
	}
}
