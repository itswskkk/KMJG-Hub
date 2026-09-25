package task_test

import (
	"context"
	"errors"
	"testing"

	"github.com/itswskkk/KMJG-Hub/server/internal/task"
)

type notifyCall struct {
	userID, eventType string
	payload           map[string]any
}

type fakeNotifier struct {
	calls []notifyCall
	err   error
}

func (f *fakeNotifier) Notify(_ context.Context, userID, eventType string, payload any) error {
	p, _ := payload.(map[string]any)
	f.calls = append(f.calls, notifyCall{userID, eventType, p})
	return f.err
}

func TestAssignToOtherMemberNotifiesAssignee(t *testing.T) {
	n := &fakeNotifier{}
	svc := &task.Service{Repo: &fakeRepo{}, Notifier: n}
	if _, _, err := svc.Assign(context.Background(), "alice", "project-1", "task-1", "bob"); err != nil {
		t.Fatal(err)
	}
	if len(n.calls) != 1 || n.calls[0].userID != "bob" || n.calls[0].eventType != "task_assigned" {
		t.Fatalf("calls=%+v", n.calls)
	}
	p := n.calls[0].payload
	if p["project_id"] != "project-1" || p["task_id"] != "task-1" || p["assigned_by_user_id"] != "alice" {
		t.Fatalf("payload=%v", p)
	}
}

func TestSelfAssignDoesNotNotify(t *testing.T) {
	n := &fakeNotifier{}
	svc := &task.Service{Repo: &fakeRepo{}, Notifier: n}
	if _, _, err := svc.Assign(context.Background(), "alice", "project-1", "task-1", "alice"); err != nil {
		t.Fatal(err)
	}
	if len(n.calls) != 0 {
		t.Fatalf("calls=%+v", n.calls)
	}
}

func TestCommentNotifiesCreatorAndAssigneeExceptAuthor(t *testing.T) {
	assignee := "carol"
	repo := &fakeRepo{task: &task.Task{ID: "task-1", ProjectID: "project-1", Title: "Ship it", CreatorID: "alice", AssigneeID: &assignee}}
	n := &fakeNotifier{}
	svc := &task.Service{Repo: repo, Notifier: n}

	if _, err := svc.AddComment(context.Background(), "alice", "project-1", "task-1", "looks good"); err != nil {
		t.Fatal(err)
	}
	if len(n.calls) != 1 || n.calls[0].userID != "carol" || n.calls[0].eventType != "task_comment" {
		t.Fatalf("author=creator: calls=%+v", n.calls)
	}
	if n.calls[0].payload["task_title"] != "Ship it" || n.calls[0].payload["body_preview"] != "looks good" {
		t.Fatalf("payload=%v", n.calls[0].payload)
	}

	n.calls = nil
	if _, err := svc.AddComment(context.Background(), "dave", "project-1", "task-1", "hi"); err != nil {
		t.Fatal(err)
	}
	if len(n.calls) != 2 || n.calls[0].userID != "alice" || n.calls[1].userID != "carol" {
		t.Fatalf("third-party author: calls=%+v", n.calls)
	}

	// Creator is also assignee: notified once.
	self := "alice"
	repo.task.AssigneeID = &self
	n.calls = nil
	if _, err := svc.AddComment(context.Background(), "dave", "project-1", "task-1", "hi"); err != nil {
		t.Fatal(err)
	}
	if len(n.calls) != 1 || n.calls[0].userID != "alice" {
		t.Fatalf("dedupe: calls=%+v", n.calls)
	}
}

func TestNotificationFailureDoesNotFailPrimaryAction(t *testing.T) {
	n := &fakeNotifier{err: errors.New("db down")}
	svc := &task.Service{Repo: &fakeRepo{task: &task.Task{ID: "task-1", CreatorID: "alice"}}, Notifier: n}
	if _, r, err := svc.Assign(context.Background(), "alice", "project-1", "task-1", "bob"); err != nil || r == nil {
		t.Fatalf("assign should succeed: r=%v err=%v", r, err)
	}
	if c, err := svc.AddComment(context.Background(), "bob", "project-1", "task-1", "hi"); err != nil || c == nil {
		t.Fatalf("comment should succeed: c=%v err=%v", c, err)
	}
	if len(n.calls) != 2 {
		t.Fatalf("calls=%+v", n.calls)
	}
}
