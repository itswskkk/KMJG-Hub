package notification_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/itswskkk/KMJG-Hub/server/internal/notification"
	"github.com/itswskkk/KMJG-Hub/server/internal/notification/notificationtest"
)

type recordingPublisher struct {
	mu        sync.Mutex
	published []notification.Notification
}

func (p *recordingPublisher) PublishNotificationCreated(n notification.Notification) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.published = append(p.published, n)
}

func newService() (*notification.Service, *notificationtest.Memory, *recordingPublisher) {
	repo := notificationtest.NewMemory()
	pub := &recordingPublisher{}
	return &notification.Service{Repo: repo, Publisher: pub}, repo, pub
}

func TestCreatePersistsAndPublishesToOwner(t *testing.T) {
	svc, _, pub := newService()
	ctx := context.Background()
	n, err := svc.Create(ctx, "alice", notification.EventFriendRequest, map[string]string{"sender_id": "bob"})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	if err := json.Unmarshal(n.Payload, &payload); err != nil || payload["sender_id"] != "bob" {
		t.Fatalf("payload=%s err=%v", n.Payload, err)
	}
	if len(pub.published) != 1 || pub.published[0].UserID != "alice" {
		t.Fatalf("published=%+v", pub.published)
	}
	if err := svc.Notify(ctx, "alice", "direct_message", nil); err != nil {
		t.Fatal(err)
	}
	if len(pub.published) != 2 || pub.published[1].EventType != notification.EventDirectMessage {
		t.Fatalf("published=%+v", pub.published)
	}
}

func TestCreateFailureIsNotPublished(t *testing.T) {
	svc, repo, pub := newService()
	repo.FailCreate = errors.New("boom")
	if err := svc.Notify(context.Background(), "alice", "task_assigned", nil); err == nil {
		t.Fatal("expected error")
	}
	if len(pub.published) != 0 {
		t.Fatal("failed create must not publish")
	}
}

func TestListPagePaginatesNewestFirstWithUnreadCount(t *testing.T) {
	svc, _, _ := newService()
	ctx := context.Background()
	total := notification.RecentLimit + 5
	for i := 0; i < total; i++ {
		if _, err := svc.Create(ctx, "alice", notification.EventTaskComment, map[string]int{"i": i}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.Create(ctx, "bob", notification.EventTaskComment, nil); err != nil {
		t.Fatal(err)
	}

	first, err := svc.ListPage(ctx, "alice", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Notifications) != notification.RecentLimit || first.NextCursor == "" || first.UnreadCount != total {
		t.Fatalf("first page: len=%d cursor=%q unread=%d", len(first.Notifications), first.NextCursor, first.UnreadCount)
	}
	for i := 1; i < len(first.Notifications); i++ {
		if first.Notifications[i].CreatedAt.After(first.Notifications[i-1].CreatedAt) {
			t.Fatal("page not newest-first")
		}
	}
	second, err := svc.ListPage(ctx, "alice", first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Notifications) != 5 || second.NextCursor != "" {
		t.Fatalf("second page: len=%d cursor=%q", len(second.Notifications), second.NextCursor)
	}
	seen := map[string]bool{}
	for _, n := range append(first.Notifications, second.Notifications...) {
		if n.UserID != "alice" || seen[n.ID] {
			t.Fatalf("unexpected notification %+v", n)
		}
		seen[n.ID] = true
	}

	var ve *notification.ValidationError
	if _, err := svc.ListPage(ctx, "alice", "!!not-a-cursor"); !errors.As(err, &ve) {
		t.Fatalf("bad cursor err=%v", err)
	}
}

func TestMarkReadOnlyByOwner(t *testing.T) {
	svc, _, _ := newService()
	ctx := context.Background()
	n, _ := svc.Create(ctx, "alice", notification.EventTaskAssigned, nil)
	if _, err := svc.Create(ctx, "alice", notification.EventTaskAssigned, nil); err != nil {
		t.Fatal(err)
	}

	if err := svc.MarkRead(ctx, n.ID, "bob"); !errors.Is(err, notification.ErrNotFound) {
		t.Fatalf("non-owner mark read err=%v", err)
	}
	if err := svc.MarkRead(ctx, "missing", "alice"); !errors.Is(err, notification.ErrNotFound) {
		t.Fatalf("missing mark read err=%v", err)
	}
	if err := svc.MarkRead(ctx, n.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := svc.MarkRead(ctx, n.ID, "alice"); err != nil {
		t.Fatalf("repeat mark read should be idempotent: %v", err)
	}
	page, _ := svc.ListPage(ctx, "alice", "")
	if page.UnreadCount != 1 {
		t.Fatalf("unread=%d", page.UnreadCount)
	}
}

func TestDeleteOnlyByOwner(t *testing.T) {
	svc, _, _ := newService()
	ctx := context.Background()
	n, _ := svc.Create(ctx, "alice", notification.EventProjectInvitation, nil)

	if err := svc.Delete(ctx, n.ID, "bob"); !errors.Is(err, notification.ErrNotFound) {
		t.Fatalf("non-owner delete err=%v", err)
	}
	if err := svc.Delete(ctx, n.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, n.ID, "alice"); !errors.Is(err, notification.ErrNotFound) {
		t.Fatalf("repeat delete err=%v", err)
	}
	page, _ := svc.ListPage(ctx, "alice", "")
	if len(page.Notifications) != 0 || page.UnreadCount != 0 {
		t.Fatalf("page=%+v", page)
	}
}
