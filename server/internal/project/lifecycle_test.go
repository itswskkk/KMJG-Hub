package project_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/project"
)

// deletedProject is a soft-deleted project held by the fake lifecycle
// implementation. Deleting moves a project out of fakeRepo.projects, so the
// existing Repository fake methods exclude it without any changes.
type deletedProject struct {
	project   project.Project
	members   []project.Member
	deletedBy string
	deletedAt time.Time
}

// The methods below make fakeRepo also satisfy project.LifecycleRepository.
var _ project.LifecycleRepository = (*fakeRepo)(nil)

func (f *fakeRepo) roleOf(projectID, userID string) (project.Role, int, bool) {
	for i, m := range f.members[projectID] {
		if m.UserID == userID {
			return m.Role, i, true
		}
	}
	return "", -1, false
}

func (f *fakeRepo) TransferOwnership(_ context.Context, projectID, actorUserID, newOwnerID string, previousOwnerRole project.Role) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.projects[projectID]; !ok {
		return project.ErrNotFound
	}
	actorRole, ai, ok := f.roleOf(projectID, actorUserID)
	if !ok || actorRole != project.RoleOwner {
		return project.ErrForbidden
	}
	_, ti, ok := f.roleOf(projectID, newOwnerID)
	if !ok {
		return project.ErrNotFound
	}
	f.members[projectID][ai].Role = previousOwnerRole
	f.members[projectID][ti].Role = project.RoleOwner
	return nil
}

func (f *fakeRepo) UpdateMemberRole(_ context.Context, projectID, actorUserID, targetUserID string, newRole project.Role) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	actorRole, _, ok := f.roleOf(projectID, actorUserID)
	targetRole, ti, tok := f.roleOf(projectID, targetUserID)
	if !ok || !tok || actorRole != project.RoleOwner || targetRole == project.RoleOwner {
		return project.ErrForbidden
	}
	f.members[projectID][ti].Role = newRole
	return nil
}

func (f *fakeRepo) Leave(_ context.Context, projectID, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.projects[projectID]; !ok {
		return project.ErrNotFound
	}
	role, i, ok := f.roleOf(projectID, userID)
	if !ok {
		return project.ErrNotFound
	}
	if role == project.RoleOwner {
		return project.ErrOwnerCannotLeave
	}
	f.members[projectID] = append(f.members[projectID][:i], f.members[projectID][i+1:]...)
	return nil
}

func (f *fakeRepo) Delete(_ context.Context, projectID, actorUserID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.projects[projectID]
	if !ok {
		return project.ErrNotFound
	}
	if role, _, ok := f.roleOf(projectID, actorUserID); !ok || role != project.RoleOwner {
		return project.ErrForbidden
	}
	if f.deleted == nil {
		f.deleted = make(map[string]*deletedProject)
	}
	f.deleted[projectID] = &deletedProject{project: *p, members: f.members[projectID], deletedBy: actorUserID, deletedAt: f.now()}
	delete(f.projects, projectID)
	delete(f.members, projectID)
	return nil
}

func (f *fakeRepo) Restore(_ context.Context, projectID, actorUserID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.deleted[projectID]
	if !ok || d.deletedBy != actorUserID {
		return project.ErrNotFound
	}
	if !f.now().Before(d.deletedAt.Add(project.RestoreWindow)) {
		return project.ErrRestoreWindowExpired
	}
	p := d.project
	f.projects[projectID] = &p
	f.members[projectID] = d.members
	delete(f.deleted, projectID)
	return nil
}

func (f *fakeRepo) ListDeleted(_ context.Context, userID string) ([]project.DeletedSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []project.DeletedSummary
	for _, d := range f.deleted {
		deadline := d.deletedAt.Add(project.RestoreWindow)
		if d.deletedBy == userID && f.now().Before(deadline) {
			out = append(out, project.DeletedSummary{Project: d.project, MemberCount: len(d.members), DeletedAt: d.deletedAt, RestoreDeadline: deadline})
		}
	}
	return out, nil
}

func (f *fakeRepo) PurgeDeletedBefore(_ context.Context, cutoff time.Time) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var storageIDs []string
	for id, d := range f.deleted {
		if d.deletedAt.Before(cutoff) {
			storageIDs = append(storageIDs, "attachment-of-"+id)
			delete(f.deleted, id)
		}
	}
	return storageIDs, nil
}

func (f *fakeRepo) now() time.Time {
	if f.clock != nil {
		return f.clock()
	}
	return time.Now()
}

// lifecycleFixture creates a project owned by "owner" with an admin and a
// member.
func lifecycleFixture(t *testing.T) (*project.Service, *fakeRepo, string) {
	t.Helper()
	repo := newFakeRepo()
	svc := &project.Service{Repo: repo, Lifecycle: repo}
	p, err := svc.Create(context.Background(), "owner", project.CreateInput{Name: "Lifecycle"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	repo.members[p.ID] = append(repo.members[p.ID],
		project.Member{UserID: "admin", Role: project.RoleAdmin},
		project.Member{UserID: "member", Role: project.RoleMember},
	)
	return svc, repo, p.ID
}

func roleIn(t *testing.T, svc *project.Service, projectID, viewer, userID string) project.Role {
	t.Helper()
	d, err := svc.GetDetail(context.Background(), viewer, projectID)
	if err != nil {
		t.Fatalf("GetDetail: %v", err)
	}
	for _, m := range d.Members {
		if m.UserID == userID {
			return m.Role
		}
	}
	return ""
}

func TestTransferOwnership(t *testing.T) {
	ctx := context.Background()
	svc, _, id := lifecycleFixture(t)

	cases := []struct {
		name   string
		actor  string
		target string
		role   project.Role
		want   error
	}{
		{"admin cannot transfer", "admin", "member", project.RoleAdmin, project.ErrNotOwner},
		{"cannot transfer to self", "owner", "owner", project.RoleAdmin, project.ErrCannotTransferToSelf},
		{"target must be a member", "owner", "stranger", project.RoleAdmin, project.ErrNotFound},
		{"non-member actor", "stranger", "member", project.RoleAdmin, project.ErrNotFound},
	}
	for _, tc := range cases {
		if err := svc.TransferOwnership(ctx, tc.actor, id, tc.target, tc.role); !errors.Is(err, tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
	var validationErr *project.ValidationError
	if err := svc.TransferOwnership(ctx, "owner", id, "member", project.RoleOwner); !errors.As(err, &validationErr) {
		t.Fatalf("previous owner role owner: expected ValidationError, got %v", err)
	}

	if err := svc.TransferOwnership(ctx, "owner", id, "member", project.RoleMember); err != nil {
		t.Fatalf("TransferOwnership: %v", err)
	}
	if got := roleIn(t, svc, id, "owner", "member"); got != project.RoleOwner {
		t.Fatalf("new owner role = %q", got)
	}
	if got := roleIn(t, svc, id, "owner", "owner"); got != project.RoleMember {
		t.Fatalf("previous owner should stay as member, got %q", got)
	}
	owners := 0
	d, _ := svc.GetDetail(ctx, "owner", id)
	for _, m := range d.Members {
		if m.Role == project.RoleOwner {
			owners++
		}
	}
	if owners != 1 {
		t.Fatalf("expected exactly one owner, got %d", owners)
	}
}

func TestUpdateMemberRoleIsOwnerOnly(t *testing.T) {
	ctx := context.Background()
	svc, _, id := lifecycleFixture(t)

	if err := svc.UpdateMemberRole(ctx, "admin", id, "member", project.RoleAdmin); !errors.Is(err, project.ErrNotOwner) {
		t.Fatalf("admin promoting: got %v, want ErrNotOwner", err)
	}
	if err := svc.UpdateMemberRole(ctx, "owner", id, "owner", project.RoleAdmin); !errors.Is(err, project.ErrCannotDemoteOwner) {
		t.Fatalf("owner demoting self: got %v, want ErrCannotDemoteOwner", err)
	}
	if err := svc.UpdateMemberRole(ctx, "owner", id, "stranger", project.RoleAdmin); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("unknown target: got %v, want ErrNotFound", err)
	}
	var validationErr *project.ValidationError
	if err := svc.UpdateMemberRole(ctx, "owner", id, "member", project.RoleOwner); !errors.As(err, &validationErr) {
		t.Fatalf("role owner: expected ValidationError, got %v", err)
	}

	if err := svc.UpdateMemberRole(ctx, "owner", id, "member", project.RoleAdmin); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if got := roleIn(t, svc, id, "owner", "member"); got != project.RoleAdmin {
		t.Fatalf("promoted role = %q", got)
	}
	if err := svc.UpdateMemberRole(ctx, "owner", id, "admin", project.RoleMember); err != nil {
		t.Fatalf("demote: %v", err)
	}
	if got := roleIn(t, svc, id, "owner", "admin"); got != project.RoleMember {
		t.Fatalf("demoted role = %q", got)
	}
}

func TestLeaveProject(t *testing.T) {
	ctx := context.Background()
	svc, _, id := lifecycleFixture(t)

	if err := svc.Leave(ctx, "owner", id); !errors.Is(err, project.ErrOwnerCannotLeave) {
		t.Fatalf("owner leaving: got %v, want ErrOwnerCannotLeave", err)
	}
	if !errors.Is(project.ErrOwnerCannotLeave, project.ErrForbidden) {
		t.Fatal("ErrOwnerCannotLeave should wrap ErrForbidden")
	}
	for _, u := range []string{"admin", "member"} {
		if err := svc.Leave(ctx, u, id); err != nil {
			t.Fatalf("%s leaving: %v", u, err)
		}
		if _, err := svc.GetDetail(ctx, u, id); !errors.Is(err, project.ErrNotFound) {
			t.Fatalf("%s should no longer see the project, got %v", u, err)
		}
	}
	if err := svc.Leave(ctx, "stranger", id); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("non-member leaving: got %v", err)
	}

	// After transferring ownership the previous owner may leave.
	svc2, _, id2 := lifecycleFixture(t)
	if err := svc2.TransferOwnership(ctx, "owner", id2, "admin", project.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := svc2.Leave(ctx, "owner", id2); err != nil {
		t.Fatalf("previous owner leaving after transfer: %v", err)
	}
}

func TestDeleteAndRestoreProject(t *testing.T) {
	ctx := context.Background()
	svc, repo, id := lifecycleFixture(t)
	now := time.Now()
	repo.clock = func() time.Time { return now }

	if err := svc.Delete(ctx, "admin", id); !errors.Is(err, project.ErrNotOwner) {
		t.Fatalf("admin deleting: got %v, want ErrNotOwner", err)
	}
	if err := svc.Delete(ctx, "owner", id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.GetDetail(ctx, "member", id); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("deleted project should be hidden, got %v", err)
	}
	if list, _ := svc.List(ctx, "owner"); len(list) != 0 {
		t.Fatalf("deleted project should not be listed, got %+v", list)
	}

	deleted, err := svc.ListDeleted(ctx, "owner")
	if err != nil || len(deleted) != 1 || deleted[0].ID != id {
		t.Fatalf("ListDeleted: %+v, %v", deleted, err)
	}
	if !deleted[0].RestoreDeadline.Equal(deleted[0].DeletedAt.Add(project.RestoreWindow)) {
		t.Fatalf("unexpected restore deadline %v", deleted[0].RestoreDeadline)
	}
	if other, _ := svc.ListDeleted(ctx, "admin"); len(other) != 0 {
		t.Fatalf("only the deleting owner sees the project, got %+v", other)
	}

	if err := svc.Restore(ctx, "admin", id); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("admin restoring: got %v, want ErrNotFound", err)
	}
	if err := svc.Restore(ctx, "owner", id); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := roleIn(t, svc, id, "member", "owner"); got != project.RoleOwner {
		t.Fatalf("restorer should be owner, got %q", got)
	}

	// Restore after the window fails.
	if err := svc.Delete(ctx, "owner", id); err != nil {
		t.Fatal(err)
	}
	repo.clock = func() time.Time { return now.Add(project.RestoreWindow + time.Minute) }
	if err := svc.Restore(ctx, "owner", id); !errors.Is(err, project.ErrRestoreWindowExpired) {
		t.Fatalf("late restore: got %v, want ErrRestoreWindowExpired", err)
	}
}

type recordingDeleter struct{ ids []string }

func (r *recordingDeleter) Delete(_ context.Context, id string) error {
	r.ids = append(r.ids, id)
	return nil
}

func TestPurgeExpiredDeletedRemovesStoredFiles(t *testing.T) {
	ctx := context.Background()
	svc, repo, id := lifecycleFixture(t)
	store := &recordingDeleter{}
	svc.Storage = store
	now := time.Now()
	repo.clock = func() time.Time { return now }
	if err := svc.Delete(ctx, "owner", id); err != nil {
		t.Fatal(err)
	}

	if err := svc.PurgeExpiredDeleted(ctx, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(store.ids) != 0 {
		t.Fatalf("nothing should be purged within the window, got %v", store.ids)
	}
	if err := svc.PurgeExpiredDeleted(ctx, now.Add(project.RestoreWindow+time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(store.ids) != 1 {
		t.Fatalf("expected the purged project's attachment to be deleted, got %v", store.ids)
	}
}
