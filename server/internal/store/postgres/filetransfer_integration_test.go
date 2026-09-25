package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/itswskkk/KMJG-Hub/server/internal/filetransfer"
)

// TestFileTransferRepositoryPostgres exercises the SQL-enforced transfer
// access rules and state machine. Opt-in like the other integration tests:
// set KMJG_TEST_DATABASE_URL to a *test* database.
func TestFileTransferRepositoryPostgres(t *testing.T) {
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
	alice, bob, erin := insertUser("ft-alice"), insertUser("ft-bob"), insertUser("ft-erin")
	friends := NewFriendRepository(pool)
	req, err := friends.SendRequest(ctx, alice, bob)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := friends.AcceptRequest(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	repo := NewFileTransferRepository(pool)

	if _, err := repo.Create(ctx, erin, alice, "a.txt", 5); !errors.Is(err, filetransfer.ErrForbidden) {
		t.Fatalf("unrelated create err=%v", err)
	}
	if _, err := repo.Create(ctx, alice, "00000000-0000-0000-0000-000000000000", "a.txt", 5); !errors.Is(err, filetransfer.ErrNotFound) {
		t.Fatalf("unknown recipient err=%v", err)
	}
	if _, err := repo.Create(ctx, alice, "not-a-uuid", "a.txt", 5); !errors.Is(err, filetransfer.ErrNotFound) {
		t.Fatalf("malformed recipient err=%v", err)
	}

	tr, err := repo.Create(ctx, alice, bob, "a.txt", 5)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Status != filetransfer.StatusPending || tr.SenderUsername != "ft-alice" || tr.RecipientUsername != "ft-bob" || tr.StorageID != nil {
		t.Fatalf("created=%+v", tr)
	}
	if _, err := repo.Get(ctx, tr.ID, erin); !errors.Is(err, filetransfer.ErrNotFound) {
		t.Fatalf("outsider get err=%v", err)
	}
	if _, err := repo.Get(ctx, "garbage", alice); !errors.Is(err, filetransfer.ErrNotFound) {
		t.Fatalf("malformed get err=%v", err)
	}
	if _, err := repo.MarkUploaded(ctx, tr.ID, alice, "s1", 5, "text/plain"); !errors.Is(err, filetransfer.ErrInvalidState) {
		t.Fatalf("upload before accept err=%v", err)
	}
	if _, err := repo.Accept(ctx, tr.ID, alice); !errors.Is(err, filetransfer.ErrForbidden) {
		t.Fatalf("sender accept err=%v", err)
	}
	if _, err := repo.Accept(ctx, tr.ID, erin); !errors.Is(err, filetransfer.ErrNotFound) {
		t.Fatalf("outsider accept err=%v", err)
	}
	accepted, err := repo.Accept(ctx, tr.ID, bob)
	if err != nil || accepted.Status != filetransfer.StatusAccepted || accepted.RespondedAt == nil {
		t.Fatalf("accept=%+v err=%v", accepted, err)
	}
	if _, err := repo.Decline(ctx, tr.ID, bob); !errors.Is(err, filetransfer.ErrInvalidState) {
		t.Fatalf("decline after accept err=%v", err)
	}
	if _, err := repo.MarkUploaded(ctx, tr.ID, bob, "s1", 5, "text/plain"); !errors.Is(err, filetransfer.ErrForbidden) {
		t.Fatalf("recipient mark uploaded err=%v", err)
	}
	if _, err := repo.MarkUploaded(ctx, tr.ID, alice, "s1", 6, "text/plain"); !errors.Is(err, filetransfer.ErrInvalidState) {
		t.Fatalf("size mismatch mark uploaded err=%v", err)
	}
	uploaded, err := repo.MarkUploaded(ctx, tr.ID, alice, "s1", 5, "text/plain")
	if err != nil || uploaded.Status != filetransfer.StatusUploaded || *uploaded.StorageID != "s1" || *uploaded.ActualFileSize != 5 || uploaded.UploadedAt == nil {
		t.Fatalf("uploaded=%+v err=%v", uploaded, err)
	}
	if _, err := repo.Cancel(ctx, tr.ID, alice); !errors.Is(err, filetransfer.ErrInvalidState) {
		t.Fatalf("cancel after upload err=%v", err)
	}

	second, _ := repo.Create(ctx, bob, alice, "b.txt", 7)
	if _, err := repo.Cancel(ctx, second.ID, alice); !errors.Is(err, filetransfer.ErrForbidden) {
		t.Fatalf("recipient cancel err=%v", err)
	}
	if cancelled, err := repo.Cancel(ctx, second.ID, bob); err != nil || cancelled.Status != filetransfer.StatusCancelled {
		t.Fatalf("cancel err=%v", err)
	}

	incoming, err := repo.ListIncoming(ctx, bob)
	if err != nil || len(incoming) != 1 || incoming[0].ID != tr.ID {
		t.Fatalf("bob incoming=%+v err=%v", incoming, err)
	}
	sent, err := repo.ListSent(ctx, bob)
	if err != nil || len(sent) != 1 || sent[0].ID != second.ID {
		t.Fatalf("bob sent=%+v err=%v", sent, err)
	}

	// Blocking revokes the ability to request.
	if _, err := friends.Block(ctx, bob, alice); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, alice, bob, "c.txt", 5); !errors.Is(err, filetransfer.ErrForbidden) {
		t.Fatalf("blocked create err=%v", err)
	}
}
