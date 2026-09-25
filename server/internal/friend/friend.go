// Package friend implements KMJG Hub friendship and blocking relationships.
// See docs/PRD.md § Friends and Direct Messages, Blocking and
// docs/ARCHITECTURE.md "Blocking".
//
// Friendships and blocks are Server-level personal relationships, stored
// independently from Project membership: blocking never alters Project
// permissions (docs/ARCHITECTURE.md "Blocking must not be treated as a
// Project authorization rule").
package friend

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound       = errors.New("friend: not found")
	ErrForbidden      = errors.New("friend: forbidden")
	ErrAlreadyFriends = errors.New("friend: already friends")
	ErrRequestPending = errors.New("friend: request already pending")
	ErrNotPending     = errors.New("friend: request is no longer pending")
	ErrSelfRequest    = errors.New("friend: cannot request friendship with self")
	ErrSelfBlock      = errors.New("friend: cannot block self")
	ErrBlocked        = errors.New("friend: blocked")
)

// RequestStatus indicates the state of a friend request.
type RequestStatus string

const (
	StatusPending  RequestStatus = "pending"
	StatusAccepted RequestStatus = "accepted"
	StatusDeclined RequestStatus = "declined"
)

// FriendRequest represents a friendship request.
type FriendRequest struct {
	ID                string
	SenderID          string
	SenderUsername    string
	RecipientID       string
	RecipientUsername string
	Status            RequestStatus
	CreatedAt         time.Time
	RespondedAt       *time.Time
}

// Friend is one entry in a user's friends list: the other party of an
// accepted friendship.
type Friend struct {
	UserID   string
	Username string
	Since    time.Time
}

// Friendship represents an accepted friendship between two users. It is
// stored once per pair in canonical order (UserAID < UserBID).
type Friendship struct {
	ID      string
	UserAID string
	UserBID string
	Since   time.Time
}

// Block represents a blocking relationship: BlockerID has blocked BlockedID.
type Block struct {
	ID              string
	BlockerID       string
	BlockedID       string
	BlockedUsername string
	Since           time.Time
}

// ValidationError describes a validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return "friend: validation error on field " + e.Field + ": " + e.Message
}

// Repository persists friend relationships. Implementations must enforce the
// relationship invariants atomically (not only rely on Service's pre-checks),
// since two concurrent requests could otherwise both pass a read-then-write
// check: blocks must be re-checked inside the write that creates a request
// or friendship, and Block must remove any friendship in the same
// transaction that records the block.
type Repository interface {
	// ResolveUser returns the user ID for a username or email
	// (case-insensitive), or ErrNotFound.
	ResolveUser(ctx context.Context, identifier string) (string, error)

	// SendRequest creates (or re-opens a previously answered) friend request
	// from senderID to recipientID. It returns ErrNotFound if recipientID
	// does not exist, ErrBlocked if either user has blocked the other,
	// ErrAlreadyFriends if they are already friends, and ErrRequestPending if
	// a pending request already exists in either direction.
	SendRequest(ctx context.Context, senderID, recipientID string) (*FriendRequest, error)

	// GetRequest returns a specific friend request, or ErrNotFound.
	GetRequest(ctx context.Context, requestID string) (*FriendRequest, error)

	// ListIncomingRequests returns all pending requests to userID, newest first.
	ListIncomingRequests(ctx context.Context, userID string) ([]FriendRequest, error)

	// ListOutgoingRequests returns all pending requests from userID, newest first.
	ListOutgoingRequests(ctx context.Context, userID string) ([]FriendRequest, error)

	// AcceptRequest moves a pending request to accepted and creates the
	// friendship. Returns ErrNotPending if it is no longer pending and
	// ErrBlocked if a block now exists between the two users.
	AcceptRequest(ctx context.Context, requestID string) (*FriendRequest, error)

	// DeclineRequest marks a pending request as declined, or ErrNotPending.
	DeclineRequest(ctx context.Context, requestID string) (*FriendRequest, error)

	// CancelRequest deletes a pending request, or ErrNotPending.
	CancelRequest(ctx context.Context, requestID string) error

	// AreFriends reports whether the two users are friends.
	AreFriends(ctx context.Context, userID1, userID2 string) (bool, error)

	// ListFriends returns all friends of userID, ordered by username.
	ListFriends(ctx context.Context, userID string) ([]Friend, error)

	// RemoveFriendship dissolves a friendship, or ErrNotFound.
	RemoveFriendship(ctx context.Context, userID1, userID2 string) error

	// Block records that blockerID blocked blockedID (idempotently), and in
	// the same transaction removes any friendship and pending friend requests
	// between them. ErrNotFound if blockedID does not exist.
	Block(ctx context.Context, blockerID, blockedID string) (*Block, error)

	// Unblock removes a block, or ErrNotFound.
	Unblock(ctx context.Context, blockerID, blockedID string) error

	// IsBlocked reports whether blockerID has blocked blockedID.
	IsBlocked(ctx context.Context, blockerID, blockedID string) (bool, error)

	// ListBlocked returns all users blocked by blockerID, ordered by username.
	ListBlocked(ctx context.Context, blockerID string) ([]Block, error)
}

// Publisher delivers real-time relationship events. Implementations must not
// block the calling request.
type Publisher interface {
	PublishRequestSent(req FriendRequest)
	PublishRequestAccepted(req FriendRequest)
	PublishRequestDeclined(req FriendRequest)
	PublishRequestCancelled(req FriendRequest)
	PublishFriendRemoved(userID, friendID string)
	PublishBlocked(blockerID, blockedID string)
	PublishUnblocked(blockerID, blockedID string)
}
