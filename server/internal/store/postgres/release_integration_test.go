package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/itswskkk/KMJG-Hub/server/internal/chat"
	"github.com/itswskkk/KMJG-Hub/server/internal/workcontext"
)

func TestReleaseRepositoriesPostgres(t *testing.T) {
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
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO users(username,email,password_hash) VALUES($1,$2,'hash') RETURNING id`, name, name+"@example.test").Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	owner, outsider := insertUser("release-owner"), insertUser("release-outsider")
	var projectID string
	if err := pool.QueryRow(ctx, `INSERT INTO projects(name,created_by) VALUES('release',$1) RETURNING id`, owner).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO project_members(project_id,user_id,role) VALUES($1,$2,'owner')`, projectID, owner); err != nil {
		t.Fatal(err)
	}

	chatRepo := NewChatRepository(pool)
	message, err := chatRepo.CreateWithAttachment(ctx, projectID, owner, "attached", chat.Attachment{StorageID: "opaque-one", Filename: "note.txt", ContentType: "text/plain", SizeBytes: 5}, 5)
	if err != nil {
		t.Fatal(err)
	}
	page, err := chatRepo.ListPage(ctx, projectID, owner, nil, 51)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || len(page[0].Attachments) != 1 || page[0].Attachments[0].ID != message.Attachments[0].ID {
		t.Fatalf("page=%+v", page)
	}
	if _, err := chatRepo.GetAttachment(ctx, projectID, message.Attachments[0].ID, outsider); !errors.Is(err, chat.ErrNotFound) {
		t.Fatalf("outsider attachment error=%v", err)
	}
	if _, err := chatRepo.CreateWithAttachment(ctx, projectID, owner, "over quota", chat.Attachment{StorageID: "opaque-two", Filename: "two.txt", ContentType: "text/plain", SizeBytes: 1}, 5); !errors.Is(err, chat.ErrStorageLimit) {
		t.Fatalf("quota error=%v", err)
	}

	workRepo := NewWorkContextRepository(pool)
	saved, err := workRepo.Upsert(ctx, projectID, owner, false, "manual", "feature/release")
	if err != nil {
		t.Fatal(err)
	}
	if saved.StatusMode != "manual" || saved.CurrentBranch != "feature/release" {
		t.Fatalf("saved=%+v", saved)
	}
	if _, err := workRepo.Upsert(ctx, projectID, outsider, true, "automatic", "main"); !errors.Is(err, workcontext.ErrNotFound) {
		t.Fatalf("outsider work context error=%v", err)
	}
}
