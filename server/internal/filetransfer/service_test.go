package filetransfer_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/itswskkk/KMJG-Hub/server/internal/filetransfer"
	"github.com/itswskkk/KMJG-Hub/server/internal/filetransfer/filetransfertest"
)

type recordingPublisher struct {
	mu     sync.Mutex
	events []string
}

func (p *recordingPublisher) record(kind string, t filetransfer.Transfer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, kind+":"+string(t.Status))
}
func (p *recordingPublisher) PublishTransferRequested(t filetransfer.Transfer) {
	p.record("requested", t)
}
func (p *recordingPublisher) PublishTransferResponded(t filetransfer.Transfer) {
	p.record("responded", t)
}
func (p *recordingPublisher) PublishTransferCancelled(t filetransfer.Transfer) {
	p.record("cancelled", t)
}
func (p *recordingPublisher) PublishTransferUploaded(t filetransfer.Transfer) {
	p.record("uploaded", t)
}

type recordingNotifier struct {
	userIDs    []string
	eventTypes []string
}

func (n *recordingNotifier) Notify(_ context.Context, userID, eventType string, _ any) error {
	n.userIDs = append(n.userIDs, userID)
	n.eventTypes = append(n.eventTypes, eventType)
	return nil
}

type fixture struct {
	svc       *filetransfer.Service
	store     *filetransfertest.Store
	publisher *recordingPublisher
	notifier  *recordingNotifier
	blocked   map[string]bool
}

func newFixture() *fixture {
	f := &fixture{store: filetransfertest.NewStore(), publisher: &recordingPublisher{}, notifier: &recordingNotifier{}, blocked: map[string]bool{}}
	users := map[string]string{"alice": "Alice", "bob": "Bob", "eve": "Eve"}
	repo := &filetransfertest.Memory{
		Username:  func(id string) (string, bool) { name, ok := users[id]; return name, ok },
		Permitted: func(a, b string) bool { return !f.blocked[a+">"+b] && !f.blocked[b+">"+a] },
	}
	f.svc = &filetransfer.Service{Repo: repo, Storage: f.store, Publisher: f.publisher, Notifier: f.notifier, MaxUploadBytes: 100}
	return f
}

func (f *fixture) request(t *testing.T, size int64) *filetransfer.Transfer {
	t.Helper()
	tr, err := f.svc.CreateRequest(context.Background(), "alice", "bob", "notes.txt", size)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return tr
}

func TestCreateRequestValidatesAndNotifies(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	var validation *filetransfer.ValidationError
	for _, name := range []string{"", "  ", "../etc/passwd", "a/b.txt", `a\b.txt`, "..", "bad\x00name", strings.Repeat("x", 256)} {
		if _, err := f.svc.CreateRequest(ctx, "alice", "bob", name, 5); !errors.As(err, &validation) {
			t.Errorf("name %q: expected validation error, got %v", name, err)
		}
	}
	if _, err := f.svc.CreateRequest(ctx, "alice", "bob", "a.txt", 0); !errors.As(err, &validation) {
		t.Errorf("zero size: %v", err)
	}
	if _, err := f.svc.CreateRequest(ctx, "alice", "bob", "a.txt", 101); !errors.Is(err, filetransfer.ErrSizeLimitExceeded) {
		t.Errorf("over limit: %v", err)
	}
	if _, err := f.svc.CreateRequest(ctx, "alice", "alice", "a.txt", 5); !errors.As(err, &validation) {
		t.Errorf("self: %v", err)
	}
	if _, err := f.svc.CreateRequest(ctx, "alice", "ghost", "a.txt", 5); !errors.Is(err, filetransfer.ErrNotFound) {
		t.Errorf("unknown recipient: %v", err)
	}
	f.blocked["bob>alice"] = true
	if _, err := f.svc.CreateRequest(ctx, "alice", "bob", "a.txt", 5); !errors.Is(err, filetransfer.ErrForbidden) {
		t.Errorf("blocked: %v", err)
	}
	delete(f.blocked, "bob>alice")

	tr := f.request(t, 5)
	if tr.Status != filetransfer.StatusPending || tr.SenderUsername != "Alice" || tr.RecipientUsername != "Bob" {
		t.Fatalf("created: %+v", tr)
	}
	if len(f.notifier.userIDs) != 1 || f.notifier.userIDs[0] != "bob" || f.notifier.eventTypes[0] != "file_transfer_request" {
		t.Fatalf("notifications: %+v %+v", f.notifier.userIDs, f.notifier.eventTypes)
	}
	if f.store.Len() != 0 {
		t.Fatal("requesting a transfer must not store any bytes")
	}
}

func TestAcceptDeclineStateMachine(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	tr := f.request(t, 5)

	if _, err := f.svc.Accept(ctx, tr.ID, "alice"); !errors.Is(err, filetransfer.ErrForbidden) {
		t.Fatalf("sender accept: %v", err)
	}
	if _, err := f.svc.Accept(ctx, tr.ID, "eve"); !errors.Is(err, filetransfer.ErrNotFound) {
		t.Fatalf("outsider accept: %v", err)
	}
	accepted, err := f.svc.Accept(ctx, tr.ID, "bob")
	if err != nil || accepted.Status != filetransfer.StatusAccepted {
		t.Fatalf("accept: %v %+v", err, accepted)
	}
	if _, err := f.svc.Accept(ctx, tr.ID, "bob"); !errors.Is(err, filetransfer.ErrInvalidState) {
		t.Fatalf("double accept: %v", err)
	}
	if _, err := f.svc.Decline(ctx, tr.ID, "bob"); !errors.Is(err, filetransfer.ErrInvalidState) {
		t.Fatalf("decline after accept: %v", err)
	}

	other := f.request(t, 5)
	if _, err := f.svc.Decline(ctx, other.ID, "alice"); !errors.Is(err, filetransfer.ErrForbidden) {
		t.Fatalf("sender decline: %v", err)
	}
	declined, err := f.svc.Decline(ctx, other.ID, "bob")
	if err != nil || declined.Status != filetransfer.StatusDeclined {
		t.Fatalf("decline: %v", err)
	}
	if _, err := f.svc.Upload(ctx, other.ID, "alice", strings.NewReader("hello"), 5, ""); !errors.Is(err, filetransfer.ErrInvalidState) {
		t.Fatalf("upload after decline: %v", err)
	}
	if f.store.Len() != 0 {
		t.Fatal("a declined transfer must not store any bytes")
	}
}

func TestCancelOnlyBySenderWhileActive(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	tr := f.request(t, 5)
	if _, err := f.svc.Cancel(ctx, tr.ID, "bob"); !errors.Is(err, filetransfer.ErrForbidden) {
		t.Fatalf("recipient cancel: %v", err)
	}
	if _, err := f.svc.Accept(ctx, tr.ID, "bob"); err != nil {
		t.Fatal(err)
	}
	cancelled, err := f.svc.Cancel(ctx, tr.ID, "alice")
	if err != nil || cancelled.Status != filetransfer.StatusCancelled {
		t.Fatalf("cancel accepted transfer: %v", err)
	}
	if _, err := f.svc.Cancel(ctx, tr.ID, "alice"); !errors.Is(err, filetransfer.ErrInvalidState) {
		t.Fatalf("double cancel: %v", err)
	}
	if _, err := f.svc.Upload(ctx, tr.ID, "alice", strings.NewReader("hello"), 5, ""); !errors.Is(err, filetransfer.ErrInvalidState) {
		t.Fatalf("upload after cancel: %v", err)
	}
}

func TestUploadAndDownload(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	tr := f.request(t, 5)

	if _, err := f.svc.Upload(ctx, tr.ID, "alice", strings.NewReader("hello"), 5, ""); !errors.Is(err, filetransfer.ErrInvalidState) {
		t.Fatalf("upload before accept: %v", err)
	}
	if _, err := f.svc.Accept(ctx, tr.ID, "bob"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Upload(ctx, tr.ID, "bob", strings.NewReader("hello"), 5, ""); !errors.Is(err, filetransfer.ErrForbidden) {
		t.Fatalf("recipient upload: %v", err)
	}
	if _, err := f.svc.Upload(ctx, tr.ID, "alice", strings.NewReader("hello!"), 6, ""); !errors.Is(err, filetransfer.ErrSizeMismatch) {
		t.Fatalf("size mismatch: %v", err)
	}
	if _, _, err := f.svc.Download(ctx, tr.ID, "bob"); !errors.Is(err, filetransfer.ErrInvalidState) {
		t.Fatalf("download before upload: %v", err)
	}
	if f.store.Len() != 0 {
		t.Fatal("rejected uploads must not store bytes")
	}

	uploaded, err := f.svc.Upload(ctx, tr.ID, "alice", strings.NewReader("hello"), 5, "")
	if err != nil || uploaded.Status != filetransfer.StatusUploaded || *uploaded.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("upload: %v %+v", err, uploaded)
	}
	if _, err := f.svc.Upload(ctx, tr.ID, "alice", strings.NewReader("hello"), 5, ""); !errors.Is(err, filetransfer.ErrInvalidState) {
		t.Fatalf("second upload: %v", err)
	}
	if f.store.Len() != 1 {
		t.Fatalf("stored objects: %d", f.store.Len())
	}

	if _, _, err := f.svc.Download(ctx, tr.ID, "alice"); !errors.Is(err, filetransfer.ErrForbidden) {
		t.Fatalf("sender download: %v", err)
	}
	if _, _, err := f.svc.Download(ctx, tr.ID, "eve"); !errors.Is(err, filetransfer.ErrNotFound) {
		t.Fatalf("outsider download: %v", err)
	}
	reader, got, err := f.svc.Download(ctx, tr.ID, "bob")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, _ := io.ReadAll(reader)
	if string(data) != "hello" || got.FileName != "notes.txt" {
		t.Fatalf("download: %q %+v", data, got)
	}

	want := []string{"requested:pending", "responded:accepted", "uploaded:uploaded"}
	if strings.Join(f.publisher.events, ",") != strings.Join(want, ",") {
		t.Fatalf("events: %v", f.publisher.events)
	}
}

func TestUploadRespectsMaxUploadBytes(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	tr := f.request(t, 50)
	if _, err := f.svc.Accept(ctx, tr.ID, "bob"); err != nil {
		t.Fatal(err)
	}
	f.svc.MaxUploadBytes = 10 // administrator lowered the limit after the request
	if _, err := f.svc.Upload(ctx, tr.ID, "alice", strings.NewReader(strings.Repeat("x", 50)), 50, ""); !errors.Is(err, filetransfer.ErrSizeLimitExceeded) {
		t.Fatalf("upload over limit: %v", err)
	}
}

func TestListIncomingAndSent(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	first := f.request(t, 5)
	second := f.request(t, 6)
	incoming, _ := f.svc.ListIncoming(ctx, "bob")
	sent, _ := f.svc.ListSent(ctx, "alice")
	if len(incoming) != 2 || incoming[0].ID != second.ID || incoming[1].ID != first.ID {
		t.Fatalf("incoming: %+v", incoming)
	}
	if len(sent) != 2 {
		t.Fatalf("sent: %+v", sent)
	}
	if other, _ := f.svc.ListIncoming(ctx, "alice"); len(other) != 0 {
		t.Fatalf("alice incoming: %+v", other)
	}
}
