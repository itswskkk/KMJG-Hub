package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/chat"
	"github.com/itswskkk/KMJG-Hub/server/internal/directmessage"
	"github.com/itswskkk/KMJG-Hub/server/internal/filetransfer"
	"github.com/itswskkk/KMJG-Hub/server/internal/invitation"
	"github.com/itswskkk/KMJG-Hub/server/internal/project"
	"github.com/itswskkk/KMJG-Hub/server/internal/task"
	"github.com/itswskkk/KMJG-Hub/server/internal/workcontext"
)

// TestDeletedProjectAccessPostgres proves that once a Project is
// soft-deleted, members who were still in it at deletion time can no longer
// read or write its Chat, Tasks, Work Context or Invitations through the
// repositories' membership-authorized queries, and that the deleted Project
// no longer counts as a shared Project for user-to-user eligibility. Restore
// brings access back. Opt-in like the other integration tests: set
// KMJG_TEST_DATABASE_URL to a *test* database.
func TestDeletedProjectAccessPostgres(t *testing.T) {
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
	owner, member, invitee, linkUser := insertUser("dp-owner"), insertUser("dp-member"), insertUser("dp-invitee"), insertUser("dp-linkuser")

	projects := NewProjectRepository(pool)
	chats := NewChatRepository(pool)
	tasks := NewTaskRepository(pool)
	contexts := NewWorkContextRepository(pool)
	invitations := NewInvitationRepository(pool)
	directMessages := NewDirectMessageRepository(pool)
	transfers := NewFileTransferRepository(pool)
	profiles := NewProfileRepository(pool, nil)

	p := &project.Project{Name: "deleted-project-access"}
	if err := projects.CreateWithOwner(ctx, p, owner); err != nil {
		t.Fatal(err)
	}
	projectID := p.ID
	if _, err := pool.Exec(ctx, `INSERT INTO project_members(project_id,user_id,role) VALUES($1,$2,'member')`, projectID, member); err != nil {
		t.Fatal(err)
	}

	// Data created while the Project is active.
	message, err := chats.CreateWithAttachment(ctx, projectID, member, "before delete", chat.Attachment{
		StorageID: "storage-dp-1", Filename: "a.txt", ContentType: "text/plain", SizeBytes: 3,
	}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	attachmentID := message.Attachments[0].ID
	tk, err := tasks.Create(ctx, projectID, owner, "task", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.AssignSelf(ctx, projectID, tk.ID, member); err != nil {
		t.Fatal(err)
	}
	assignment, err := tasks.RequestAssignment(ctx, projectID, tk.ID, owner, member)
	if err != nil {
		t.Fatal(err)
	}
	directInvite, err := invitations.CreateDirect(ctx, projectID, owner, "dp-invitee", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := invitations.CreateCredential(ctx, projectID, owner, "dp-token-hash", nil, nil); err != nil {
		t.Fatal(err)
	}
	if shares, err := profiles.ShareProject(ctx, owner, member); err != nil || !shares {
		t.Fatalf("ShareProject before delete = %v, %v; want true", shares, err)
	}

	if err := projects.Delete(ctx, projectID, owner); err != nil {
		t.Fatal(err)
	}

	isNotFound := func(t *testing.T, err error, want error) {
		t.Helper()
		if !errors.Is(err, want) {
			t.Fatalf("got err %v, want %v", err, want)
		}
	}

	t.Run("chat", func(t *testing.T) {
		_, err := chats.Create(ctx, projectID, member, "after delete")
		isNotFound(t, err, chat.ErrNotFound)
		_, err = chats.CreateWithAttachment(ctx, projectID, member, "after", chat.Attachment{StorageID: "storage-dp-2", Filename: "b.txt", ContentType: "text/plain", SizeBytes: 1}, 1<<20)
		isNotFound(t, err, chat.ErrNotFound)
		_, err = chats.ListPage(ctx, projectID, member, nil, 50)
		isNotFound(t, err, chat.ErrNotFound)
		_, err = chats.ListFiles(ctx, projectID, member)
		isNotFound(t, err, chat.ErrNotFound)
		_, err = chats.GetAttachment(ctx, projectID, attachmentID, member)
		isNotFound(t, err, chat.ErrNotFound)
		_, err = chats.SoftDelete(ctx, projectID, message.ID, owner)
		isNotFound(t, err, chat.ErrNotFound)
	})

	t.Run("tasks", func(t *testing.T) {
		_, err := tasks.List(ctx, projectID, member)
		isNotFound(t, err, task.ErrNotFound)
		_, err = tasks.Get(ctx, projectID, tk.ID, member)
		isNotFound(t, err, task.ErrNotFound)
		_, err = tasks.Create(ctx, projectID, member, "new", "", nil)
		isNotFound(t, err, task.ErrNotFound)
		_, err = tasks.SetStatus(ctx, projectID, tk.ID, member, task.Status("done"))
		isNotFound(t, err, task.ErrNotFound)
		_, err = tasks.AssignSelf(ctx, projectID, tk.ID, member)
		isNotFound(t, err, task.ErrNotFound)
		_, err = tasks.SetCurrent(ctx, projectID, tk.ID, member)
		isNotFound(t, err, task.ErrNotFound)
		_, err = tasks.RequestAssignment(ctx, projectID, tk.ID, member, owner)
		isNotFound(t, err, task.ErrNotFound)
		_, err = tasks.ListComments(ctx, projectID, tk.ID, member)
		isNotFound(t, err, task.ErrNotFound)
		_, err = tasks.AddComment(ctx, projectID, tk.ID, member, "comment")
		isNotFound(t, err, task.ErrNotFound)
		pending, err := tasks.ListPendingAssignments(ctx, member)
		if err != nil || len(pending) != 0 {
			t.Fatalf("ListPendingAssignments = %v, %v; want none", pending, err)
		}
		_, err = tasks.RespondAssignment(ctx, assignment.ID, member, true)
		isNotFound(t, err, task.ErrNotFound)
	})

	t.Run("work context", func(t *testing.T) {
		_, err := contexts.Upsert(ctx, projectID, member, true, "automatic", "main")
		isNotFound(t, err, workcontext.ErrNotFound)
		_, err = contexts.List(ctx, projectID, member)
		isNotFound(t, err, workcontext.ErrNotFound)
	})

	t.Run("invitations", func(t *testing.T) {
		now := time.Now()
		_, err := invitations.CreateDirect(ctx, projectID, owner, "dp-linkuser", nil)
		isNotFound(t, err, invitation.ErrNotFound)
		_, err = invitations.ListForProject(ctx, projectID, owner)
		isNotFound(t, err, invitation.ErrNotFound)
		received, err := invitations.ListReceived(ctx, invitee)
		if err != nil || len(received) != 0 {
			t.Fatalf("ListReceived = %v, %v; want none", received, err)
		}
		_, err = invitations.Accept(ctx, directInvite.ID, invitee, now)
		isNotFound(t, err, invitation.ErrNotFound)
		isNotFound(t, invitations.Decline(ctx, directInvite.ID, invitee, now), invitation.ErrNotFound)
		isNotFound(t, invitations.Cancel(ctx, projectID, directInvite.ID, owner, now), invitation.ErrNotFound)
		_, err = invitations.CreateCredential(ctx, projectID, owner, "dp-token-hash-2", nil, nil)
		isNotFound(t, err, invitation.ErrNotFound)
		_, err = invitations.ListCredentials(ctx, projectID, owner)
		isNotFound(t, err, invitation.ErrNotFound)
		_, err = invitations.ConsumeCredential(ctx, "dp-token-hash", linkUser, now)
		isNotFound(t, err, invitation.ErrNotFound)
		var joined bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id=$1 AND user_id IN ($2,$3))`, projectID, invitee, linkUser).Scan(&joined); err != nil {
			t.Fatal(err)
		}
		if joined {
			t.Fatal("a user joined a deleted Project through an invitation")
		}
	})

	t.Run("deleted project is not a shared project", func(t *testing.T) {
		if shares, err := profiles.ShareProject(ctx, owner, member); err != nil || shares {
			t.Fatalf("ShareProject after delete = %v, %v; want false", shares, err)
		}
		_, err := directMessages.Create(ctx, member, owner, "hi")
		isNotFound(t, err, directmessage.ErrForbidden)
		_, err = transfers.Create(ctx, member, owner, "f.txt", 10)
		isNotFound(t, err, filetransfer.ErrForbidden)
	})

	t.Run("restore brings access back", func(t *testing.T) {
		if err := projects.Restore(ctx, projectID, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := chats.Create(ctx, projectID, member, "after restore"); err != nil {
			t.Fatal(err)
		}
		if _, err := tasks.List(ctx, projectID, member); err != nil {
			t.Fatal(err)
		}
		if _, err := contexts.Upsert(ctx, projectID, member, true, "automatic", "main"); err != nil {
			t.Fatal(err)
		}
		if _, err := invitations.Accept(ctx, directInvite.ID, invitee, time.Now()); err != nil {
			t.Fatal(err)
		}
		if _, err := invitations.ConsumeCredential(ctx, "dp-token-hash", linkUser, time.Now()); err != nil {
			t.Fatal(err)
		}
	})
}
