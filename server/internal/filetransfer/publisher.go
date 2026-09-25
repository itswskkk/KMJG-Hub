package filetransfer

import "github.com/itswskkk/KMJG-Hub/server/internal/realtime"

const (
	RequestedEvent = "file_transfer.requested"
	RespondedEvent = "file_transfer.responded"
	CancelledEvent = "file_transfer.cancelled"
	UploadedEvent  = "file_transfer.uploaded"
)

// RealtimePublisher adapts the shared authenticated connection Hub to the
// narrow Publisher interface used by Service. Audiences are always derived
// from the transfer itself, never from Client input. Events are reload
// hints: the HTTP list endpoints stay authoritative.
type RealtimePublisher struct {
	Hub *realtime.Hub
}

func (p *RealtimePublisher) PublishTransferRequested(t Transfer) {
	p.send(t.RecipientID, RequestedEvent, t)
}

func (p *RealtimePublisher) PublishTransferResponded(t Transfer) {
	p.send(t.SenderID, RespondedEvent, t)
}

func (p *RealtimePublisher) PublishTransferCancelled(t Transfer) {
	p.send(t.RecipientID, CancelledEvent, t)
}

func (p *RealtimePublisher) PublishTransferUploaded(t Transfer) {
	p.send(t.RecipientID, UploadedEvent, t)
}

func (p *RealtimePublisher) send(userID, eventType string, t Transfer) {
	env, err := realtime.NewEnvelope(eventType, transferEvent(t))
	if err == nil {
		p.Hub.SendToUser(userID, env)
	}
}

func transferEvent(t Transfer) map[string]any {
	return map[string]any{
		"id":                 t.ID,
		"sender_id":          t.SenderID,
		"sender_username":    t.SenderUsername,
		"recipient_id":       t.RecipientID,
		"recipient_username": t.RecipientUsername,
		"file_name":          t.FileName,
		"file_size":          t.DeclaredFileSize,
		"status":             string(t.Status),
		"created_at":         t.CreatedAt,
	}
}
