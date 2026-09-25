package directmessage_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/itswskkk/KMJG-Hub/server/internal/directmessage"
)

type fakeNotifier struct {
	userIDs  []string
	payloads []map[string]any
	err      error
}

func (f *fakeNotifier) Notify(_ context.Context, userID, eventType string, payload any) error {
	if eventType != "direct_message" {
		return errors.New("unexpected event type " + eventType)
	}
	p, _ := payload.(map[string]any)
	f.userIDs = append(f.userIDs, userID)
	f.payloads = append(f.payloads, p)
	return f.err
}

func TestSendNotifiesRecipientWithPreview(t *testing.T) {
	n := &fakeNotifier{}
	svc := &directmessage.Service{Repo: newFakeRepo(allowAll), Notifier: n}
	body := strings.Repeat("é", 150)
	m, err := svc.Send(context.Background(), "alice", "bob", body)
	if err != nil {
		t.Fatal(err)
	}
	if len(n.userIDs) != 1 || n.userIDs[0] != "bob" {
		t.Fatalf("notified=%v", n.userIDs)
	}
	p := n.payloads[0]
	if p["sender_id"] != "alice" || p["message_id"] != m.ID || p["body_preview"] != strings.Repeat("é", 100) {
		t.Fatalf("payload=%v", p)
	}
}

func TestSendSucceedsWhenNotificationFails(t *testing.T) {
	n := &fakeNotifier{err: errors.New("db down")}
	svc := &directmessage.Service{Repo: newFakeRepo(allowAll), Notifier: n}
	if _, err := svc.Send(context.Background(), "alice", "bob", "hi"); err != nil {
		t.Fatalf("send should succeed: %v", err)
	}
}

func TestForbiddenSendDoesNotNotify(t *testing.T) {
	n := &fakeNotifier{}
	svc := &directmessage.Service{Repo: newFakeRepo(func(string, string) bool { return false }), Notifier: n}
	if _, err := svc.Send(context.Background(), "alice", "bob", "hi"); !errors.Is(err, directmessage.ErrForbidden) {
		t.Fatalf("err=%v", err)
	}
	if len(n.userIDs) != 0 {
		t.Fatalf("notified=%v", n.userIDs)
	}
}
