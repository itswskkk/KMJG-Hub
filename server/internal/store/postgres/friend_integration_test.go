package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/itswskkk/KMJG-Hub/server/internal/friend"
)

// TestFriendRepositoryPostgres exercises the friend/block SQL and its
// transactional rules against a real database. Opt-in like the other
// integration tests: set KMJG_TEST_DATABASE_URL to a *test* database.
func TestFriendRepositoryPostgres(t *testing.T) {
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
	alice, bob, carol := insertUser("fr-alice"), insertUser("fr-bob"), insertUser("fr-carol")

	repo := NewFriendRepository(pool)
	profiles := NewProfileRepository(pool, nil)

	if id, err := repo.ResolveUser(ctx, "FR-BOB"); err != nil || id != bob {
		t.Fatalf("ResolveUser=%q,%v", id, err)
	}
	if _, err := repo.SendRequest(ctx, alice, "not-a-uuid"); !errors.Is(err, friend.ErrNotFound) {
		t.Fatalf("invalid recipient err=%v", err)
	}

	req, err := repo.SendRequest(ctx, alice, bob)
	if err != nil {
		t.Fatal(err)
	}
	if req.SenderUsername != "fr-alice" || req.RecipientUsername != "fr-bob" || req.Status != friend.StatusPending {
		t.Fatalf("req=%+v", req)
	}
	if _, err := repo.SendRequest(ctx, bob, alice); !errors.Is(err, friend.ErrRequestPending) {
		t.Fatalf("reverse pending err=%v", err)
	}
	incoming, err := repo.ListIncomingRequests(ctx, bob)
	if err != nil || len(incoming) != 1 || incoming[0].ID != req.ID {
		t.Fatalf("incoming=%+v err=%v", incoming, err)
	}

	accepted, err := repo.AcceptRequest(ctx, req.ID)
	if err != nil || accepted.Status != friend.StatusAccepted || accepted.RespondedAt == nil {
		t.Fatalf("accepted=%+v err=%v", accepted, err)
	}
	if _, err := repo.AcceptRequest(ctx, req.ID); !errors.Is(err, friend.ErrNotPending) {
		t.Fatalf("re-accept err=%v", err)
	}
	for _, p := range [][2]string{{alice, bob}, {bob, alice}} {
		if ok, err := repo.AreFriends(ctx, p[0], p[1]); err != nil || !ok {
			t.Fatalf("AreFriends(%v)=%v,%v", p, ok, err)
		}
		if ok, err := profiles.AreFollowers(ctx, p[0], p[1]); err != nil || !ok {
			t.Fatalf("AreFollowers(%v)=%v,%v", p, ok, err)
		}
	}
	if list, err := repo.ListFriends(ctx, bob); err != nil || len(list) != 1 || list[0].UserID != alice || list[0].Username != "fr-alice" {
		t.Fatalf("bob friends=%+v err=%v", list, err)
	}
	if _, err := repo.SendRequest(ctx, bob, alice); !errors.Is(err, friend.ErrAlreadyFriends) {
		t.Fatalf("already friends err=%v", err)
	}

	// Blocking removes the friendship and pending requests atomically.
	pending, err := repo.SendRequest(ctx, carol, alice)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Block(ctx, alice, bob); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Block(ctx, alice, carol); err != nil {
		t.Fatal(err)
	}
	if again, err := repo.Block(ctx, alice, carol); err != nil || again.BlockedUsername != "fr-carol" {
		t.Fatalf("idempotent block=%+v err=%v", again, err)
	}
	if ok, _ := repo.AreFriends(ctx, alice, bob); ok {
		t.Fatal("friendship survived block")
	}
	if _, err := repo.GetRequest(ctx, pending.ID); !errors.Is(err, friend.ErrNotFound) {
		t.Fatalf("pending request survived block: %v", err)
	}
	if _, err := repo.SendRequest(ctx, bob, alice); !errors.Is(err, friend.ErrBlocked) {
		t.Fatalf("blocked request err=%v", err)
	}
	if ok, err := profiles.IsBlocked(ctx, bob, alice); err != nil || !ok {
		t.Fatalf("profile IsBlocked(bob by alice)=%v,%v", ok, err)
	}
	if ok, _ := profiles.IsBlocked(ctx, alice, bob); ok {
		t.Fatal("block must be directional")
	}
	if list, err := repo.ListBlocked(ctx, alice); err != nil || len(list) != 2 {
		t.Fatalf("blocked=%+v err=%v", list, err)
	}

	if err := repo.Unblock(ctx, alice, bob); err != nil {
		t.Fatal(err)
	}
	if err := repo.Unblock(ctx, alice, bob); !errors.Is(err, friend.ErrNotFound) {
		t.Fatalf("unblock again err=%v", err)
	}
	// The old accepted (alice->bob) row is re-opened, not duplicated.
	reopened, err := repo.SendRequest(ctx, alice, bob)
	if err != nil || reopened.ID != req.ID || reopened.Status != friend.StatusPending {
		t.Fatalf("reopened=%+v err=%v", reopened, err)
	}
	if declined, err := repo.DeclineRequest(ctx, reopened.ID); err != nil || declined.Status != friend.StatusDeclined {
		t.Fatalf("declined=%+v err=%v", declined, err)
	}
	if err := repo.CancelRequest(ctx, reopened.ID); !errors.Is(err, friend.ErrNotPending) {
		t.Fatalf("cancel declined err=%v", err)
	}
	out, err := repo.SendRequest(ctx, bob, alice)
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := repo.ListOutgoingRequests(ctx, bob); len(list) != 1 {
		t.Fatalf("outgoing=%+v", list)
	}
	if err := repo.CancelRequest(ctx, out.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveFriendship(ctx, alice, bob); !errors.Is(err, friend.ErrNotFound) {
		t.Fatalf("remove non-friend err=%v", err)
	}
}
