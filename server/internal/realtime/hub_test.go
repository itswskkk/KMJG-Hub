package realtime_test

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/realtime"
)

// newTestClient returns a Client wired to a no-op transport close, plus a
// closedCh that is closed when that close hook runs — enough to assert
// whether Hub actually tore a connection down without a real network
// connection.
func newTestClient(userID, tokenHash string) (*realtime.Client, <-chan struct{}) {
	closed := make(chan struct{})
	var once sync.Once
	c := realtime.NewClient(userID, tokenHash, func() {
		once.Do(func() { close(closed) })
	})
	return c, closed
}

func waitFor(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal(msg)
	}
}

func assertNoSignal(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal(msg)
	case <-time.After(50 * time.Millisecond):
	}
}

// callRecorder is a mutex-protected recorder for OnUserOnline/OnUserOffline
// invocations. Hub now dispatches those callbacks from its own internal
// goroutine, asynchronously with respect to the Register/Unregister call
// that triggered them (see hub.go's runTransitions), so tests must
// synchronize on and wait for recorded calls rather than assume they have
// already happened by the time Register/Unregister returns.
type callRecorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *callRecorder) add(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, s)
}

func (r *callRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

// waitLen blocks (polling) until at least n calls have been recorded, or
// fails the test after timeout.
func (r *callRecorder) waitLen(t *testing.T, n int, timeout time.Duration) []string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		got := r.snapshot()
		if len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected at least %d recorded call(s) within %s, got %v", n, timeout, got)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestOneConnectionMakesUserOnline(t *testing.T) {
	hub := realtime.NewHub(context.Background())

	var recorder callRecorder
	hub.OnUserOnline = func(_ context.Context, userID string) { recorder.add(userID) }

	c, _ := newTestClient("user-1", "hash-1")
	if hub.IsOnline("user-1") {
		t.Fatal("expected user-1 offline before registering")
	}

	hub.Register(c)

	// IsOnline reflects the connection-count commit, which happens
	// synchronously inside Register — unlike the callback below, no wait
	// is needed here.
	if !hub.IsOnline("user-1") {
		t.Fatal("expected user-1 online after registering")
	}
	got := recorder.waitLen(t, 1, time.Second)
	if len(got) != 1 || got[0] != "user-1" {
		t.Fatalf("expected exactly one OnUserOnline(user-1) call, got %v", got)
	}
}

func TestLastConnectionDisconnectMakesUserOffline(t *testing.T) {
	hub := realtime.NewHub(context.Background())

	var recorder callRecorder
	hub.OnUserOffline = func(_ context.Context, userID string) { recorder.add(userID) }

	c, _ := newTestClient("user-1", "hash-1")
	hub.Register(c)
	hub.Unregister(c)

	if hub.IsOnline("user-1") {
		t.Fatal("expected user-1 offline after its only connection is unregistered")
	}
	got := recorder.waitLen(t, 1, time.Second)
	if len(got) != 1 || got[0] != "user-1" {
		t.Fatalf("expected exactly one OnUserOffline(user-1) call, got %v", got)
	}
}

func TestOneOfMultipleConnectionsDisconnectingStaysOnline(t *testing.T) {
	hub := realtime.NewHub(context.Background())

	var onlineRec, offlineRec callRecorder
	hub.OnUserOnline = func(_ context.Context, userID string) { onlineRec.add(userID) }
	hub.OnUserOffline = func(_ context.Context, userID string) { offlineRec.add(userID) }

	c1, _ := newTestClient("user-1", "hash-1")
	c2, _ := newTestClient("user-1", "hash-2") // second device/tab, distinct session

	hub.Register(c1)
	hub.Register(c2)
	if got := onlineRec.waitLen(t, 1, time.Second); len(got) != 1 {
		t.Fatalf("expected exactly one online transition for two connections from the same user, got %v", got)
	}

	hub.Unregister(c1)
	if !hub.IsOnline("user-1") {
		t.Fatal("expected user-1 to remain online while a second connection is still active")
	}
	// Give a hypothetical (incorrect) offline callback a chance to fire
	// before asserting its absence — there is no positive event to wait on
	// here, only an absence, so this uses a bounded delay like
	// assertNoSignal elsewhere in this file.
	time.Sleep(50 * time.Millisecond)
	if got := offlineRec.snapshot(); len(got) != 0 {
		t.Fatalf("expected no offline transition yet, got %v", got)
	}

	hub.Unregister(c2)
	if hub.IsOnline("user-1") {
		t.Fatal("expected user-1 offline once its last connection disconnects")
	}
	if got := offlineRec.waitLen(t, 1, time.Second); len(got) != 1 {
		t.Fatalf("expected exactly one offline transition, got %v", got)
	}
}

func TestUnregisterIsIdempotent(t *testing.T) {
	hub := realtime.NewHub(context.Background())
	var recorder callRecorder
	hub.OnUserOffline = func(_ context.Context, _ string) { recorder.add("x") }

	c, _ := newTestClient("user-1", "hash-1")
	hub.Register(c)
	hub.Unregister(c)
	hub.Unregister(c) // duplicate call, e.g. from both a forced close and the transport's own cleanup

	recorder.waitLen(t, 1, time.Second)
	time.Sleep(50 * time.Millisecond) // let a hypothetical duplicate callback fire before asserting its absence
	if got := recorder.snapshot(); len(got) != 1 {
		t.Fatalf("expected exactly one offline transition despite duplicate Unregister, got %v", got)
	}
}

// TestTransitionCallbacksPreserveCommitOrderAcrossDispatchBatches is a
// deterministic regression test for the ordering bug originally fixed in
// hub.go: Register/Unregister used to invoke OnUserOnline/OnUserOffline
// directly from whichever goroutine committed the transition, with no
// ordering guarantee between one transition's callback and the next
// transition's callback if the first was slow. A later "offline" callback
// could complete before an earlier, still-in-flight "online" callback — a
// stale, out-of-order presence event.
//
// Hub now also coalesces same-user transitions that are still sitting
// undispatched in the same pending batch (see the bounded-queue tests
// below), so this test specifically exercises two transitions that land in
// *separate* dispatch batches — the case coalescing does not apply to —
// and checks they are still delivered in commit order. It forces that
// deterministically (no reliance on scheduler luck or sleep-based timing to
// create the race): it blocks the first OnUserOnline call on a gate the
// test controls until the dispatcher has already taken sole ownership of
// that transition (so a second, later transition for the same user cannot
// coalesce with it), commits the second transition while the first is
// still blocked, and asserts neither has completed until released.
func TestTransitionCallbacksPreserveCommitOrderAcrossDispatchBatches(t *testing.T) {
	hub := realtime.NewHub(context.Background())

	var mu sync.Mutex
	var order []string

	firstOnlineEntered := make(chan struct{})
	firstOnlineGate := make(chan struct{})
	var enterOnce sync.Once

	hub.OnUserOnline = func(_ context.Context, userID string) {
		enterOnce.Do(func() { close(firstOnlineEntered) })
		<-firstOnlineGate // hold this callback in flight on purpose

		mu.Lock()
		order = append(order, "online:"+userID)
		mu.Unlock()
	}
	hub.OnUserOffline = func(_ context.Context, userID string) {
		mu.Lock()
		order = append(order, "offline:"+userID)
		mu.Unlock()
	}

	c1, _ := newTestClient("user-1", "hash-1")

	// Commit #1: online. The dispatcher drains this into its own batch and
	// blocks inside the callback below, which removes it from
	// pendingByUser entirely (the map was already swapped for a fresh one)
	// — so commit #2 below cannot coalesce with it.
	hub.Register(c1)
	waitFor(t, firstOnlineEntered, "expected the first OnUserOnline call to have started")

	// Commit #2: offline (c1 was the only connection). Recorded into the
	// fresh pendingByUser map, to be dispatched as its own callback once
	// the dispatcher gets back around to it.
	hub.Unregister(c1)

	// While commit #1's callback is still blocked, the offline callback
	// must not have fired yet — proving dispatch is still strictly
	// sequential (a single dispatcher goroutine), not reordered.
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	inFlight := append([]string(nil), order...)
	mu.Unlock()
	if len(inFlight) != 0 {
		t.Fatalf("expected no callbacks to have completed while the first is still blocked, got %v", inFlight)
	}

	close(firstOnlineGate) // release commit #1's callback

	deadline := time.Now().Add(time.Second)
	for {
		mu.Lock()
		done := len(order) >= 2
		mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			mu.Lock()
			t.Fatalf("timed out waiting for both callbacks, got %v", order)
			mu.Unlock()
		}
		time.Sleep(time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"online:user-1", "offline:user-1"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("expected callbacks in commit order %v, got %v", want, order)
	}
}

// TestRapidChurnForSameUserCoalescesToFinalState is a deterministic
// regression test for the unbounded-queue fix: pendingByUser holds at most
// one entry per user, so rapid connect/disconnect churn for one user while
// the dispatcher is busy elsewhere must coalesce down to a single
// eventual callback reflecting the truthful final state — never one
// callback per raw commit, and never a stale intermediate state.
//
// The dispatcher is deterministically stalled (not via sleep) by blocking
// an unrelated "blocker" user's OnUserOnline call on a gate, so every
// commit for "user-1" below is guaranteed to land in the same
// not-yet-drained pendingByUser map before any of them are dispatched.
func TestRapidChurnForSameUserCoalescesToFinalState(t *testing.T) {
	hub := realtime.NewHub(context.Background())

	var mu sync.Mutex
	var onlineCalls, offlineCalls int
	var lastState string

	blockerEntered := make(chan struct{})
	blockerGate := make(chan struct{})
	var enterOnce sync.Once

	hub.OnUserOnline = func(_ context.Context, userID string) {
		if userID == "blocker" {
			enterOnce.Do(func() { close(blockerEntered) })
			<-blockerGate
			return
		}
		mu.Lock()
		onlineCalls++
		lastState = "online"
		mu.Unlock()
	}
	hub.OnUserOffline = func(_ context.Context, userID string) {
		mu.Lock()
		offlineCalls++
		lastState = "offline"
		mu.Unlock()
	}

	blocker, _ := newTestClient("blocker", "hash-blocker")
	hub.Register(blocker)
	waitFor(t, blockerEntered, "expected the blocker's OnUserOnline to have started")

	// 500 independent connect/disconnect pairs for the same user — 1000
	// real, individually-committed transitions — all recorded while the
	// dispatcher is stuck on the blocker above. If pendingByUser were an
	// unbounded per-event queue, this would grow to 1000 entries; because
	// it coalesces per user, it can hold at most one.
	const churn = 500
	for i := 0; i < churn; i++ {
		c := realtime.NewClient("user-1", fmt.Sprintf("hash-%d", i), func() {})
		hub.Register(c)
		hub.Unregister(c)
	}
	if hub.IsOnline("user-1") {
		t.Fatal("expected user-1 offline after every churned connection closed")
	}

	close(blockerGate) // let the dispatcher proceed to drain the coalesced backlog

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		total := onlineCalls + offlineCalls
		mu.Unlock()
		if total > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the coalesced callback")
		}
		time.Sleep(time.Millisecond)
	}

	// Give a hypothetical (incorrect) second callback a chance to fire
	// before asserting there is only ever one.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if total := onlineCalls + offlineCalls; total != 1 {
		t.Fatalf("expected exactly one coalesced callback for %d churns, got online=%d offline=%d", churn, onlineCalls, offlineCalls)
	}
	if lastState != "offline" {
		t.Fatalf("expected the coalesced callback to reflect the true final state (offline), got %q", lastState)
	}
}

// TestChurnAcrossManyUsersCoalescesPerUserAndReflectsFinalState shows the
// bound in the shape it actually matters: however many raw connect/
// disconnect commits occur, the number of eventually-delivered callbacks
// is bounded by the number of *distinct users*, not by event volume, and
// each user's coalesced callback reflects that specific user's own true
// final state (not another user's, and not a stale one).
func TestChurnAcrossManyUsersCoalescesPerUserAndReflectsFinalState(t *testing.T) {
	const numUsers = 50
	const churnPerUser = 20

	hub := realtime.NewHub(context.Background())

	var mu sync.Mutex
	finalOnline := make(map[string]bool)
	callCount := make(map[string]int)

	blockerEntered := make(chan struct{})
	blockerGate := make(chan struct{})
	var enterOnce sync.Once

	hub.OnUserOnline = func(_ context.Context, userID string) {
		if userID == "blocker" {
			enterOnce.Do(func() { close(blockerEntered) })
			<-blockerGate
			return
		}
		mu.Lock()
		finalOnline[userID] = true
		callCount[userID]++
		mu.Unlock()
	}
	hub.OnUserOffline = func(_ context.Context, userID string) {
		mu.Lock()
		finalOnline[userID] = false
		callCount[userID]++
		mu.Unlock()
	}

	blocker, _ := newTestClient("blocker", "hash-blocker")
	hub.Register(blocker)
	waitFor(t, blockerEntered, "expected the blocker's OnUserOnline to have started")

	// numUsers distinct users each churn churnPerUser times (2*churnPerUser
	// raw commits each) while the dispatcher is stalled. Every user's
	// connections are closed again except the very last user's very last
	// connection, which is left open — giving a real mix of final states
	// to verify below, not just "everyone offline."
	lastUserID := fmt.Sprintf("user-%d", numUsers-1)
	for u := 0; u < numUsers; u++ {
		userID := fmt.Sprintf("user-%d", u)
		for i := 0; i < churnPerUser; i++ {
			c := realtime.NewClient(userID, fmt.Sprintf("hash-%d-%d", u, i), func() {})
			hub.Register(c)
			if userID == lastUserID && i == churnPerUser-1 {
				continue // leave this one connection open
			}
			hub.Unregister(c)
		}
	}

	close(blockerGate)

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		total := 0
		for _, n := range callCount {
			total += n
		}
		mu.Unlock()
		if total == numUsers {
			break
		}
		if time.Now().After(deadline) {
			mu.Lock()
			t.Fatalf("timed out waiting for one coalesced callback per user (%d expected), got %v", numUsers, callCount)
			mu.Unlock()
		}
		time.Sleep(time.Millisecond)
	}

	// Give any hypothetical extra (non-coalesced) callback a chance to
	// fire before asserting the per-user count is exactly one.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	for u := 0; u < numUsers; u++ {
		userID := fmt.Sprintf("user-%d", u)
		if callCount[userID] != 1 {
			t.Fatalf("expected exactly one coalesced callback for %s despite %d churns, got %d", userID, churnPerUser, callCount[userID])
		}
		wantOnline := userID == lastUserID
		if finalOnline[userID] != wantOnline {
			t.Fatalf("expected %s's final coalesced state to be online=%v, got %v", userID, wantOnline, finalOnline[userID])
		}
	}
	if !hub.IsOnline(lastUserID) {
		t.Fatalf("expected %s to actually be online (its last connection was left open)", lastUserID)
	}
}

func TestSendToUserDeliversToAllUserConnections(t *testing.T) {
	hub := realtime.NewHub(context.Background())
	c1, _ := newTestClient("user-1", "hash-1")
	c2, _ := newTestClient("user-1", "hash-2")
	other, _ := newTestClient("user-2", "hash-3")
	hub.Register(c1)
	hub.Register(c2)
	hub.Register(other)

	env, err := realtime.NewEnvelope("test.event", map[string]string{"hello": "world"})
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	hub.SendToUser("user-1", env)

	for i, c := range []*realtime.Client{c1, c2} {
		select {
		case got := <-c.Outbound():
			if got.Type != "test.event" {
				t.Fatalf("client %d: expected test.event, got %q", i, got.Type)
			}
		case <-time.After(time.Second):
			t.Fatalf("client %d: expected to receive the event", i)
		}
	}

	select {
	case <-other.Outbound():
		t.Fatal("user-2's connection should not have received user-1's event")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestEnqueueDropsSlowConsumer(t *testing.T) {
	hub := realtime.NewHub(context.Background())
	c, closed := newTestClient("user-1", "hash-1")
	hub.Register(c)

	env, err := realtime.NewEnvelope("test.event", struct{}{})
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}

	// Fill the outbound buffer without draining it, then push one more: the
	// Client must be treated as a slow/broken consumer and disconnected
	// rather than blocking this goroutine or growing the queue further.
	const overflow = 40
	done := make(chan struct{})
	go func() {
		for i := 0; i < overflow; i++ {
			c.Enqueue(env)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Enqueue blocked instead of dropping the slow consumer")
	}
	waitFor(t, closed, "expected the slow consumer's connection to be closed")
}

func TestCloseByTokenHashClosesOnlyMatchingConnection(t *testing.T) {
	hub := realtime.NewHub(context.Background())
	c1, closed1 := newTestClient("user-1", "hash-1")
	c2, closed2 := newTestClient("user-1", "hash-2")
	hub.Register(c1)
	hub.Register(c2)

	hub.CloseByTokenHash("hash-1")

	waitFor(t, closed1, "expected the matching connection to be closed")
	assertNoSignal(t, closed2, "expected the other connection to remain open")
}

func TestSweepClosesInactiveSessions(t *testing.T) {
	hub := realtime.NewHub(context.Background())
	active, activeClosed := newTestClient("user-1", "active-hash")
	stale, staleClosed := newTestClient("user-2", "stale-hash")
	hub.Register(active)
	hub.Register(stale)

	hub.Sweep(func(tokenHash string) bool { return tokenHash == "active-hash" })

	waitFor(t, staleClosed, "expected the stale session's connection to be closed")
	assertNoSignal(t, activeClosed, "expected the active session's connection to remain open")
}

func TestShutdownClosesEveryConnection(t *testing.T) {
	hub := realtime.NewHub(context.Background())
	c1, closed1 := newTestClient("user-1", "hash-1")
	c2, closed2 := newTestClient("user-2", "hash-2")
	hub.Register(c1)
	hub.Register(c2)

	hub.Shutdown()

	waitFor(t, closed1, "expected connection 1 to be closed on shutdown")
	waitFor(t, closed2, "expected connection 2 to be closed on shutdown")
}

func TestEnvelopeRoundTrip(t *testing.T) {
	env, err := realtime.NewEnvelope("presence.updated", map[string]any{
		"project_id": "p1",
		"user_id":    "u1",
		"online":     true,
	})
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	if env.V != realtime.ProtocolVersion {
		t.Fatalf("expected protocol version %d, got %d", realtime.ProtocolVersion, env.V)
	}
	if env.Type != "presence.updated" {
		t.Fatalf("expected type presence.updated, got %q", env.Type)
	}
	if len(env.Data) == 0 {
		t.Fatal("expected non-empty data payload")
	}
}

// TestConcurrentRegisterUnregisterSendRace exercises Hub under concurrent
// use from many goroutines simultaneously registering, unregistering, and
// broadcasting, intended to be run with -race to catch data races in the
// connection registry.
func TestConcurrentRegisterUnregisterSendRace(t *testing.T) {
	hub := realtime.NewHub(context.Background())
	hub.OnUserOnline = func(context.Context, string) {}
	hub.OnUserOffline = func(context.Context, string) {}

	env, err := realtime.NewEnvelope("test.event", struct{}{})
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			userID := "user-1"
			c, _ := newTestClient(userID, "hash")
			hub.Register(c)
			hub.SendToUser(userID, env)
			_ = hub.IsOnline(userID)
			hub.Unregister(c)
		}(i)
	}
	wg.Wait()
}
