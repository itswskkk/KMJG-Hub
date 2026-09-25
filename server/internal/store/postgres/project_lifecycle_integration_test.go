package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/project"
)

// TestProjectLifecycleRepositoryPostgres exercises the SQL-enforced Project
// lifecycle rules (ownership transfer, role changes, leave, soft delete and
// restore) against a real database. Opt-in like the other integration
// tests: set KMJG_TEST_DATABASE_URL to a *test* database.
func TestProjectLifecycleRepositoryPostgres(t *testing.T) {
	databaseURL := os.Getenv("KMJG_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("KMJG_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var databaseName string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(databaseName), "test") {
		t.Fatalf("refusing destructive integration setup on non-test database %q", databaseName)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE users CASCADE`); err != nil {
		t.Fatal(err)
	}
	insertUser := func(name string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO users(username,email,password_hash) VALUES($1,$2,'hash') RETURNING id`, name, name+"@example.test").Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	owner, admin, member, outsider := insertUser("lc-owner"), insertUser("lc-admin"), insertUser("lc-member"), insertUser("lc-outsider")

	repo := NewProjectRepository(pool)
	newProject := func(name string) string {
		p := &project.Project{Name: name}
		if err := repo.CreateWithOwner(ctx, p, owner); err != nil {
			t.Fatal(err)
		}
		for userID, role := range map[string]string{admin: "admin", member: "member"} {
			if _, err := pool.Exec(ctx, `INSERT INTO project_members(project_id,user_id,role) VALUES($1,$2,$3)`, p.ID, userID, role); err != nil {
				t.Fatal(err)
			}
		}
		return p.ID
	}
	roleOf := func(projectID, userID string) string {
		var role string
		if err := pool.QueryRow(ctx, `SELECT role FROM project_members WHERE project_id=$1 AND user_id=$2`, projectID, userID).Scan(&role); err != nil {
			return ""
		}
		return role
	}
	ownerCount := func(projectID string) int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM project_members WHERE project_id=$1 AND role='owner'`, projectID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("transfer ownership", func(t *testing.T) {
		id := newProject("transfer")
		if err := repo.TransferOwnership(ctx, id, admin, member, project.RoleAdmin); !errors.Is(err, project.ErrForbidden) {
			t.Fatalf("admin transferring: got %v", err)
		}
		if err := repo.TransferOwnership(ctx, id, owner, outsider, project.RoleAdmin); !errors.Is(err, project.ErrNotFound) {
			t.Fatalf("transfer to non-member: got %v", err)
		}
		if err := repo.TransferOwnership(ctx, id, owner, member, project.RoleMember); err != nil {
			t.Fatal(err)
		}
		if roleOf(id, member) != "owner" || roleOf(id, owner) != "member" || ownerCount(id) != 1 {
			t.Fatalf("after transfer: member=%s owner=%s owners=%d", roleOf(id, member), roleOf(id, owner), ownerCount(id))
		}
		// The previous owner no longer has owner powers.
		if err := repo.TransferOwnership(ctx, id, owner, admin, project.RoleMember); !errors.Is(err, project.ErrForbidden) {
			t.Fatalf("previous owner transferring again: got %v", err)
		}
	})

	t.Run("concurrent transfers keep exactly one owner", func(t *testing.T) {
		id := newProject("concurrent")
		var wg sync.WaitGroup
		for _, target := range []string{admin, member} {
			wg.Add(1)
			go func(target string) {
				defer wg.Done()
				_ = repo.TransferOwnership(ctx, id, owner, target, project.RoleAdmin)
			}(target)
		}
		wg.Wait()
		if n := ownerCount(id); n != 1 {
			t.Fatalf("expected exactly one owner, got %d", n)
		}
	})

	t.Run("single owner index", func(t *testing.T) {
		id := newProject("index")
		if _, err := pool.Exec(ctx, `UPDATE project_members SET role='owner' WHERE project_id=$1 AND user_id=$2`, id, admin); err == nil {
			t.Fatal("expected the single-owner index to reject a second owner")
		}
	})

	t.Run("only owner changes roles", func(t *testing.T) {
		id := newProject("roles")
		if err := repo.UpdateMemberRole(ctx, id, admin, member, project.RoleAdmin); !errors.Is(err, project.ErrForbidden) {
			t.Fatalf("admin promoting: got %v", err)
		}
		if err := repo.UpdateMemberRole(ctx, id, owner, owner, project.RoleAdmin); !errors.Is(err, project.ErrForbidden) {
			t.Fatalf("owner demoting self: got %v", err)
		}
		if err := repo.UpdateMemberRole(ctx, id, owner, member, project.RoleAdmin); err != nil {
			t.Fatal(err)
		}
		if err := repo.UpdateMemberRole(ctx, id, owner, admin, project.RoleMember); err != nil {
			t.Fatal(err)
		}
		if roleOf(id, member) != "admin" || roleOf(id, admin) != "member" {
			t.Fatalf("roles: member=%s admin=%s", roleOf(id, member), roleOf(id, admin))
		}
	})

	t.Run("leave", func(t *testing.T) {
		id := newProject("leave")
		if err := repo.Leave(ctx, id, owner); !errors.Is(err, project.ErrOwnerCannotLeave) {
			t.Fatalf("owner leaving: got %v", err)
		}
		if err := repo.Leave(ctx, id, outsider); !errors.Is(err, project.ErrNotFound) {
			t.Fatalf("outsider leaving: got %v", err)
		}
		if err := repo.Leave(ctx, id, member); err != nil {
			t.Fatal(err)
		}
		if roleOf(id, member) != "" {
			t.Fatal("member should be gone")
		}
	})

	t.Run("delete, hide and restore", func(t *testing.T) {
		id := newProject("delete-restore")
		if err := repo.Delete(ctx, id, admin); !errors.Is(err, project.ErrForbidden) {
			t.Fatalf("admin deleting: got %v", err)
		}
		if err := repo.Delete(ctx, id, owner); err != nil {
			t.Fatal(err)
		}
		if err := repo.Delete(ctx, id, owner); !errors.Is(err, project.ErrForbidden) {
			t.Fatalf("deleting twice: got %v", err)
		}
		if _, err := repo.GetDetailForUser(ctx, id, member); !errors.Is(err, project.ErrNotFound) {
			t.Fatalf("deleted project detail: got %v", err)
		}
		list, err := repo.ListForUser(ctx, owner)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range list {
			if s.ID == id {
				t.Fatal("deleted project still listed")
			}
		}
		if ids, _ := repo.ListMemberUserIDs(ctx, id); len(ids) != 0 {
			t.Fatalf("deleted project should have no fan-out members, got %v", ids)
		}
		// Lifecycle writes are blocked on a deleted project.
		if err := repo.Leave(ctx, id, member); !errors.Is(err, project.ErrNotFound) {
			t.Fatalf("leaving deleted project: got %v", err)
		}
		if err := repo.RemoveMember(ctx, id, owner, member); !errors.Is(err, project.ErrForbidden) {
			t.Fatalf("removing member of deleted project: got %v", err)
		}

		deleted, err := repo.ListDeleted(ctx, owner)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, d := range deleted {
			if d.ID == id {
				found = true
				if d.RestoreDeadline.Sub(d.DeletedAt) != project.RestoreWindow || d.MemberCount != 3 {
					t.Fatalf("unexpected deleted summary %+v", d)
				}
			}
		}
		if !found {
			t.Fatal("deleted project missing from ListDeleted")
		}
		if other, _ := repo.ListDeleted(ctx, admin); len(other) != 0 {
			t.Fatalf("admin should not see deleted projects: %+v", other)
		}

		if err := repo.Restore(ctx, id, admin); !errors.Is(err, project.ErrNotFound) {
			t.Fatalf("admin restoring: got %v", err)
		}
		if err := repo.Restore(ctx, id, owner); err != nil {
			t.Fatal(err)
		}
		if err := repo.Restore(ctx, id, owner); !errors.Is(err, project.ErrNotFound) {
			t.Fatalf("restoring an active project: got %v", err)
		}
		if _, err := repo.GetDetailForUser(ctx, id, member); err != nil {
			t.Fatalf("restored project detail: %v", err)
		}
		if roleOf(id, owner) != "owner" || ownerCount(id) != 1 {
			t.Fatal("restorer should be the single owner")
		}
	})

	t.Run("restore after window fails and purge removes the project", func(t *testing.T) {
		id := newProject("expired")
		if err := repo.Delete(ctx, id, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE projects SET deleted_at = now() - interval '31 days' WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
		if err := repo.Restore(ctx, id, owner); !errors.Is(err, project.ErrRestoreWindowExpired) {
			t.Fatalf("late restore: got %v", err)
		}
		if deleted, _ := repo.ListDeleted(ctx, owner); len(deleted) != 0 {
			for _, d := range deleted {
				if d.ID == id {
					t.Fatal("expired project should not be listed as restorable")
				}
			}
		}

		var messageID string
		if err := pool.QueryRow(ctx, `INSERT INTO project_chat_messages(project_id,author_user_id,body) VALUES($1,$2,'hi') RETURNING id`, id, owner).Scan(&messageID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO project_chat_attachments(message_id,project_id,storage_id,filename,content_type,size_bytes) VALUES($1,$2,'lc-storage-1','a.txt','text/plain',1)`, messageID, id); err != nil {
			t.Fatal(err)
		}

		storageIDs, err := repo.PurgeDeletedBefore(ctx, time.Now().Add(-project.RestoreWindow))
		if err != nil {
			t.Fatal(err)
		}
		if len(storageIDs) != 1 || storageIDs[0] != "lc-storage-1" {
			t.Fatalf("unexpected purged storage IDs %v", storageIDs)
		}
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE id=$1)`, id).Scan(&exists); err != nil || exists {
			t.Fatalf("project should be purged (exists=%v err=%v)", exists, err)
		}
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1)`, id).Scan(&exists); err != nil || exists {
			t.Fatalf("memberships should cascade (exists=%v err=%v)", exists, err)
		}
	})

	t.Run("malformed ids are not found", func(t *testing.T) {
		if err := repo.Delete(ctx, "not-a-uuid", owner); !errors.Is(err, project.ErrNotFound) {
			t.Fatalf("Delete: %v", err)
		}
		if err := repo.Restore(ctx, "not-a-uuid", owner); !errors.Is(err, project.ErrNotFound) {
			t.Fatalf("Restore: %v", err)
		}
		if err := repo.Leave(ctx, "not-a-uuid", owner); !errors.Is(err, project.ErrNotFound) {
			t.Fatalf("Leave: %v", err)
		}
		if err := repo.TransferOwnership(ctx, "not-a-uuid", owner, member, project.RoleAdmin); !errors.Is(err, project.ErrNotFound) {
			t.Fatalf("TransferOwnership: %v", err)
		}
		if err := repo.UpdateMemberRole(ctx, "not-a-uuid", owner, member, project.RoleAdmin); !errors.Is(err, project.ErrNotFound) {
			t.Fatalf("UpdateMemberRole: %v", err)
		}
	})
}
