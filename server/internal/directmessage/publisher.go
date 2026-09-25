package directmessage

import "github.com/itswskkk/KMJG-Hub/server/internal/realtime"

const (
	MessageCreatedEvent = "direct_message.created"
	MessageDeletedEvent = "direct_message.deleted"
)

// RealtimePublisher adapts the shared authenticated connection Hub to the
// narrow Publisher interface used by Service.
type RealtimePublisher struct {
	Hub *realtime.Hub
}

// PublishMessageCreated notifies only the recipient: the sender already has
// the message from their HTTP response.
func (p *RealtimePublisher) PublishMessageCreated(message Message) {
	env, err := realtime.NewEnvelope(MessageCreatedEvent, messageEvent(message))
	if err == nil {
		p.Hub.SendToUser(message.RecipientID, env)
	}
}

// PublishMessageDeleted notifies both participants, since either may have
// the conversation open (including the sender on another device).
func (p *RealtimePublisher) PublishMessageDeleted(senderID, recipientID, messageID string) {
	env, err := realtime.NewEnvelope(MessageDeletedEvent, map[string]string{
		"message_id":   messageID,
		"sender_id":    senderID,
		"recipient_id": recipientID,
	})
	if err != nil {
		return
	}
	p.Hub.SendToUser(senderID, env)
	p.Hub.SendToUser(recipientID, env)
}

func messageEvent(message Message) map[string]any {
	return map[string]any{
		"id":                 message.ID,
		"sender_id":          message.SenderID,
		"sender_username":    message.SenderUsername,
		"recipient_id":       message.RecipientID,
		"recipient_username": message.RecipientUsername,
		"body":               message.Body,
		"created_at":         message.CreatedAt,
	}
}
