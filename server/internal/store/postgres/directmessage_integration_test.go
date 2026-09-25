package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/directmessage"
)

// TestDirectMessageRepositoryPostgres exercises the SQL-enforced DM access
// rules against a real database. Opt-in like the other integration tests:
// set KMJG_TEST_DATABASE_URL to a *test* database.
func TestDirectMessageRepositoryPostgres(t *testing.T) {
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
	alice, bob, carol, dave, erin := insertUser("dm-alice"), insertUser("dm-bob"), insertUser("dm-carol"), insertUser("dm-dave"), insertUser("dm-erin")

	friends := NewFriendRepository(pool)
	repo := NewDirectMessageRepository(pool)

	// alice & bob: friends. carol & dave: share a Project. erin: unrelated.
	req, err := friends.SendRequest(ctx, alice, bob)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := friends.AcceptRequest(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	var projectID string
	if err := pool.QueryRow(ctx, `INSERT INTO projects(name,created_by) VALUES('DM Project',$1) RETURNING id`, carol).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO project_members(project_id,user_id,role) VALUES($1,$2,'owner'),($1,$3,'member')`, projectID, carol, dave); err != nil {
		t.Fatal(err)
	}

	m1, err := repo.Create(ctx, alice, bob, "hi bob")
	if err != nil {
		t.Fatalf("friends send: %v", err)
	}
	if m1.SenderUsername != "dm-alice" || m1.RecipientUsername != "dm-bob" || m1.Body != "hi bob" {
		t.Fatalf("m1=%+v", m1)
	}
	if _, err := repo.Create(ctx, dave, carol, "hi carol"); err != nil {
		t.Fatalf("project members send: %v", err)
	}
	if _, err := repo.Create(ctx, erin, alice, "hello?"); !errors.Is(err, directmessage.ErrForbidden) {
		t.Fatalf("unrelated send err=%v", err)
	}
	if _, err := repo.Create(ctx, alice, "not-a-uuid", "x"); !errors.Is(err, directmessage.ErrNotFound) {
		t.Fatalf("invalid recipient err=%v", err)
	}
	if _, err := repo.Create(ctx, alice, "00000000-0000-0000-0000-000000000000", "x"); !errors.Is(err, directmessage.ErrNotFound) {
		t.Fatalf("missing recipient err=%v", err)
	}

	// Leaving the only shared Project revokes access (not friends).
	if _, err := pool.Exec(ctx, `DELETE FROM project_members WHERE project_id=$1 AND user_id=$2`, projectID, dave); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, dave, carol, "still here?"); !errors.Is(err, directmessage.ErrForbidden) {
		t.Fatalf("after leaving project err=%v", err)
	}

	// History, pagination, conversations.
	m2, err := repo.Create(ctx, bob, alice, "hi alice")
	if err != nil {
		t.Fatal(err)
	}
	page, err := repo.ListPage(ctx, alice, bob, nil, 10)
	if err != nil || len(page) != 2 || page[0].ID != m1.ID || page[1].ID != m2.ID {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	older, err := repo.ListPage(ctx, bob, alice, &directmessage.Cursor{CreatedAt: m2.CreatedAt, ID: m2.ID}, 10)
	if err != nil || len(older) != 1 || older[0].ID != m1.ID {
		t.Fatalf("older=%+v err=%v", older, err)
	}
	if other, err := repo.ListPage(ctx, erin, bob, nil, 10); err != nil || len(other) != 0 {
		t.Fatalf("outsider page=%+v err=%v", other, err)
	}
	if _, err := repo.ListPage(ctx, alice, "not-a-uuid", nil, 10); !errors.Is(err, directmessage.ErrNotFound) {
		t.Fatalf("invalid other err=%v", err)
	}
	convs, err := repo.ListConversations(ctx, alice)
	if err != nil || len(convs) != 1 || convs[0].OtherUserID != bob || convs[0].LastMessageBody != "hi alice" || convs[0].LastMessageFromMe {
		t.Fatalf("convs=%+v err=%v", convs, err)
	}

	// Deletion: sender only; outsiders don't learn the message exists.
	if _, err := repo.SoftDelete(ctx, m2.ID, alice); !errors.Is(err, directmessage.ErrForbidden) {
		t.Fatalf("recipient delete err=%v", err)
	}
	if _, err := repo.SoftDelete(ctx, m2.ID, erin); !errors.Is(err, directmessage.ErrNotFound) {
		t.Fatalf("outsider delete err=%v", err)
	}
	deleted, err := repo.SoftDelete(ctx, m2.ID, bob)
	if err != nil || deleted.ID != m2.ID || deleted.RecipientID != alice {
		t.Fatalf("sender delete=%+v err=%v", deleted, err)
	}
	if _, err := repo.SoftDelete(ctx, m2.ID, bob); !errors.Is(err, directmessage.ErrNotFound) {
		t.Fatalf("repeat delete err=%v", err)
	}
	if page, _ := repo.ListPage(ctx, alice, bob, nil, 10); len(page) != 1 {
		t.Fatalf("deleted message still listed: %+v", page)
	}

	// Retention purge removes only expired soft-deleted rows.
	if err := repo.PurgeDeletedBefore(ctx, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	var remaining int
	pool.QueryRow(ctx, `SELECT count(*) FROM direct_messages WHERE id=$1`, m2.ID).Scan(&remaining)
	if remaining != 1 {
		t.Fatal("purge removed a message still inside the retention window")
	}
	if err := repo.PurgeDeletedBefore(ctx, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	pool.QueryRow(ctx, `SELECT count(*) FROM direct_messages WHERE id=$1`, m2.ID).Scan(&remaining)
	if remaining != 0 {
		t.Fatal("purge kept an expired deleted message")
	}

	// Blocking (either direction) revokes access even between friends.
	if _, err := friends.Block(ctx, bob, alice); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, alice, bob, "blocked?"); !errors.Is(err, directmessage.ErrForbidden) {
		t.Fatalf("blocked send err=%v", err)
	}
	if _, err := repo.Create(ctx, bob, alice, "blocker send"); !errors.Is(err, directmessage.ErrForbidden) {
		t.Fatalf("blocker send err=%v", err)
	}
}
