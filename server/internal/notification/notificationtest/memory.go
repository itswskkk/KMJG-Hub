// Package notificationtest provides an in-memory notification.Repository for
// tests in any package, mirroring internal/store/postgres.NotificationRepository.
package notificationtest

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/notification"
)

// Memory is a concurrency-safe in-memory notification.Repository. Each
// created notification gets a strictly increasing CreatedAt so ordering is
// deterministic.
type Memory struct {
	mu     sync.Mutex
	nextID int
	items  []notification.Notification
	// FailCreate, when set, makes Create return this error.
	FailCreate error
}

func NewMemory() *Memory { return &Memory{} }

func (m *Memory) Create(_ context.Context, userID string, eventType notification.EventType, payload any) (*notification.Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.FailCreate != nil {
		return nil, m.FailCreate
	}
	if payload == nil {
		payload = map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	m.nextID++
	n := notification.Notification{
		ID:        fmt.Sprintf("n-%04d", m.nextID),
		UserID:    userID,
		EventType: eventType,
		Payload:   raw,
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(m.nextID) * time.Second),
	}
	m.items = append(m.items, n)
	return &n, nil
}

func (m *Memory) ListPage(_ context.Context, userID string, before *notification.Cursor, limit int) ([]notification.Notification, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	unread := 0
	matching := []notification.Notification{}
	for _, n := range m.items {
		if n.UserID != userID {
			continue
		}
		if n.ReadAt == nil {
			unread++
		}
		if before != nil && !(n.CreatedAt.Before(before.CreatedAt) || (n.CreatedAt.Equal(before.CreatedAt) && n.ID < before.ID)) {
			continue
		}
		matching = append(matching, n)
	}
	sort.Slice(matching, func(i, j int) bool {
		if !matching[i].CreatedAt.Equal(matching[j].CreatedAt) {
			return matching[i].CreatedAt.After(matching[j].CreatedAt)
		}
		return matching[i].ID > matching[j].ID
	})
	if len(matching) > limit {
		matching = matching[:limit]
	}
	return matching, unread, nil
}

func (m *Memory) MarkRead(_ context.Context, notificationID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.items {
		if m.items[i].ID == notificationID && m.items[i].UserID == userID {
			if m.items[i].ReadAt == nil {
				now := time.Now().UTC()
				m.items[i].ReadAt = &now
			}
			return nil
		}
	}
	return notification.ErrNotFound
}

func (m *Memory) Delete(_ context.Context, notificationID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.items {
		if m.items[i].ID == notificationID && m.items[i].UserID == userID {
			m.items = append(m.items[:i], m.items[i+1:]...)
			return nil
		}
	}
	return notification.ErrNotFound
}

// ForUser returns a snapshot of userID's notifications in creation order.
func (m *Memory) ForUser(userID string) []notification.Notification {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []notification.Notification{}
	for _, n := range m.items {
		if n.UserID == userID {
			out = append(out, n)
		}
	}
	return out
}
