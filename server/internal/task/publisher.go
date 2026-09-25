package task

import "github.com/itswskkk/KMJG-Hub/server/internal/realtime"

const TaskChangedEvent = "project.task.changed"

// RealtimePublisher adapts the shared authenticated connection Hub to the
// deliberately small event-publishing contract used by Service.
type RealtimePublisher struct{ Hub *realtime.Hub }

func (p *RealtimePublisher) PublishTaskChanged(userID, projectID, taskID string) {
	env, err := realtime.NewEnvelope(TaskChangedEvent, map[string]string{
		"project_id": projectID,
		"task_id":    taskID,
	})
	if err == nil {
		p.Hub.SendToUser(userID, env)
	}
}
