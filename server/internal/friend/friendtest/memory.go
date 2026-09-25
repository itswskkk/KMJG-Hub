// Package friendtest provides an in-memory friend.Repository for tests in
// any package (service tests and HTTP handler tests), mirroring the
// semantics of internal/store/postgres.FriendRepository.
package friendtest

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/friend"
)

// UserDirectory is the minimal user lookup the fake needs.
type UserDirectory interface {
	// Username returns the username for id, or ok=false if no such user.
	Username(id string) (string, bool)
	// Resolve returns the user ID for a username or email, or ok=false.
	Resolve(identifier string) (string, bool)
}

// StaticUsers is a UserDirectory backed by a fixed id->username map.
type StaticUsers map[string]string

func (u StaticUsers) Username(id string) (string, bool) {
	name, ok := u[id]
	return name, ok
}

func (u StaticUsers) Resolve(identifier string) (string, bool) {
	for id, name := range u {
		if strings.EqualFold(name, identifier) {
			return id, true
		}
	}
	return "", false
}

type pair struct{ a, b string }

func pairOf(x, y string) pair {
	if x < y {
		return pair{x, y}
	}
	return pair{y, x}
}

type Memory struct {
	mu       sync.Mutex
	users    UserDirectory
	nextID   int
	requests map[string]*friend.FriendRequest
	friends  map[pair]time.Time
	blocks   map[pair]*friend.Block // keyed (blocker, blocked), not canonical
}

func NewMemory(users UserDirectory) *Memory {
	return &Memory{
		users:    users,
		requests: make(map[string]*friend.FriendRequest),
		friends:  make(map[pair]time.Time),
		blocks:   make(map[pair]*friend.Block),
	}
}

var _ friend.Repository = (*Memory)(nil)

func (m *Memory) id(prefix string) string {
	m.nextID++
	return fmt.Sprintf("%s-%d", prefix, m.nextID)
}

func (m *Memory) eitherBlocked(x, y string) bool {
	return m.blocks[pair{x, y}] != nil || m.blocks[pair{y, x}] != nil
}

func (m *Memory) pendingBetween(x, y string) bool {
	for _, r := range m.requests {
		if r.Status == friend.StatusPending &&
			((r.SenderID == x && r.RecipientID == y) || (r.SenderID == y && r.RecipientID == x)) {
			return true
		}
	}
	return false
}

func (m *Memory) ResolveUser(_ context.Context, identifier string) (string, error) {
	if id, ok := m.users.Resolve(identifier); ok {
		return id, nil
	}
	return "", friend.ErrNotFound
}

func (m *Memory) SendRequest(_ context.Context, senderID, recipientID string) (*friend.FriendRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	senderName, ok1 := m.users.Username(senderID)
	recipientName, ok2 := m.users.Username(recipientID)
	if !ok1 || !ok2 {
		return nil, friend.ErrNotFound
	}
	switch {
	case m.eitherBlocked(senderID, recipientID):
		return nil, friend.ErrBlocked
	case !m.friends[pairOf(senderID, recipientID)].IsZero():
		return nil, friend.ErrAlreadyFriends
	case m.pendingBetween(senderID, recipientID):
		return nil, friend.ErrRequestPending
	}
	// Mirror the unique (sender, recipient) constraint: re-open an answered
	// request rather than creating a second row.
	for _, r := range m.requests {
		if r.SenderID == senderID && r.RecipientID == recipientID {
			r.Status, r.CreatedAt, r.RespondedAt = friend.StatusPending, time.Now().UTC(), nil
			copy := *r
			return &copy, nil
		}
	}
	req := &friend.FriendRequest{
		ID: m.id("freq"), SenderID: senderID, SenderUsername: senderName,
		RecipientID: recipientID, RecipientUsername: recipientName,
		Status: friend.StatusPending, CreatedAt: time.Now().UTC(),
	}
	m.requests[req.ID] = req
	copy := *req
	return &copy, nil
}

func (m *Memory) GetRequest(_ context.Context, requestID string) (*friend.FriendRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.requests[requestID]
	if !ok {
		return nil, friend.ErrNotFound
	}
	copy := *r
	return &copy, nil
}

func (m *Memory) listRequests(match func(*friend.FriendRequest) bool) []friend.FriendRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []friend.FriendRequest{}
	for _, r := range m.requests {
		if r.Status == friend.StatusPending && match(r) {
			out = append(out, *r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (m *Memory) ListIncomingRequests(_ context.Context, userID string) ([]friend.FriendRequest, error) {
	return m.listRequests(func(r *friend.FriendRequest) bool { return r.RecipientID == userID }), nil
}

func (m *Memory) ListOutgoingRequests(_ context.Context, userID string) ([]friend.FriendRequest, error) {
	return m.listRequests(func(r *friend.FriendRequest) bool { return r.SenderID == userID }), nil
}

func (m *Memory) AcceptRequest(_ context.Context, requestID string) (*friend.FriendRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.requests[requestID]
	if !ok {
		return nil, friend.ErrNotFound
	}
	if m.eitherBlocked(r.SenderID, r.RecipientID) {
		return nil, friend.ErrBlocked
	}
	if r.Status != friend.StatusPending {
		return nil, friend.ErrNotPending
	}
	now := time.Now().UTC()
	r.Status, r.RespondedAt = friend.StatusAccepted, &now
	if m.friends[pairOf(r.SenderID, r.RecipientID)].IsZero() {
		m.friends[pairOf(r.SenderID, r.RecipientID)] = now
	}
	copy := *r
	return &copy, nil
}

func (m *Memory) DeclineRequest(_ context.Context, requestID string) (*friend.FriendRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.requests[requestID]
	if !ok {
		return nil, friend.ErrNotFound
	}
	if r.Status != friend.StatusPending {
		return nil, friend.ErrNotPending
	}
	now := time.Now().UTC()
	r.Status, r.RespondedAt = friend.StatusDeclined, &now
	copy := *r
	return &copy, nil
}

func (m *Memory) CancelRequest(_ context.Context, requestID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.requests[requestID]
	if !ok {
		return friend.ErrNotFound
	}
	if r.Status != friend.StatusPending {
		return friend.ErrNotPending
	}
	delete(m.requests, requestID)
	return nil
}

func (m *Memory) AreFriends(_ context.Context, userID1, userID2 string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return !m.friends[pairOf(userID1, userID2)].IsZero(), nil
}

func (m *Memory) ListFriends(_ context.Context, userID string) ([]friend.Friend, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []friend.Friend{}
	for p, since := range m.friends {
		other := ""
		switch userID {
		case p.a:
			other = p.b
		case p.b:
			other = p.a
		default:
			continue
		}
		name, _ := m.users.Username(other)
		out = append(out, friend.Friend{UserID: other, Username: name, Since: since})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out, nil
}

func (m *Memory) RemoveFriendship(_ context.Context, userID1, userID2 string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := pairOf(userID1, userID2)
	if m.friends[p].IsZero() {
		return friend.ErrNotFound
	}
	delete(m.friends, p)
	return nil
}

func (m *Memory) Block(_ context.Context, blockerID, blockedID string) (*friend.Block, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	name, ok := m.users.Username(blockedID)
	if !ok {
		return nil, friend.ErrNotFound
	}
	key := pair{blockerID, blockedID}
	if m.blocks[key] == nil {
		m.blocks[key] = &friend.Block{ID: m.id("block"), BlockerID: blockerID, BlockedID: blockedID, BlockedUsername: name, Since: time.Now().UTC()}
	}
	delete(m.friends, pairOf(blockerID, blockedID))
	for id, r := range m.requests {
		if r.Status == friend.StatusPending &&
			((r.SenderID == blockerID && r.RecipientID == blockedID) || (r.SenderID == blockedID && r.RecipientID == blockerID)) {
			delete(m.requests, id)
		}
	}
	copy := *m.blocks[key]
	return &copy, nil
}

func (m *Memory) Unblock(_ context.Context, blockerID, blockedID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := pair{blockerID, blockedID}
	if m.blocks[key] == nil {
		return friend.ErrNotFound
	}
	delete(m.blocks, key)
	return nil
}

func (m *Memory) IsBlocked(_ context.Context, blockerID, blockedID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.blocks[pair{blockerID, blockedID}] != nil, nil
}

func (m *Memory) ListBlocked(_ context.Context, blockerID string) ([]friend.Block, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []friend.Block{}
	for key, b := range m.blocks {
		if key.a == blockerID {
			out = append(out, *b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BlockedUsername < out[j].BlockedUsername })
	return out, nil
}
