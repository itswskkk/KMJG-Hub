package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/itswskkk/KMJG-Hub/server/internal/github"
)

// TestGitHubStorePostgres exercises GitHub identity and repository
// persistence against a real database. Opt-in like the other integration
// tests: set KMJG_TEST_DATABASE_URL to a *test* database.
func TestGitHubStorePostgres(t *testing.T) {
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
	alice, bob := insertUser("gh-alice"), insertUser("gh-bob")
	insertProject := func(name string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO projects(name,created_by) VALUES($1,$2) RETURNING id`, name, alice).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	p1, p2 := insertProject("gh-one"), insertProject("gh-two")
	store := NewGitHubStore(pool)

	// Identities.
	if _, err := store.GetIdentity(ctx, alice); !errors.Is(err, github.ErrNotConnected) {
		t.Fatalf("unlinked identity: %v", err)
	}
	if _, err := store.GetIdentity(ctx, "not-a-uuid"); !errors.Is(err, github.ErrNotConnected) {
		t.Fatalf("invalid uuid identity: %v", err)
	}
	if err := store.SaveIdentity(ctx, alice, 1001, "alice-gh", []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	id, err := store.GetIdentity(ctx, alice)
	if err != nil || id.GitHubLogin != "alice-gh" || id.ConnectedAt.IsZero() {
		t.Fatalf("identity: %+v %v", id, err)
	}
	if tok, err := store.GetEncryptedAccessToken(ctx, alice); err != nil || string(tok) != "\x01\x02\x03" {
		t.Fatalf("token: %v %v", tok, err)
	}
	if err := store.SaveIdentity(ctx, alice, 1001, "alice-gh", []byte{4}); err != nil {
		t.Fatalf("re-save same account: %v", err)
	}
	if err := store.SaveIdentity(ctx, bob, 1001, "alice-gh", []byte{5}); !errors.Is(err, github.ErrIdentityInUse) {
		t.Fatalf("second user same account: %v", err)
	}
	if err := store.DisconnectIdentity(ctx, alice); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetEncryptedAccessToken(ctx, alice); !errors.Is(err, github.ErrNotConnected) {
		t.Fatalf("token after disconnect: %v", err)
	}
	if err := store.DisconnectIdentity(ctx, alice); !errors.Is(err, github.ErrNotConnected) {
		t.Fatalf("repeat disconnect: %v", err)
	}
	if err := store.SaveIdentity(ctx, bob, 1001, "alice-gh", []byte{5}); err != nil {
		t.Fatalf("account reusable after unlink: %v", err)
	}

	// Repositories.
	avail := github.AvailableRepository{ExternalID: 77, OwnerLogin: "octo", Name: "hub", HTMLURL: "https://github.com/octo/hub", DefaultBranch: "trunk"}
	if _, err := store.GetRepository(ctx, p1); !errors.Is(err, github.ErrNotFound) {
		t.Fatalf("no repo yet: %v", err)
	}
	repo, err := store.ConnectRepository(ctx, p1, alice, avail)
	if err != nil {
		t.Fatal(err)
	}
	if repo.ProjectID != p1 || repo.Provider != "github" || repo.ExternalID != 77 || repo.DefaultBranch != "trunk" ||
		repo.ConnectedByUserID != alice || !repo.NotifyAllMembers || !repo.NotifyAllBranches || repo.PostPushesToChat {
		t.Fatalf("repo: %+v", repo)
	}
	if _, err := store.ConnectRepository(ctx, p1, alice, avail); !errors.Is(err, github.ErrAlreadyConnected) {
		t.Fatalf("second connect: %v", err)
	}
	if _, err := store.ConnectRepository(ctx, "not-a-uuid", alice, avail); !errors.Is(err, github.ErrNotFound) {
		t.Fatalf("invalid project: %v", err)
	}
	if _, err := store.ConnectRepository(ctx, p2, bob, avail); err != nil {
		t.Fatalf("same repo, other project: %v", err)
	}
	byExt, err := store.ListRepositoriesByExternalID(ctx, 77)
	if err != nil || len(byExt) != 2 {
		t.Fatalf("by external id: %d %v", len(byExt), err)
	}
	if err := store.UpdateNotificationConfig(ctx, p1, true, false, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.GetRepository(ctx, p1); !got.PostPushesToChat || got.NotifyAllMembers || got.NotifyAllBranches {
		t.Fatalf("config: %+v", got)
	}

	// Deleting the connecting user keeps the connection (SET NULL).
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, bob); err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetRepository(ctx, p2); err != nil || got.ConnectedByUserID != "" {
		t.Fatalf("null connector: %+v %v", got, err)
	}

	if err := store.DisconnectRepository(ctx, p1); err != nil {
		t.Fatal(err)
	}
	if err := store.DisconnectRepository(ctx, p1); !errors.Is(err, github.ErrNotFound) {
		t.Fatalf("repeat disconnect: %v", err)
	}
	if err := store.UpdateNotificationConfig(ctx, p1, true, true, true); !errors.Is(err, github.ErrNotFound) {
		t.Fatalf("config without repo: %v", err)
	}
}
