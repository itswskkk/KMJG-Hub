package notification

import "github.com/itswskkk/KMJG-Hub/server/internal/realtime"

const CreatedEvent = "notification.created"

// RealtimePublisher adapts the shared authenticated connection Hub to the
// narrow Publisher interface used by Service.
type RealtimePublisher struct {
	Hub *realtime.Hub
}

// PublishNotificationCreated delivers the notification only to its owner.
func (p *RealtimePublisher) PublishNotificationCreated(n Notification) {
	env, err := realtime.NewEnvelope(CreatedEvent, map[string]any{
		"id":         n.ID,
		"event_type": n.EventType,
		"payload":    n.Payload,
		"created_at": n.CreatedAt,
		"read_at":    n.ReadAt,
	})
	if err == nil {
		p.Hub.SendToUser(n.UserID, env)
	}
}
