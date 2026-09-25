package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/itswskkk/KMJG-Hub/server/internal/task"
)

// TestTaskRepositoryPostgres exercises the authorization and transactional
// rules that repository fakes cannot prove. It is opt-in so ordinary unit
// tests remain hermetic; CI/local integration runs set KMJG_TEST_DATABASE_URL.
func TestTaskRepositoryPostgres(t *testing.T) {
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

	userID := func(username string) string {
		t.Helper()
		var id string
		err := pool.QueryRow(ctx, `INSERT INTO users(username,email,password_hash) VALUES($1,$2,'hash') RETURNING id`, username, username+"@example.test").Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	owner, member, outsider := userID("owner"), userID("member"), userID("outsider")
	projectID := func(name string) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO projects(name,created_by) VALUES($1,$2) RETURNING id`, name, owner).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO project_members(project_id,user_id,role) VALUES($1,$2,'owner'),($1,$3,'member')`, id, owner, member); err != nil {
			t.Fatal(err)
		}
		return id
	}
	projectOne, projectTwo := projectID("one"), projectID("two")
	repo := NewTaskRepository(pool)

	created, err := repo.Create(ctx, projectOne, owner, "Assigned task", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.List(ctx, projectOne, outsider); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("outsider List error = %v, want ErrNotFound", err)
	}
	request, err := repo.RequestAssignment(ctx, projectOne, created.ID, owner, member)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RespondAssignment(ctx, request.ID, outsider, true); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("outsider response error = %v, want ErrNotFound", err)
	}
	accepted, err := repo.RespondAssignment(ctx, request.ID, member, true)
	if err != nil || accepted.AssigneeID == nil || *accepted.AssigneeID != member {
		t.Fatalf("accepted task = %+v, err=%v", accepted, err)
	}
	current, err := repo.SetCurrent(ctx, projectOne, created.ID, member)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != task.StatusInProgress {
		t.Fatalf("current status = %q, want in_progress", current.Status)
	}

	other, err := repo.Create(ctx, projectTwo, member, "Other project task", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err = repo.AssignSelf(ctx, projectTwo, other.ID, member)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetCurrent(ctx, projectTwo, other.ID, member); err != nil {
		t.Fatal(err)
	}
	var currentTaskID string
	if err := pool.QueryRow(ctx, `SELECT task_id FROM user_current_tasks WHERE user_id=$1`, member).Scan(&currentTaskID); err != nil {
		t.Fatal(err)
	}
	if currentTaskID != other.ID {
		t.Fatalf("current task = %s, want %s", currentTaskID, other.ID)
	}
	if _, err := repo.SetCurrent(ctx, projectOne, created.ID, outsider); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("unassigned outsider SetCurrent error = %v, want ErrNotFound", err)
	}
}
