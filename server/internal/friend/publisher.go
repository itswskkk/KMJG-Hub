package friend

import (
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/realtime"
)

const (
	RequestSentEvent      = "friend_request.sent"
	RequestAcceptedEvent  = "friend_request.accepted"
	RequestDeclinedEvent  = "friend_request.declined"
	RequestCancelledEvent = "friend_request.cancelled"
	FriendRemovedEvent    = "friendship.removed"
	UserBlockedEvent      = "user.blocked"
	UserUnblockedEvent    = "user.unblocked"
)

// RealtimePublisher adapts the shared authenticated connection Hub to
// Publisher. Events are hints for Clients to refresh; HTTP stays
// authoritative. Relationship events go to both parties so every open
// session of each stays in sync (docs/PRD.md § Blocking allows the blocked
// user to be informed).
type RealtimePublisher struct {
	Hub *realtime.Hub
}

func (p *RealtimePublisher) send(eventType string, data any, userIDs ...string) {
	env, err := realtime.NewEnvelope(eventType, data)
	if err != nil {
		return
	}
	for _, id := range userIDs {
		p.Hub.SendToUser(id, env)
	}
}

func (p *RealtimePublisher) PublishRequestSent(req FriendRequest) {
	p.send(RequestSentEvent, requestEvent(req), req.RecipientID, req.SenderID)
}

func (p *RealtimePublisher) PublishRequestAccepted(req FriendRequest) {
	p.send(RequestAcceptedEvent, requestEvent(req), req.SenderID, req.RecipientID)
}

func (p *RealtimePublisher) PublishRequestDeclined(req FriendRequest) {
	p.send(RequestDeclinedEvent, requestEvent(req), req.SenderID, req.RecipientID)
}

func (p *RealtimePublisher) PublishRequestCancelled(req FriendRequest) {
	p.send(RequestCancelledEvent, requestEvent(req), req.RecipientID, req.SenderID)
}

func (p *RealtimePublisher) PublishFriendRemoved(userID, friendID string) {
	p.send(FriendRemovedEvent, map[string]string{"user_id": userID, "friend_id": friendID}, userID, friendID)
}

func (p *RealtimePublisher) PublishBlocked(blockerID, blockedID string) {
	p.send(UserBlockedEvent, map[string]string{"blocker_id": blockerID, "blocked_id": blockedID}, blockerID, blockedID)
}

func (p *RealtimePublisher) PublishUnblocked(blockerID, blockedID string) {
	p.send(UserUnblockedEvent, map[string]string{"blocker_id": blockerID, "blocked_id": blockedID}, blockerID, blockedID)
}

func requestEvent(req FriendRequest) map[string]any {
	var respondedAt *time.Time
	if req.RespondedAt != nil {
		t := *req.RespondedAt
		respondedAt = &t
	}
	return map[string]any{
		"id":                 req.ID,
		"sender_id":          req.SenderID,
		"sender_username":    req.SenderUsername,
		"recipient_id":       req.RecipientID,
		"recipient_username": req.RecipientUsername,
		"status":             req.Status,
		"created_at":         req.CreatedAt,
		"responded_at":       respondedAt,
	}
}
