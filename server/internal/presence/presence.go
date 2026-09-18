// Package presence derives Project member online/offline presence from the
// Server's live authenticated WebSocket connections
// (internal/realtime.Hub), per docs/ARCHITECTURE.md "Presence and Work
// Status Architecture".
//
// This package intentionally does not implement Work Status, Current
// Branch, or Current Task. Those are separate, larger pieces of the same
// architecture section and are out of scope for this checkpoint; presence
// (Online/Offline derived from connections) is kept structurally distinct
// so that adding Work Status later does not require reshaping this event
// model.
package presence

import (
	"context"
	"log/slog"

	"github.com/itswskkk/KMJG-Hub/server/internal/realtime"
)

// Event types carried in a realtime.Envelope's Type field.
const (
	// EventSnapshot reports the full current presence state for one
	// Project, sent to a single newly authenticated connection right after
	// it registers (docs/ARCHITECTURE.md "Reconnection": "After
	// reconnection, the Client must synchronize relevant Server state
	// rather than assuming that no events were missed.").
	EventSnapshot = "presence.snapshot"
	// EventUpdated reports that one user's presence changed within one
	// Project.
	EventUpdated = "presence.updated"
)

// MemberPresence is one user's online state within a presence.snapshot.
type MemberPresence struct {
	UserID string `json:"user_id"`
	Online bool   `json:"online"`
}

// SnapshotData is presence.snapshot's payload: the full current presence
// state for one Project, scoped to that Project so a Client can never infer
// anything about a Project it does not belong to from this event alone.
type SnapshotData struct {
	ProjectID string           `json:"project_id"`
	Members   []MemberPresence `json:"members"`
}

// UpdatedData is presence.updated's payload: per docs/ARCHITECTURE.md's
// requirement that presence events make unambiguous "what event occurred;
// which Project context it belongs to ...; which user changed; whether the
// user is currently online."
type UpdatedData struct {
	ProjectID string `json:"project_id"`
	UserID    string `json:"user_id"`
	Online    bool   `json:"online"`
}

// ProjectMembership is the minimal Project-membership query the presence
// system needs. It exists as its own narrow interface (rather than this
// package importing internal/project directly) so the authorization
// boundary presence relies on is explicit and independently testable:
// presence audience is derived entirely from this Server-side query, never
// from anything a Client claims about which Projects or users it wants
// updates for (docs/ARCHITECTURE.md "Event Authorization": "A Client must
// never be trusted to subscribe to arbitrary protected resources simply by
// knowing a Project ... identifier.").
//
// internal/project.Service satisfies this interface; see its
// ProjectIDsForUser and MemberUserIDs methods.
type ProjectMembership interface {
	// ProjectIDsForUser returns the IDs of Projects userID currently
	// belongs to.
	ProjectIDsForUser(ctx context.Context, userID string) ([]string, error)
	// MemberUserIDs returns the user IDs of projectID's current members.
	MemberUserIDs(ctx context.Context, projectID string) ([]string, error)
}

// Broadcaster is the subset of *realtime.Hub the presence Service needs.
// Declared narrowly here (rather than depending on the concrete Hub type
// throughout this file) purely to keep this package's dependency on
// internal/realtime explicit and minimal.
type Broadcaster interface {
	IsOnline(userID string) bool
	SendToUser(userID string, env realtime.Envelope)
}

// Service computes and distributes Project member presence. Its
// HandleUserOnline/HandleUserOffline methods are intended to be wired as a
// realtime.Hub's OnUserOnline/OnUserOffline callbacks.
type Service struct {
	Membership ProjectMembership
	Hub        Broadcaster
}

// HandleUserOnline broadcasts that userID just became Online to every user
// authorized to see it (the members of every Project userID belongs to).
func (s *Service) HandleUserOnline(ctx context.Context, userID string) {
	s.broadcastChange(ctx, userID, true)
}

// HandleUserOffline broadcasts that userID just became Offline, per
// docs/ARCHITECTURE.md "Multiple Client Connections": this only fires once
// userID's last live connection is gone, never for one of several.
func (s *Service) HandleUserOffline(ctx context.Context, userID string) {
	s.broadcastChange(ctx, userID, false)
}

func (s *Service) broadcastChange(ctx context.Context, userID string, online bool) {
	projectIDs, err := s.Membership.ProjectIDsForUser(ctx, userID)
	if err != nil {
		slog.Error("presence: list projects for user failed", "error", err)
		return
	}

	for _, projectID := range projectIDs {
		memberIDs, err := s.Membership.MemberUserIDs(ctx, projectID)
		if err != nil {
			slog.Error("presence: list project members failed", "error", err)
			continue
		}

		env, err := realtime.NewEnvelope(EventUpdated, UpdatedData{
			ProjectID: projectID,
			UserID:    userID,
			Online:    online,
		})
		if err != nil {
			slog.Error("presence: encode event failed", "error", err)
			continue
		}

		// Broadcast only to that Project's own members: this is the
		// enforcement point for cross-Project presence isolation
		// (docs/ARCHITECTURE.md "Clients must not receive Project events
		// for Projects they are not authorized to access."). The user whose
		// presence just changed is skipped: they already know their own
		// connection state (they are the one connecting or disconnecting),
		// so notifying them of it too would be redundant and, worse, would
		// race with their own "connected" acknowledgment and initial
		// snapshot on the very connection that just triggered this event.
		for _, memberID := range memberIDs {
			if memberID == userID {
				continue
			}
			s.Hub.SendToUser(memberID, env)
		}
	}
}

// SendSnapshot pushes one presence.snapshot event per Project client.UserID
// belongs to, addressed to that single connection. Called right after a
// connection authenticates and registers, so a freshly connected or
// reconnected Client always gets authoritative current state rather than
// assuming nothing changed while it was away (docs/ARCHITECTURE.md
// "Reconnection").
func (s *Service) SendSnapshot(ctx context.Context, client *realtime.Client) error {
	projectIDs, err := s.Membership.ProjectIDsForUser(ctx, client.UserID)
	if err != nil {
		return err
	}

	for _, projectID := range projectIDs {
		memberIDs, err := s.Membership.MemberUserIDs(ctx, projectID)
		if err != nil {
			return err
		}

		members := make([]MemberPresence, len(memberIDs))
		for i, memberID := range memberIDs {
			members[i] = MemberPresence{UserID: memberID, Online: s.Hub.IsOnline(memberID)}
		}

		env, err := realtime.NewEnvelope(EventSnapshot, SnapshotData{ProjectID: projectID, Members: members})
		if err != nil {
			return err
		}
		client.Enqueue(env)
	}

	return nil
}
