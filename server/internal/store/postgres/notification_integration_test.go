package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/itswskkk/KMJG-Hub/server/internal/notification"
)

// TestNotificationRepositoryPostgres exercises owner-scoped notification
// persistence against a real database. Opt-in like the other integration
// tests: set KMJG_TEST_DATABASE_URL to a *test* database.
func TestNotificationRepositoryPostgres(t *testing.T) {
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
	alice, bob := insertUser("n-alice"), insertUser("n-bob")
	repo := NewNotificationRepository(pool)

	n1, err := repo.Create(ctx, alice, notification.EventFriendRequest, map[string]string{"sender_id": bob})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	if err := json.Unmarshal(n1.Payload, &payload); err != nil || payload["sender_id"] != bob {
		t.Fatalf("payload=%s err=%v", n1.Payload, err)
	}
	if n1.UserID != alice || n1.EventType != notification.EventFriendRequest || n1.ReadAt != nil {
		t.Fatalf("n1=%+v", n1)
	}
	if _, err := repo.Create(ctx, alice, notification.EventType("bogus"), nil); err == nil {
		t.Fatal("unknown event type should violate CHECK constraint")
	}
	if _, err := repo.Create(ctx, "not-a-uuid", notification.EventTaskAssigned, nil); !errors.Is(err, notification.ErrNotFound) {
		t.Fatalf("invalid user id err=%v", err)
	}
	for i := 0; i < 4; i++ {
		if _, err := repo.Create(ctx, alice, notification.EventTaskComment, map[string]int{"i": i}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.Create(ctx, bob, notification.EventDirectMessage, nil); err != nil {
		t.Fatal(err)
	}

	// Keyset pagination: newest first, no overlap, only alice's.
	page1, unread, err := repo.ListPage(ctx, alice, nil, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 3 || unread != 5 {
		t.Fatalf("page1 len=%d unread=%d", len(page1), unread)
	}
	last := page1[len(page1)-1]
	page2, _, err := repo.ListPage(ctx, alice, &notification.Cursor{CreatedAt: last.CreatedAt, ID: last.ID}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 2 {
		t.Fatalf("page2 len=%d", len(page2))
	}
	seen := map[string]bool{}
	for _, n := range append(page1, page2...) {
		if n.UserID != alice || seen[n.ID] {
			t.Fatalf("unexpected %+v", n)
		}
		seen[n.ID] = true
	}
	if page2[len(page2)-1].ID != n1.ID {
		t.Fatal("oldest notification should be last")
	}

	// MarkRead: owner-only, idempotent, invalid IDs are not found.
	if err := repo.MarkRead(ctx, n1.ID, bob); !errors.Is(err, notification.ErrNotFound) {
		t.Fatalf("non-owner mark read err=%v", err)
	}
	if err := repo.MarkRead(ctx, "not-a-uuid", alice); !errors.Is(err, notification.ErrNotFound) {
		t.Fatalf("invalid id mark read err=%v", err)
	}
	if err := repo.MarkRead(ctx, n1.ID, alice); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkRead(ctx, n1.ID, alice); err != nil {
		t.Fatalf("repeat mark read err=%v", err)
	}
	if _, unread, _ = repo.ListPage(ctx, alice, nil, 10); unread != 4 {
		t.Fatalf("unread after mark=%d", unread)
	}

	// Delete: owner-only.
	if err := repo.Delete(ctx, n1.ID, bob); !errors.Is(err, notification.ErrNotFound) {
		t.Fatalf("non-owner delete err=%v", err)
	}
	if err := repo.Delete(ctx, n1.ID, alice); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, n1.ID, alice); !errors.Is(err, notification.ErrNotFound) {
		t.Fatalf("repeat delete err=%v", err)
	}
	if err := repo.Delete(ctx, "not-a-uuid", alice); !errors.Is(err, notification.ErrNotFound) {
		t.Fatalf("invalid id delete err=%v", err)
	}
	all, _, _ := repo.ListPage(ctx, alice, nil, 10)
	if len(all) != 4 {
		t.Fatalf("after delete len=%d", len(all))
	}
}
