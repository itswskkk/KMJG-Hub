package chat

import "github.com/itswskkk/KMJG-Hub/server/internal/realtime"

const (
	MessageCreatedEvent = "project.message.created"
	MessageDeletedEvent = "project.message.deleted"
)

// RealtimePublisher adapts the shared authenticated connection Hub to the
// narrow Publisher interface used by Service.
type RealtimePublisher struct {
	Hub *realtime.Hub
}

func (p *RealtimePublisher) PublishMessageCreated(userID string, message Message) {
	env, err := realtime.NewEnvelope(MessageCreatedEvent, messageEvent(message))
	if err == nil {
		p.Hub.SendToUser(userID, env)
	}
}

func (p *RealtimePublisher) PublishMessageDeleted(userID, projectID, messageID string) {
	env, err := realtime.NewEnvelope(MessageDeletedEvent, map[string]string{
		"project_id": projectID,
		"message_id": messageID,
	})
	if err == nil {
		p.Hub.SendToUser(userID, env)
	}
}

func messageEvent(message Message) map[string]any {
	return map[string]any{
		"id":              message.ID,
		"project_id":      message.ProjectID,
		"kind":            message.Kind,
		"author_id":       message.AuthorID,
		"author_username": message.AuthorUsername,
		"body":            message.Body,
		"created_at":      message.CreatedAt,
		"attachments":     message.Attachments,
	}
}
