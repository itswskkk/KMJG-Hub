package workcontext

import "github.com/itswskkk/KMJG-Hub/server/internal/realtime"

const UpdatedEvent = "project.work-context.updated"

type RealtimePublisher struct{ Hub *realtime.Hub }

func (p *RealtimePublisher) PublishWorkContext(userID string, value Context) {
	env, err := realtime.NewEnvelope(UpdatedEvent, value)
	if err == nil {
		p.Hub.SendToUser(userID, env)
	}
}
