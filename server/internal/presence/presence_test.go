package presence_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/presence"
	"github.com/itswskkk/KMJG-Hub/server/internal/realtime"
)

// fakeMembership is a minimal in-memory presence.ProjectMembership fixture:
// project IDs to member user IDs, set up directly by each test rather than
// through any product API (there is no product-facing way to add a second
// Project member yet — see PROGRESS.md).
type fakeMembership struct {
	mu               sync.Mutex
	projectsByUser   map[string][]string
	membersByProject map[string][]string
}

func newFakeMembership() *fakeMembership {
	return &fakeMembership{
		projectsByUser:   make(map[string][]string),
		membersByProject: make(map[string][]string),
	}
}

func (f *fakeMembership) addProject(projectID string, memberIDs ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.membersByProject[projectID] = memberIDs
	for _, uid := range memberIDs {
		f.projectsByUser[uid] = append(f.projectsByUser[uid], projectID)
	}
}

func (f *fakeMembership) ProjectIDsForUser(_ context.Context, userID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.projectsByUser[userID]...), nil
}

func (f *fakeMembership) MemberUserIDs(_ context.Context, projectID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.membersByProject[projectID]...), nil
}

func newTestClient(userID string) *realtime.Client {
	return realtime.NewClient(userID, "hash-"+userID, func() {})
}

func recvUpdated(t *testing.T, c *realtime.Client) presence.UpdatedData {
	t.Helper()
	select {
	case env := <-c.Outbound():
		if env.Type != presence.EventUpdated {
			t.Fatalf("expected event type %q, got %q", presence.EventUpdated, env.Type)
		}
		var data presence.UpdatedData
		if err := json.Unmarshal(env.Data, &data); err != nil {
			t.Fatalf("decode presence.updated payload: %v", err)
		}
		return data
	case <-time.After(time.Second):
		t.Fatal("expected a presence.updated event")
		return presence.UpdatedData{}
	}
}

func assertNoEvent(t *testing.T, c *realtime.Client) {
	t.Helper()
	select {
	case env := <-c.Outbound():
		t.Fatalf("expected no event, got %q", env.Type)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestPresenceIsolatedAcrossProjects is the core cross-Project isolation
// check: A belongs to Project X (with B) and Project Y (with C). When A
// connects, B must learn about it via Project X's context only, and C must
// learn about it via Project Y's context only — neither should see the
// other Project's context, and a completely unrelated user D must see
// nothing at all.
func TestPresenceIsolatedAcrossProjects(t *testing.T) {
	membership := newFakeMembership()
	membership.addProject("project-x", "user-a", "user-b")
	membership.addProject("project-y", "user-a", "user-c")

	hub := realtime.NewHub(context.Background())
	svc := &presence.Service{Membership: membership, Hub: hub}
	hub.OnUserOnline = svc.HandleUserOnline
	hub.OnUserOffline = svc.HandleUserOffline

	clientB := newTestClient("user-b")
	clientC := newTestClient("user-c")
	clientD := newTestClient("user-d") // no shared project with anyone
	hub.Register(clientB)
	hub.Register(clientC)
	hub.Register(clientD)

	clientA := newTestClient("user-a")
	hub.Register(clientA) // triggers HandleUserOnline("user-a")

	updateB := recvUpdated(t, clientB)
	if updateB.ProjectID != "project-x" || updateB.UserID != "user-a" || !updateB.Online {
		t.Fatalf("unexpected event for B: %+v", updateB)
	}
	assertNoEvent(t, clientB) // must not also receive project-y's event

	updateC := recvUpdated(t, clientC)
	if updateC.ProjectID != "project-y" || updateC.UserID != "user-a" || !updateC.Online {
		t.Fatalf("unexpected event for C: %+v", updateC)
	}
	assertNoEvent(t, clientC) // must not also receive project-x's event

	assertNoEvent(t, clientD) // shares no project with user-a at all

	// Now user-a disconnects: both B and C should learn Offline, still
	// scoped to their own shared Project.
	hub.Unregister(clientA)

	offlineB := recvUpdated(t, clientB)
	if offlineB.ProjectID != "project-x" || offlineB.Online {
		t.Fatalf("unexpected offline event for B: %+v", offlineB)
	}
	offlineC := recvUpdated(t, clientC)
	if offlineC.ProjectID != "project-y" || offlineC.Online {
		t.Fatalf("unexpected offline event for C: %+v", offlineC)
	}
	assertNoEvent(t, clientD)
}

func TestSendSnapshotReflectsCurrentOnlineMembers(t *testing.T) {
	membership := newFakeMembership()
	membership.addProject("project-x", "user-a", "user-b", "user-c")

	hub := realtime.NewHub(context.Background())
	svc := &presence.Service{Membership: membership, Hub: hub}
	hub.OnUserOnline = svc.HandleUserOnline
	hub.OnUserOffline = svc.HandleUserOffline

	// B is already connected; C is not. A is connecting now.
	hub.Register(newTestClient("user-b"))

	clientA := newTestClient("user-a")
	hub.Register(clientA)

	if err := svc.SendSnapshot(context.Background(), clientA); err != nil {
		t.Fatalf("SendSnapshot: %v", err)
	}

	// clientA's queue may also legitimately receive a presence.updated
	// about user-b's earlier online transition here: Hub dispatches
	// transition callbacks asynchronously on its own goroutine (see
	// hub.go's runTransitions), so user-b's broadcast — enqueued before
	// user-a even registered — can be processed at any point after A
	// registers, racing harmlessly with this test's own direct
	// SendSnapshot call. This mirrors real concurrent connections (B and A
	// registering from different goroutines) rather than being an
	// artifact of the fix, so the test drains for the snapshot instead of
	// assuming it is the first (or only) message, while still requiring
	// any interleaved update to be exactly the one legitimate event it
	// could be.
	var snapshot *presence.SnapshotData
	deadline := time.After(time.Second)
	for snapshot == nil {
		select {
		case env := <-clientA.Outbound():
			switch env.Type {
			case presence.EventSnapshot:
				var data presence.SnapshotData
				if err := json.Unmarshal(env.Data, &data); err != nil {
					t.Fatalf("decode snapshot: %v", err)
				}
				snapshot = &data
			case presence.EventUpdated:
				var data presence.UpdatedData
				if err := json.Unmarshal(env.Data, &data); err != nil {
					t.Fatalf("decode update: %v", err)
				}
				if data.ProjectID != "project-x" || data.UserID != "user-b" || !data.Online {
					t.Fatalf("unexpected interleaved update: %+v", data)
				}
			default:
				t.Fatalf("unexpected event type %q", env.Type)
			}
		case <-deadline:
			t.Fatal("expected a presence.snapshot event")
		}
	}

	if snapshot.ProjectID != "project-x" {
		t.Fatalf("expected project-x, got %q", snapshot.ProjectID)
	}
	online := make(map[string]bool, len(snapshot.Members))
	for _, m := range snapshot.Members {
		online[m.UserID] = m.Online
	}
	if !online["user-a"] {
		t.Error("expected user-a (self, just registered) to be online in its own snapshot")
	}
	if !online["user-b"] {
		t.Error("expected user-b to be online in the snapshot")
	}
	if online["user-c"] {
		t.Error("expected user-c to be offline in the snapshot")
	}
}

func TestMultipleConnectionsSameUserOnlyOneTransitionBroadcast(t *testing.T) {
	membership := newFakeMembership()
	membership.addProject("project-x", "user-a", "user-b")

	hub := realtime.NewHub(context.Background())
	svc := &presence.Service{Membership: membership, Hub: hub}
	hub.OnUserOnline = svc.HandleUserOnline
	hub.OnUserOffline = svc.HandleUserOffline

	clientB := newTestClient("user-b")
	hub.Register(clientB)

	aDevice1 := newTestClient("user-a")
	aDevice2 := newTestClient("user-a")
	hub.Register(aDevice1)
	hub.Register(aDevice2)

	recvUpdated(t, clientB) // exactly one online event expected
	assertNoEvent(t, clientB)

	hub.Unregister(aDevice1) // one of two connections closing: no offline event
	assertNoEvent(t, clientB)

	hub.Unregister(aDevice2) // last connection closing: offline event
	offline := recvUpdated(t, clientB)
	if offline.Online {
		t.Fatalf("expected an offline event, got %+v", offline)
	}
}
