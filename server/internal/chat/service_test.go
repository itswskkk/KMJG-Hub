package chat_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/chat"
	"github.com/itswskkk/KMJG-Hub/server/internal/storage"
)

type fakeRepo struct {
	createdBody string
	message     *chat.Message
	createErr   error
	deleteErr   error
	purgeCutoff time.Time
	listLimit   int
	attachment  chat.Attachment
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
func (f *fakeRepo) ListPage(_ context.Context, _, _ string, _ *chat.Cursor, limit int) ([]chat.Message, error) {
	f.listLimit = limit
	return nil, nil
}
func (f *fakeRepo) CreateWithAttachment(_ context.Context, projectID, authorID, body string, attachment chat.Attachment, _ int64) (*chat.Message, error) {
	attachment.ID = "a1"
	attachment.MessageID = "m-attachment"
	attachment.ProjectID = projectID
	f.attachment = attachment
	return &chat.Message{ID: "m-attachment", ProjectID: projectID, AuthorID: authorID, Body: body, Attachments: []chat.Attachment{attachment}, CreatedAt: time.Now()}, nil
}
func (f *fakeRepo) GetAttachment(context.Context, string, string, string) (*chat.Attachment, error) {
	if f.attachment.ID == "" {
		return nil, chat.ErrNotFound
	}
	copy := f.attachment
	return &copy, nil
}
func (f *fakeRepo) ListExpiredAttachmentStorageIDs(context.Context, time.Time) ([]string, error) {
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

func TestListPageUsesBoundedHistoryLimit(t *testing.T) {
	repo := &fakeRepo{}
	svc := &chat.Service{Repo: repo}
	if _, err := svc.ListPage(context.Background(), "u1", "p1", ""); err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	if repo.listLimit != chat.RecentMessageLimit+1 {
		t.Fatalf("limit = %d, want %d", repo.listLimit, chat.RecentMessageLimit+1)
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

func TestSendAndOpenAttachmentUsesOpaqueStorageAndPublishes(t *testing.T) {
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeRepo{}
	publisher := &fakePublisher{}
	svc := &chat.Service{Repo: repo, Storage: store, MaxUploadBytes: 1024, MaxProjectStorageBytes: 4096, Membership: fakeMembership{ids: []string{"u1"}}, Publisher: publisher}
	message, err := svc.SendAttachment(context.Background(), "u1", "p1", " note ", "note.txt", "text/plain", 5, strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if message.Body != "note" || repo.attachment.StorageID == "" || repo.attachment.StorageID == "note.txt" {
		t.Fatalf("unexpected attachment message: %+v", message)
	}
	attachment, reader, err := svc.OpenAttachment(context.Background(), "u1", "p1", "a1")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	content := new(strings.Builder)
	if _, err := io.Copy(content, reader); err != nil {
		t.Fatal(err)
	}
	if attachment.Filename != "note.txt" || content.String() != "hello" {
		t.Fatalf("attachment=%+v content=%q", attachment, content.String())
	}
	if len(publisher.created) != 1 {
		t.Fatalf("created recipients=%v", publisher.created)
	}
}

func TestSendAttachmentRejectsConfiguredFileLimit(t *testing.T) {
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := &chat.Service{Repo: &fakeRepo{}, Storage: store, MaxUploadBytes: 4, MaxProjectStorageBytes: 100}
	_, err = svc.SendAttachment(context.Background(), "u1", "p1", "", "large.bin", "application/octet-stream", 5, strings.NewReader("12345"))
	var validation *chat.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}
