package directmessage_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/directmessage"
)

// fakeRepo is an in-memory directmessage.Repository. allowed decides whether
// a pair may exchange messages, standing in for the SQL relationship check.
type fakeRepo struct {
	nextID   int
	messages []directmessage.Message
	deleted  map[string]bool
	allowed  func(a, b string) bool
	purgedAt time.Time
}

func newFakeRepo(allowed func(a, b string) bool) *fakeRepo {
	return &fakeRepo{deleted: map[string]bool{}, allowed: allowed}
}

func (f *fakeRepo) Create(_ context.Context, senderID, recipientID, body string) (*directmessage.Message, error) {
	if !f.allowed(senderID, recipientID) {
		return nil, directmessage.ErrForbidden
	}
	f.nextID++
	m := directmessage.Message{ID: fmt.Sprintf("dm-%d", f.nextID), SenderID: senderID, RecipientID: recipientID, Body: body,
		CreatedAt: time.Unix(int64(f.nextID), 0).UTC()}
	f.messages = append(f.messages, m)
	return &m, nil
}

func (f *fakeRepo) ListPage(_ context.Context, viewerID, otherUserID string, before *directmessage.Cursor, limit int) ([]directmessage.Message, error) {
	var matching []directmessage.Message
	for _, m := range f.messages {
		if f.deleted[m.ID] {
			continue
		}
		if !((m.SenderID == viewerID && m.RecipientID == otherUserID) || (m.SenderID == otherUserID && m.RecipientID == viewerID)) {
			continue
		}
		if before != nil && !m.CreatedAt.Before(before.CreatedAt) {
			continue
		}
		matching = append(matching, m)
	}
	if len(matching) > limit {
		matching = matching[len(matching)-limit:]
	}
	return matching, nil
}

func (f *fakeRepo) ListConversations(context.Context, string) ([]directmessage.Conversation, error) {
	return []directmessage.Conversation{}, nil
}

func (f *fakeRepo) SoftDelete(_ context.Context, messageID, actorID string) (*directmessage.Message, error) {
	for _, m := range f.messages {
		if m.ID != messageID || f.deleted[m.ID] {
			continue
		}
		if m.SenderID == actorID {
			f.deleted[m.ID] = true
			return &m, nil
		}
		if m.RecipientID == actorID {
			return nil, directmessage.ErrForbidden
		}
	}
	return nil, directmessage.ErrNotFound
}

func (f *fakeRepo) PurgeDeletedBefore(_ context.Context, cutoff time.Time) error {
	f.purgedAt = cutoff
	return nil
}

type fakePublisher struct {
	created []directmessage.Message
	deleted []string
}

func (p *fakePublisher) PublishMessageCreated(m directmessage.Message) {
	p.created = append(p.created, m)
}
func (p *fakePublisher) PublishMessageDeleted(senderID, recipientID, messageID string) {
	p.deleted = append(p.deleted, senderID+"|"+recipientID+"|"+messageID)
}

func allowAll(string, string) bool { return true }

func TestSendValidatesBody(t *testing.T) {
	pub := &fakePublisher{}
	svc := &directmessage.Service{Repo: newFakeRepo(allowAll), Publisher: pub}
	ctx := context.Background()

	for name, body := range map[string]string{
		"empty":      "",
		"whitespace": "   \n\t ",
		"oversized":  strings.Repeat("a", directmessage.MaxMessageCharacters+1),
		"bad utf8":   "\xff\xfe",
	} {
		_, err := svc.Send(ctx, "alice", "bob", body)
		var ve *directmessage.ValidationError
		if !errors.As(err, &ve) || ve.Field != "body" {
			t.Errorf("%s: expected body ValidationError, got %v", name, err)
		}
	}
	if len(pub.created) != 0 {
		t.Fatalf("invalid sends published events: %+v", pub.created)
	}

	// Exactly at the limit (counted in characters, not bytes) is accepted.
	if _, err := svc.Send(ctx, "alice", "bob", strings.Repeat("é", directmessage.MaxMessageCharacters)); err != nil {
		t.Fatalf("max-length body rejected: %v", err)
	}
	var ve *directmessage.ValidationError
	if _, err := svc.Send(ctx, "alice", "alice", "hi"); !errors.As(err, &ve) {
		t.Fatalf("self-send: expected ValidationError, got %v", err)
	}
}

func TestSendTrimsAndPublishesToRecipient(t *testing.T) {
	pub := &fakePublisher{}
	svc := &directmessage.Service{Repo: newFakeRepo(allowAll), Publisher: pub}
	m, err := svc.Send(context.Background(), "alice", "bob", "  hello  ")
	if err != nil {
		t.Fatal(err)
	}
	if m.Body != "hello" {
		t.Fatalf("body not trimmed: %q", m.Body)
	}
	if len(pub.created) != 1 || pub.created[0].ID != m.ID || pub.created[0].RecipientID != "bob" {
		t.Fatalf("unexpected published events: %+v", pub.created)
	}
}

func TestSendForbiddenDoesNotPublish(t *testing.T) {
	pub := &fakePublisher{}
	svc := &directmessage.Service{Repo: newFakeRepo(func(string, string) bool { return false }), Publisher: pub}
	if _, err := svc.Send(context.Background(), "alice", "bob", "hi"); !errors.Is(err, directmessage.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if len(pub.created) != 0 {
		t.Fatal("forbidden send published an event")
	}
}

func TestDeleteOnlyBySender(t *testing.T) {
	pub := &fakePublisher{}
	svc := &directmessage.Service{Repo: newFakeRepo(allowAll), Publisher: pub}
	ctx := context.Background()
	m, err := svc.Send(ctx, "alice", "bob", "oops")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, m.ID, "bob"); !errors.Is(err, directmessage.ErrForbidden) {
		t.Fatalf("recipient delete: expected ErrForbidden, got %v", err)
	}
	if err := svc.Delete(ctx, m.ID, "carol"); !errors.Is(err, directmessage.ErrNotFound) {
		t.Fatalf("outsider delete: expected ErrNotFound, got %v", err)
	}
	if len(pub.deleted) != 0 {
		t.Fatal("rejected delete published an event")
	}
	if err := svc.Delete(ctx, m.ID, "alice"); err != nil {
		t.Fatalf("sender delete: %v", err)
	}
	if len(pub.deleted) != 1 || pub.deleted[0] != "alice|bob|"+m.ID {
		t.Fatalf("unexpected delete events: %v", pub.deleted)
	}
	if err := svc.Delete(ctx, m.ID, "alice"); !errors.Is(err, directmessage.ErrNotFound) {
		t.Fatalf("second delete: expected ErrNotFound, got %v", err)
	}
}

func TestListPagePaginates(t *testing.T) {
	svc := &directmessage.Service{Repo: newFakeRepo(allowAll)}
	ctx := context.Background()
	total := directmessage.RecentMessageLimit + 5
	for i := 0; i < total; i++ {
		if _, err := svc.Send(ctx, "alice", "bob", fmt.Sprintf("m%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	first, err := svc.ListPage(ctx, "bob", "alice", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Messages) != directmessage.RecentMessageLimit || first.NextCursor == "" {
		t.Fatalf("first page: %d messages, cursor=%q", len(first.Messages), first.NextCursor)
	}
	if last := first.Messages[len(first.Messages)-1].Body; last != fmt.Sprintf("m%d", total-1) {
		t.Fatalf("first page should end at newest message, got %q", last)
	}
	second, err := svc.ListPage(ctx, "bob", "alice", first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Messages) != 5 || second.NextCursor != "" || second.Messages[0].Body != "m0" {
		t.Fatalf("second page: %+v cursor=%q", second.Messages, second.NextCursor)
	}

	var ve *directmessage.ValidationError
	if _, err := svc.ListPage(ctx, "bob", "alice", "!!not-a-cursor"); !errors.As(err, &ve) {
		t.Fatalf("bad cursor: expected ValidationError, got %v", err)
	}
}

func TestPurgeUsesRetentionWindow(t *testing.T) {
	repo := newFakeRepo(allowAll)
	svc := &directmessage.Service{Repo: repo}
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	if err := svc.PurgeExpiredDeleted(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if want := now.Add(-30 * 24 * time.Hour); !repo.purgedAt.Equal(want) {
		t.Fatalf("cutoff=%v want %v", repo.purgedAt, want)
	}
}
