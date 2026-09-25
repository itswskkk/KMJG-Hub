package chat_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/chat"
)

type fakeRepo struct {
	createdBody string
	message     *chat.Message
	createErr   error
	deleteErr   error
	purgeCutoff time.Time
	listLimit   int
}

func (f *fakeRepo) Create(_ context.Context, projectID, authorID, body string) (*chat.Message, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.createdBody = body
	message := &chat.Message{ID: "m-1", ProjectID: projectID, AuthorID: authorID, Body: body, CreatedAt: time.Now()}
	f.message = message
	return message, nil
}
func (f *fakeRepo) ListRecent(_ context.Context, _, _ string, limit int) ([]chat.Message, error) {
	f.listLimit = limit
	return nil, nil
}
func (f *fakeRepo) SoftDelete(_ context.Context, projectID, messageID, _ string) (*chat.Message, error) {
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	return &chat.Message{ID: messageID, ProjectID: projectID}, nil
}
func (f *fakeRepo) PurgeDeletedBefore(_ context.Context, cutoff time.Time) error {
	f.purgeCutoff = cutoff
	return nil
}

type fakeMembership struct{ ids []string }

func (f fakeMembership) MemberUserIDs(context.Context, string) ([]string, error) { return f.ids, nil }

type fakePublisher struct{ created, deleted []string }

func (f *fakePublisher) PublishMessageCreated(userID string, _ chat.Message) {
	f.created = append(f.created, userID)
}
func (f *fakePublisher) PublishMessageDeleted(userID, _, _ string) {
	f.deleted = append(f.deleted, userID)
}

func TestSendValidatesTrimsPersistsThenPublishes(t *testing.T) {
	repo := &fakeRepo{}
	publisher := &fakePublisher{}
	svc := &chat.Service{Repo: repo, Membership: fakeMembership{ids: []string{"u1", "u2"}}, Publisher: publisher}

	message, err := svc.Send(context.Background(), "u1", "p1", "  hello team  ")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if message.Body != "hello team" || repo.createdBody != "hello team" {
		t.Fatalf("message was not trimmed: %+v", message)
	}
	if len(publisher.created) != 2 {
		t.Fatalf("expected two recipients, got %v", publisher.created)
	}
}

func TestListRecentUsesBoundedHistoryLimit(t *testing.T) {
	repo := &fakeRepo{}
	svc := &chat.Service{Repo: repo}
	if _, err := svc.ListRecent(context.Background(), "u1", "p1"); err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if repo.listLimit != chat.RecentMessageLimit {
		t.Fatalf("limit = %d, want %d", repo.listLimit, chat.RecentMessageLimit)
	}
}

func TestSendRejectsEmptyAndOversizedMessages(t *testing.T) {
	svc := &chat.Service{Repo: &fakeRepo{}}
	for _, body := range []string{"   ", strings.Repeat("ก", chat.MaxMessageCharacters+1)} {
		_, err := svc.Send(context.Background(), "u1", "p1", body)
		var validationErr *chat.ValidationError
		if !errors.As(err, &validationErr) {
			t.Fatalf("expected ValidationError for %q, got %v", body[:min(len(body), 3)], err)
		}
	}
}

func TestSendAcceptsFourThousandUnicodeCodePoints(t *testing.T) {
	repo := &fakeRepo{}
	svc := &chat.Service{Repo: repo}
	body := strings.Repeat("😀", chat.MaxMessageCharacters)
	if _, err := svc.Send(context.Background(), "u1", "p1", body); err != nil {
		t.Fatalf("expected %d emoji to be accepted, got %v", chat.MaxMessageCharacters, err)
	}
}

func TestSendDoesNotPublishWhenPersistenceFails(t *testing.T) {
	publisher := &fakePublisher{}
	svc := &chat.Service{Repo: &fakeRepo{createErr: chat.ErrNotFound}, Membership: fakeMembership{ids: []string{"u1"}}, Publisher: publisher}
	if _, err := svc.Send(context.Background(), "u1", "p1", "hello"); !errors.Is(err, chat.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if len(publisher.created) != 0 {
		t.Fatalf("published before persistence: %v", publisher.created)
	}
}

func TestDeletePublishesOnlyAfterSuccessfulSoftDelete(t *testing.T) {
	publisher := &fakePublisher{}
	repo := &fakeRepo{}
	svc := &chat.Service{Repo: repo, Membership: fakeMembership{ids: []string{"u1", "u2"}}, Publisher: publisher}
	if err := svc.Delete(context.Background(), "u1", "p1", "m1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(publisher.deleted) != 2 {
		t.Fatalf("expected two recipients, got %v", publisher.deleted)
	}

	repo.deleteErr = chat.ErrForbidden
	if err := svc.Delete(context.Background(), "u1", "p1", "m2"); !errors.Is(err, chat.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if len(publisher.deleted) != 2 {
		t.Fatal("failed deletion was published")
	}
}

func TestPurgeExpiredDeletedUsesThirtyDayCutoff(t *testing.T) {
	repo := &fakeRepo{}
	svc := &chat.Service{Repo: repo}
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	if err := svc.PurgeExpiredDeleted(context.Background(), now); err != nil {
		t.Fatalf("PurgeExpiredDeleted: %v", err)
	}
	want := now.Add(-30 * 24 * time.Hour)
	if !repo.purgeCutoff.Equal(want) {
		t.Fatalf("cutoff = %v, want %v", repo.purgeCutoff, want)
	}
}
