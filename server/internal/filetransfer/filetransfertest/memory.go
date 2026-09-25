// Package filetransfertest provides in-memory fakes for filetransfer tests,
// mirroring the rules internal/store/postgres.FileTransferRepository
// enforces in SQL.
package filetransfertest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/filetransfer"
	"github.com/itswskkk/KMJG-Hub/server/internal/storage"
)

// Memory is an in-memory filetransfer.Repository.
type Memory struct {
	mu        sync.Mutex
	nextID    int
	transfers []*filetransfer.Transfer

	// Username resolves a user ID to a username; ok=false means the user
	// does not exist (ErrNotFound). Required.
	Username func(id string) (string, bool)
	// Permitted reports whether the pair may exchange files (friends or a
	// shared Project, neither blocked). Nil permits every pair.
	Permitted func(senderID, recipientID string) bool
}

var _ filetransfer.Repository = (*Memory)(nil)

func (m *Memory) Create(_ context.Context, senderID, recipientID, fileName string, declaredSize int64) (*filetransfer.Transfer, error) {
	senderName, ok1 := m.Username(senderID)
	recipientName, ok2 := m.Username(recipientID)
	if !ok1 || !ok2 {
		return nil, filetransfer.ErrNotFound
	}
	if senderID == recipientID || (m.Permitted != nil && !m.Permitted(senderID, recipientID)) {
		return nil, filetransfer.ErrForbidden
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	t := &filetransfer.Transfer{
		ID: fmt.Sprintf("transfer-%d", m.nextID), SenderID: senderID, SenderUsername: senderName,
		RecipientID: recipientID, RecipientUsername: recipientName, FileName: fileName,
		DeclaredFileSize: declaredSize, Status: filetransfer.StatusPending,
		CreatedAt: time.Date(2026, 1, 1, 0, 0, m.nextID, 0, time.UTC),
	}
	m.transfers = append(m.transfers, t)
	copy := *t
	return &copy, nil
}

func (m *Memory) find(transferID, userID string) *filetransfer.Transfer {
	for _, t := range m.transfers {
		if t.ID == transferID && (t.SenderID == userID || t.RecipientID == userID) {
			return t
		}
	}
	return nil
}

func (m *Memory) Get(_ context.Context, transferID, userID string) (*filetransfer.Transfer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.find(transferID, userID)
	if t == nil {
		return nil, filetransfer.ErrNotFound
	}
	copy := *t
	return &copy, nil
}

func (m *Memory) list(match func(*filetransfer.Transfer) bool) []filetransfer.Transfer {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]filetransfer.Transfer, 0)
	for _, t := range m.transfers {
		if match(t) {
			out = append(out, *t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (m *Memory) ListIncoming(_ context.Context, userID string) ([]filetransfer.Transfer, error) {
	return m.list(func(t *filetransfer.Transfer) bool { return t.RecipientID == userID }), nil
}

func (m *Memory) ListSent(_ context.Context, userID string) ([]filetransfer.Transfer, error) {
	return m.list(func(t *filetransfer.Transfer) bool { return t.SenderID == userID }), nil
}

// transition mirrors the SQL guarded UPDATE plus its not_found / forbidden
// / invalid_state classification.
func (m *Memory) transition(transferID, actorID string, actorIsSender bool, from []filetransfer.Status, apply func(*filetransfer.Transfer)) (*filetransfer.Transfer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.find(transferID, actorID)
	if t == nil {
		return nil, filetransfer.ErrNotFound
	}
	if (actorIsSender && t.SenderID != actorID) || (!actorIsSender && t.RecipientID != actorID) {
		return nil, filetransfer.ErrForbidden
	}
	allowed := false
	for _, s := range from {
		allowed = allowed || t.Status == s
	}
	if !allowed {
		return nil, filetransfer.ErrInvalidState
	}
	apply(t)
	copy := *t
	return &copy, nil
}

func now() *time.Time { n := time.Now().UTC(); return &n }

func (m *Memory) Accept(_ context.Context, transferID, recipientID string) (*filetransfer.Transfer, error) {
	return m.transition(transferID, recipientID, false, []filetransfer.Status{filetransfer.StatusPending}, func(t *filetransfer.Transfer) {
		t.Status, t.RespondedAt = filetransfer.StatusAccepted, now()
	})
}

func (m *Memory) Decline(_ context.Context, transferID, recipientID string) (*filetransfer.Transfer, error) {
	return m.transition(transferID, recipientID, false, []filetransfer.Status{filetransfer.StatusPending}, func(t *filetransfer.Transfer) {
		t.Status, t.RespondedAt = filetransfer.StatusDeclined, now()
	})
}

func (m *Memory) Cancel(_ context.Context, transferID, senderID string) (*filetransfer.Transfer, error) {
	return m.transition(transferID, senderID, true, []filetransfer.Status{filetransfer.StatusPending, filetransfer.StatusAccepted}, func(t *filetransfer.Transfer) {
		t.Status = filetransfer.StatusCancelled
		if t.RespondedAt == nil {
			t.RespondedAt = now()
		}
	})
}

func (m *Memory) MarkUploaded(_ context.Context, transferID, senderID, storageID string, actualSize int64, contentType string) (*filetransfer.Transfer, error) {
	return m.transition(transferID, senderID, true, []filetransfer.Status{filetransfer.StatusAccepted}, func(t *filetransfer.Transfer) {
		t.Status, t.StorageID, t.ActualFileSize, t.ContentType, t.UploadedAt = filetransfer.StatusUploaded, &storageID, &actualSize, &contentType, now()
	})
}

// Store is an in-memory filetransfer.FileStore.
type Store struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func NewStore() *Store { return &Store{objects: map[string][]byte{}} }

func (s *Store) Put(_ context.Context, id string, src io.Reader, size int64) error {
	data, err := io.ReadAll(io.LimitReader(src, size+1))
	if err != nil {
		return err
	}
	if int64(len(data)) != size {
		return fmt.Errorf("storage object size mismatch")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.objects[id]; exists {
		return fmt.Errorf("storage object already exists")
	}
	s.objects[id] = data
	return nil
}

func (s *Store) Open(_ context.Context, id string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.objects[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *Store) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.objects[id]; !ok {
		return storage.ErrNotFound
	}
	delete(s.objects, id)
	return nil
}

// Exists satisfies storage.Store too, so the fake can back Project Chat.
func (s *Store) Exists(_ context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.objects[id]
	return ok, nil
}

// Len reports how many objects are stored.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.objects)
}
