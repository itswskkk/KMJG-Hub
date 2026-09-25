package httpapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/project"
)

type fakeDeletedProject struct {
	project   project.Project
	members   []membership
	deletedBy string
	deletedAt time.Time
}

// The methods below make fakeProjectRepo also satisfy
// project.LifecycleRepository. Deleting moves a project out of
// f.projects, so the Repository fake methods hide it unchanged.
var _ project.LifecycleRepository = (*fakeProjectRepo)(nil)

func (f *fakeProjectRepo) memberIndex(projectID, userID string) int {
	for i, m := range f.members[projectID] {
		if m.UserID == userID {
			return i
		}
	}
	return -1
}

func (f *fakeProjectRepo) TransferOwnership(_ context.Context, projectID, actorUserID, newOwnerID string, previousOwnerRole project.Role) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.projects[projectID]; !ok {
		return project.ErrNotFound
	}
	ai, ti := f.memberIndex(projectID, actorUserID), f.memberIndex(projectID, newOwnerID)
	if ai < 0 || f.members[projectID][ai].Role != project.RoleOwner {
		return project.ErrForbidden
	}
	if ti < 0 {
		return project.ErrNotFound
	}
	f.members[projectID][ai].Role = previousOwnerRole
	f.members[projectID][ti].Role = project.RoleOwner
	return nil
}

func (f *fakeProjectRepo) UpdateMemberRole(_ context.Context, projectID, actorUserID, targetUserID string, newRole project.Role) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ai, ti := f.memberIndex(projectID, actorUserID), f.memberIndex(projectID, targetUserID)
	if ai < 0 || ti < 0 || f.members[projectID][ai].Role != project.RoleOwner || f.members[projectID][ti].Role == project.RoleOwner {
		return project.ErrForbidden
	}
	f.members[projectID][ti].Role = newRole
	return nil
}

func (f *fakeProjectRepo) Leave(_ context.Context, projectID, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.memberIndex(projectID, userID)
	if _, ok := f.projects[projectID]; !ok || i < 0 {
		return project.ErrNotFound
	}
	if f.members[projectID][i].Role == project.RoleOwner {
		return project.ErrOwnerCannotLeave
	}
	f.members[projectID] = append(f.members[projectID][:i], f.members[projectID][i+1:]...)
	return nil
}

func (f *fakeProjectRepo) Delete(_ context.Context, projectID, actorUserID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.projects[projectID]
	if !ok {
		return project.ErrNotFound
	}
	if i := f.memberIndex(projectID, actorUserID); i < 0 || f.members[projectID][i].Role != project.RoleOwner {
		return project.ErrForbidden
	}
	if f.deleted == nil {
		f.deleted = make(map[string]*fakeDeletedProject)
	}
	f.deleted[projectID] = &fakeDeletedProject{project: *p, members: f.members[projectID], deletedBy: actorUserID, deletedAt: time.Now().UTC()}
	delete(f.projects, projectID)
	delete(f.members, projectID)
	return nil
}

func (f *fakeProjectRepo) Restore(_ context.Context, projectID, actorUserID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.deleted[projectID]
	if !ok || d.deletedBy != actorUserID {
		return project.ErrNotFound
	}
	if time.Since(d.deletedAt) >= project.RestoreWindow {
		return project.ErrRestoreWindowExpired
	}
	p := d.project
	f.projects[projectID] = &p
	f.members[projectID] = d.members
	delete(f.deleted, projectID)
	return nil
}

func (f *fakeProjectRepo) ListDeleted(_ context.Context, userID string) ([]project.DeletedSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []project.DeletedSummary
	for _, d := range f.deleted {
		deadline := d.deletedAt.Add(project.RestoreWindow)
		if d.deletedBy == userID && time.Now().Before(deadline) {
			out = append(out, project.DeletedSummary{Project: d.project, MemberCount: len(d.members), DeletedAt: d.deletedAt, RestoreDeadline: deadline})
		}
	}
	return out, nil
}

func (f *fakeProjectRepo) PurgeDeletedBefore(_ context.Context, cutoff time.Time) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, d := range f.deleted {
		if d.deletedAt.Before(cutoff) {
			delete(f.deleted, id)
		}
	}
	return nil, nil
}

// expireDeletion is a test-only helper that backdates a deletion past the
// restore window.
func (f *fakeProjectRepo) expireDeletion(projectID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted[projectID].deletedAt = time.Now().Add(-project.RestoreWindow - time.Hour)
}

type lifecycleUser struct{ id, token string }

func setupLifecycleProject(t *testing.T) (http.Handler, *fakeProjectRepo, string, map[string]lifecycleUser) {
	t.Helper()
	router, _, projectRepo := newTestRouterWithHandlers()
	users := map[string]lifecycleUser{}
	for _, name := range []string{"owner", "admin", "member", "outsider"} {
		rec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register", map[string]string{
			"username": name, "email": name + "@example.com", "password": "hunter22222",
		}, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("register %s: %d %s", name, rec.Code, rec.Body.String())
		}
		var body struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		}
		decodeJSON(t, rec, &body)
		users[name] = lifecycleUser{id: body.User.ID, token: extractToken(t, rec)}
	}
	createRec := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Team Project"}, users["owner"].token)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create project: %d %s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	decodeJSON(t, createRec, &created)
	projectRepo.addMember(created.ID, users["admin"].id, project.RoleAdmin)
	projectRepo.addMember(created.ID, users["member"].id, project.RoleMember)
	return router, projectRepo, created.ID, users
}

func memberRoles(t *testing.T, router http.Handler, projectID, token string) map[string]string {
	t.Helper()
	rec := doJSON(t, router, http.MethodGet, "/api/v1/projects/"+projectID, nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get project: %d %s", rec.Code, rec.Body.String())
	}
	var detail struct {
		Members []struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"members"`
	}
	decodeJSON(t, rec, &detail)
	roles := map[string]string{}
	for _, m := range detail.Members {
		roles[m.ID] = m.Role
	}
	return roles
}

func expectLifecycleStatus(t *testing.T, label string, got, want int, body string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: expected %d, got %d: %s", label, want, got, body)
	}
}

func TestTransferProjectOwnershipHTTP(t *testing.T) {
	router, _, id, u := setupLifecycleProject(t)
	path := "/api/v1/projects/" + id + "/transfer-ownership"

	rec := doJSON(t, router, http.MethodPost, path, map[string]string{"new_owner_id": u["member"].id, "previous_owner_role": "admin"}, u["admin"].token)
	expectLifecycleStatus(t, "admin transferring", rec.Code, http.StatusForbidden, rec.Body.String())
	rec = doJSON(t, router, http.MethodPost, path, map[string]string{"new_owner_id": u["member"].id, "previous_owner_role": "owner"}, u["owner"].token)
	expectLifecycleStatus(t, "invalid previous role", rec.Code, http.StatusBadRequest, rec.Body.String())
	rec = doJSON(t, router, http.MethodPost, path, map[string]string{"new_owner_id": u["owner"].id, "previous_owner_role": "admin"}, u["owner"].token)
	expectLifecycleStatus(t, "transfer to self", rec.Code, http.StatusBadRequest, rec.Body.String())
	rec = doJSON(t, router, http.MethodPost, path, map[string]string{"new_owner_id": u["outsider"].id, "previous_owner_role": "admin"}, u["owner"].token)
	expectLifecycleStatus(t, "transfer to non-member", rec.Code, http.StatusNotFound, rec.Body.String())

	rec = doJSON(t, router, http.MethodPost, path, map[string]string{"new_owner_id": u["member"].id, "previous_owner_role": "admin"}, u["owner"].token)
	expectLifecycleStatus(t, "transfer", rec.Code, http.StatusNoContent, rec.Body.String())

	roles := memberRoles(t, router, id, u["owner"].token)
	if roles[u["member"].id] != "owner" || roles[u["owner"].id] != "admin" {
		t.Fatalf("unexpected roles after transfer: %+v", roles)
	}

	// The previous owner may now leave.
	rec = doJSON(t, router, http.MethodPost, "/api/v1/projects/"+id+"/leave", nil, u["owner"].token)
	expectLifecycleStatus(t, "previous owner leaving", rec.Code, http.StatusNoContent, rec.Body.String())
}

func TestUpdateProjectMemberRoleHTTP(t *testing.T) {
	router, _, id, u := setupLifecycleProject(t)
	rolePath := func(userID string) string { return "/api/v1/projects/" + id + "/members/" + userID + "/role" }

	rec := doJSON(t, router, http.MethodPut, rolePath(u["member"].id), map[string]string{"role": "admin"}, u["admin"].token)
	expectLifecycleStatus(t, "admin promoting", rec.Code, http.StatusForbidden, rec.Body.String())
	rec = doJSON(t, router, http.MethodPut, rolePath(u["owner"].id), map[string]string{"role": "admin"}, u["owner"].token)
	expectLifecycleStatus(t, "owner demoting self", rec.Code, http.StatusConflict, rec.Body.String())
	rec = doJSON(t, router, http.MethodPut, rolePath(u["member"].id), map[string]string{"role": "owner"}, u["owner"].token)
	expectLifecycleStatus(t, "role owner", rec.Code, http.StatusBadRequest, rec.Body.String())
	rec = doJSON(t, router, http.MethodPut, rolePath(u["member"].id), map[string]string{"role": "admin"}, u["outsider"].token)
	expectLifecycleStatus(t, "outsider", rec.Code, http.StatusNotFound, rec.Body.String())

	rec = doJSON(t, router, http.MethodPut, rolePath(u["member"].id), map[string]string{"role": "admin"}, u["owner"].token)
	expectLifecycleStatus(t, "promote", rec.Code, http.StatusNoContent, rec.Body.String())
	rec = doJSON(t, router, http.MethodPut, rolePath(u["admin"].id), map[string]string{"role": "member"}, u["owner"].token)
	expectLifecycleStatus(t, "demote", rec.Code, http.StatusNoContent, rec.Body.String())

	roles := memberRoles(t, router, id, u["owner"].token)
	if roles[u["member"].id] != "admin" || roles[u["admin"].id] != "member" {
		t.Fatalf("unexpected roles: %+v", roles)
	}
}

func TestLeaveProjectHTTP(t *testing.T) {
	router, _, id, u := setupLifecycleProject(t)
	path := "/api/v1/projects/" + id + "/leave"

	rec := doJSON(t, router, http.MethodPost, path, nil, u["owner"].token)
	expectLifecycleStatus(t, "owner leaving", rec.Code, http.StatusConflict, rec.Body.String())
	rec = doJSON(t, router, http.MethodPost, path, nil, u["outsider"].token)
	expectLifecycleStatus(t, "outsider leaving", rec.Code, http.StatusNotFound, rec.Body.String())
	for _, name := range []string{"admin", "member"} {
		rec = doJSON(t, router, http.MethodPost, path, nil, u[name].token)
		expectLifecycleStatus(t, name+" leaving", rec.Code, http.StatusNoContent, rec.Body.String())
		rec = doJSON(t, router, http.MethodGet, "/api/v1/projects/"+id, nil, u[name].token)
		expectLifecycleStatus(t, name+" reading after leaving", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestDeleteAndRestoreProjectHTTP(t *testing.T) {
	router, repo, id, u := setupLifecycleProject(t)

	rec := doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+id, nil, u["admin"].token)
	expectLifecycleStatus(t, "admin deleting", rec.Code, http.StatusForbidden, rec.Body.String())
	rec = doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+id, nil, u["outsider"].token)
	expectLifecycleStatus(t, "outsider deleting", rec.Code, http.StatusNotFound, rec.Body.String())
	rec = doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+id, nil, u["owner"].token)
	expectLifecycleStatus(t, "owner deleting", rec.Code, http.StatusNoContent, rec.Body.String())

	rec = doJSON(t, router, http.MethodGet, "/api/v1/projects/"+id, nil, u["member"].token)
	expectLifecycleStatus(t, "reading deleted project", rec.Code, http.StatusNotFound, rec.Body.String())
	rec = doJSON(t, router, http.MethodGet, "/api/v1/projects", nil, u["owner"].token)
	var list struct {
		Projects []struct{ ID string } `json:"projects"`
	}
	decodeJSON(t, rec, &list)
	if len(list.Projects) != 0 {
		t.Fatalf("deleted project still listed: %+v", list.Projects)
	}

	rec = doJSON(t, router, http.MethodGet, "/api/v1/projects/deleted", nil, u["owner"].token)
	expectLifecycleStatus(t, "list deleted", rec.Code, http.StatusOK, rec.Body.String())
	var deleted struct {
		Projects []struct {
			ID              string    `json:"id"`
			Name            string    `json:"name"`
			DeletedAt       time.Time `json:"deleted_at"`
			RestoreDeadline time.Time `json:"restore_deadline"`
		} `json:"projects"`
	}
	decodeJSON(t, rec, &deleted)
	if len(deleted.Projects) != 1 || deleted.Projects[0].ID != id || deleted.Projects[0].RestoreDeadline.Sub(deleted.Projects[0].DeletedAt) != project.RestoreWindow {
		t.Fatalf("unexpected deleted list: %+v", deleted.Projects)
	}
	rec = doJSON(t, router, http.MethodGet, "/api/v1/projects/deleted", nil, u["admin"].token)
	decodeJSON(t, rec, &deleted)
	if len(deleted.Projects) != 0 {
		t.Fatalf("admin should not see the owner's deleted projects: %+v", deleted.Projects)
	}

	rec = doJSON(t, router, http.MethodPost, "/api/v1/projects/"+id+"/restore", nil, u["admin"].token)
	expectLifecycleStatus(t, "admin restoring", rec.Code, http.StatusNotFound, rec.Body.String())
	rec = doJSON(t, router, http.MethodPost, "/api/v1/projects/"+id+"/restore", nil, u["owner"].token)
	expectLifecycleStatus(t, "owner restoring", rec.Code, http.StatusNoContent, rec.Body.String())
	if roles := memberRoles(t, router, id, u["member"].token); roles[u["owner"].id] != "owner" {
		t.Fatalf("restorer should be owner again: %+v", roles)
	}

	rec = doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+id, nil, u["owner"].token)
	expectLifecycleStatus(t, "delete again", rec.Code, http.StatusNoContent, rec.Body.String())
	repo.expireDeletion(id)
	rec = doJSON(t, router, http.MethodPost, "/api/v1/projects/"+id+"/restore", nil, u["owner"].token)
	expectLifecycleStatus(t, "restore after window", rec.Code, http.StatusGone, rec.Body.String())
}
