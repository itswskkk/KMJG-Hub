package friend

import (
	"context"
	"log/slog"
	"strings"
)

// Service implements friend request, friendship, and blocking operations.
// Every method takes the acting user's ID and authorizes the action against
// it; handlers never decide who may act on a request.
type Service struct {
	Repo      Repository
	Publisher Publisher // optional; nil disables real-time events
	Notifier  Notifier  // optional; nil disables notifications
}

// Notifier creates persistent notifications. Satisfied by
// *notification.Service; declared here so this package does not import it.
type Notifier interface {
	Notify(ctx context.Context, userID, eventType string, payload any) error
}

// notify creates a notification best-effort: failures are logged and never
// fail the primary operation. A nil Notifier disables notifications.
func (s *Service) notify(ctx context.Context, userID, eventType string, payload any) {
	if s.Notifier == nil {
		return
	}
	if err := s.Notifier.Notify(ctx, userID, eventType, payload); err != nil {
		slog.Warn("friend: create notification", "event_type", eventType, "error", err)
	}
}

// ResolveUser returns the user ID for a username or email, or ErrNotFound.
func (s *Service) ResolveUser(ctx context.Context, field, identifier string) (string, error) {
	identifier = strings.TrimSpace(identifier)
	if err := validateUserID(field, identifier); err != nil {
		return "", err
	}
	return s.Repo.ResolveUser(ctx, identifier)
}

// SendRequest creates a friend request from senderID to recipientID. It
// rejects self-requests, requests between users where either has blocked the
// other, requests between existing friends, and duplicate pending requests.
func (s *Service) SendRequest(ctx context.Context, senderID, recipientID string) (*FriendRequest, error) {
	if err := validateSendRequest(senderID, recipientID); err != nil {
		return nil, err
	}
	req, err := s.Repo.SendRequest(ctx, senderID, recipientID)
	if err != nil {
		return nil, err
	}
	if s.Publisher != nil {
		s.Publisher.PublishRequestSent(*req)
	}
	s.notify(ctx, req.RecipientID, "friend_request", map[string]any{
		"sender_id":       req.SenderID,
		"sender_username": req.SenderUsername,
		"request_id":      req.ID,
	})
	return req, nil
}

// GetRequest returns a friend request visible to userID (its sender or
// recipient). Anyone else gets ErrNotFound so request IDs don't leak.
func (s *Service) GetRequest(ctx context.Context, userID, requestID string) (*FriendRequest, error) {
	req, err := s.Repo.GetRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if req.SenderID != userID && req.RecipientID != userID {
		return nil, ErrNotFound
	}
	return req, nil
}

// ListIncomingRequests returns userID's pending received requests.
func (s *Service) ListIncomingRequests(ctx context.Context, userID string) ([]FriendRequest, error) {
	return s.Repo.ListIncomingRequests(ctx, userID)
}

// ListOutgoingRequests returns userID's pending sent requests.
func (s *Service) ListOutgoingRequests(ctx context.Context, userID string) ([]FriendRequest, error) {
	return s.Repo.ListOutgoingRequests(ctx, userID)
}

// AcceptRequest accepts a pending request. Only the recipient may accept.
func (s *Service) AcceptRequest(ctx context.Context, userID, requestID string) (*FriendRequest, error) {
	if err := s.authorizeRecipient(ctx, userID, requestID); err != nil {
		return nil, err
	}
	req, err := s.Repo.AcceptRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if s.Publisher != nil {
		s.Publisher.PublishRequestAccepted(*req)
	}
	return req, nil
}

// DeclineRequest declines a pending request. Only the recipient may decline.
func (s *Service) DeclineRequest(ctx context.Context, userID, requestID string) (*FriendRequest, error) {
	if err := s.authorizeRecipient(ctx, userID, requestID); err != nil {
		return nil, err
	}
	req, err := s.Repo.DeclineRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if s.Publisher != nil {
		s.Publisher.PublishRequestDeclined(*req)
	}
	return req, nil
}

// CancelRequest withdraws a pending request. Only the sender may cancel.
func (s *Service) CancelRequest(ctx context.Context, userID, requestID string) error {
	req, err := s.GetRequest(ctx, userID, requestID)
	if err != nil {
		return err
	}
	if req.SenderID != userID {
		return ErrForbidden
	}
	if req.Status != StatusPending {
		return ErrNotPending
	}
	if err := s.Repo.CancelRequest(ctx, requestID); err != nil {
		return err
	}
	if s.Publisher != nil {
		s.Publisher.PublishRequestCancelled(*req)
	}
	return nil
}

func (s *Service) authorizeRecipient(ctx context.Context, userID, requestID string) error {
	req, err := s.GetRequest(ctx, userID, requestID)
	if err != nil {
		return err
	}
	if req.RecipientID != userID {
		return ErrForbidden
	}
	if req.Status != StatusPending {
		return ErrNotPending
	}
	return nil
}

// ListFriends returns userID's friends.
func (s *Service) ListFriends(ctx context.Context, userID string) ([]Friend, error) {
	return s.Repo.ListFriends(ctx, userID)
}

// AreFriends reports whether the two users are friends.
func (s *Service) AreFriends(ctx context.Context, userID1, userID2 string) (bool, error) {
	return s.Repo.AreFriends(ctx, userID1, userID2)
}

// RemoveFriendship ends userID's friendship with friendID. Either party may
// remove it; ErrNotFound if they are not friends.
func (s *Service) RemoveFriendship(ctx context.Context, userID, friendID string) error {
	if err := validateUserID("user_id", friendID); err != nil {
		return err
	}
	if userID == friendID {
		return ErrNotFound
	}
	if err := s.Repo.RemoveFriendship(ctx, userID, friendID); err != nil {
		return err
	}
	if s.Publisher != nil {
		s.Publisher.PublishFriendRemoved(userID, friendID)
	}
	return nil
}

// Block blocks blockedID on behalf of blockerID, removing any friendship and
// pending requests between them (docs/PRD.md § Blocking). Blocking an
// already-blocked user is a no-op that returns the existing block.
func (s *Service) Block(ctx context.Context, blockerID, blockedID string) (*Block, error) {
	if err := validateBlock(blockerID, blockedID); err != nil {
		return nil, err
	}
	b, err := s.Repo.Block(ctx, blockerID, blockedID)
	if err != nil {
		return nil, err
	}
	if s.Publisher != nil {
		s.Publisher.PublishBlocked(blockerID, blockedID)
	}
	return b, nil
}

// Unblock removes blockerID's block of blockedID. It does not restore any
// previous friendship (docs/PRD.md § Blocking).
func (s *Service) Unblock(ctx context.Context, blockerID, blockedID string) error {
	if err := validateUserID("user_id", blockedID); err != nil {
		return err
	}
	if err := s.Repo.Unblock(ctx, blockerID, blockedID); err != nil {
		return err
	}
	if s.Publisher != nil {
		s.Publisher.PublishUnblocked(blockerID, blockedID)
	}
	return nil
}

// IsBlocked reports whether blockerID has blocked blockedID.
func (s *Service) IsBlocked(ctx context.Context, blockerID, blockedID string) (bool, error) {
	return s.Repo.IsBlocked(ctx, blockerID, blockedID)
}

// EitherBlocked reports whether either user has blocked the other, the
// condition that disables personal communication between them.
func (s *Service) EitherBlocked(ctx context.Context, userID1, userID2 string) (bool, error) {
	a, err := s.Repo.IsBlocked(ctx, userID1, userID2)
	if err != nil || a {
		return a, err
	}
	return s.Repo.IsBlocked(ctx, userID2, userID1)
}

// ListBlocked returns the users blockerID has blocked.
func (s *Service) ListBlocked(ctx context.Context, blockerID string) ([]Block, error) {
	return s.Repo.ListBlocked(ctx, blockerID)
}
