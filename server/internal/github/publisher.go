package github

import "github.com/itswskkk/KMJG-Hub/server/internal/realtime"

const (
	RepositoryConnectedEvent    = "project.git.repository_connected"
	RepositoryDisconnectedEvent = "project.git.repository_disconnected"
	PushedEvent                 = "project.git.pushed"
)

// RealtimePublisher adapts the shared connection Hub to Publisher.
type RealtimePublisher struct {
	Hub *realtime.Hub
}

func (p *RealtimePublisher) PublishRepositoryConnected(userID string, repo Repository) {
	p.send(userID, RepositoryConnectedEvent, map[string]any{
		"project_id":     repo.ProjectID,
		"provider":       repo.Provider,
		"owner_login":    repo.OwnerLogin,
		"name":           repo.Name,
		"html_url":       repo.HTMLURL,
		"default_branch": repo.DefaultBranch,
	})
}

func (p *RealtimePublisher) PublishRepositoryDisconnected(userID, projectID string) {
	p.send(userID, RepositoryDisconnectedEvent, map[string]string{"project_id": projectID})
}

func (p *RealtimePublisher) PublishPush(userID, projectID string, event PushEvent) {
	p.send(userID, PushedEvent, map[string]any{
		"project_id":       projectID,
		"branch":           event.Branch,
		"pusher_login":     event.PusherLogin,
		"commit_count":     event.CommitCount,
		"commit_summaries": event.CommitSummaries,
	})
}

func (p *RealtimePublisher) send(userID, eventType string, payload any) {
	env, err := realtime.NewEnvelope(eventType, payload)
	if err == nil {
		p.Hub.SendToUser(userID, env)
	}
}
