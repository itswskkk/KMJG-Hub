package realtime

import (
	"context"
	"sync"
)

// Hub is the Server's registry of currently-connected authenticated
// WebSocket Clients, keyed by user. It is the sole source of truth for
// "is this user currently connected" (docs/ARCHITECTURE.md: "The Server
// must not depend on a permanently stored `online` boolean as the
// authoritative source of presence") and for delivering events to a user's
// live connections.
//
// Hub is safe for concurrent use. It has no knowledge of presence, Projects,
// or authorization — those live in internal/presence, which is wired to a
// Hub's OnUserOnline/OnUserOffline callbacks. That separation keeps Hub
// reusable for any future real-time feature that needs "deliver this event
// to this user's live connections" beyond presence.
type Hub struct {
	ctx context.Context

	mu     sync.RWMutex
	byUser map[string]map[*Client]struct{}

	// OnUserOnline and OnUserOffline are invoked when a user's live
	// connection count transitions 0->1 or 1->0 respectively
	// (docs/ARCHITECTURE.md "Presence": "A user may be considered Online
	// while at least one valid Client connection ... remains active.").
	// They must be set before the Hub starts accepting registrations; Hub
	// only reads them from goroutines other than the one that sets them
	// after that point. Left nil, transitions are simply not reported
	// (useful for tests that only need connection bookkeeping).
	OnUserOnline  func(ctx context.Context, userID string)
	OnUserOffline func(ctx context.Context, userID string)

	// Transition callbacks must observe the same relative order as the
	// underlying 0<->1 connection-count transitions actually committed,
	// even under rapid concurrent connect/disconnect/reconnect for the
	// same user — otherwise a downstream consumer (internal/presence) can
	// broadcast a stale "online" event after the true, later "offline"
	// state has already been reached. Calling OnUserOnline/OnUserOffline
	// directly from whichever goroutine happened to commit the transition
	// (the original implementation) does not guarantee this: two
	// goroutines can commit transitions A-then-B under h.mu, then race to
	// invoke their own callback afterwards with no ordering between them,
	// letting B's callback run before A's.
	//
	// The fix: each commit records its outcome in pendingByUser (guarded by
	// its own transitionsMu, never h.mu) inside the same h.mu critical
	// section that commits the connection-count change, so the record
	// order is exactly the commit order. A single dedicated goroutine
	// (runTransitions) drains that map and invokes callbacks, with no lock
	// held during invocation — so a callback is free to call back into
	// IsOnline/SendToUser (which take h.mu) without any risk of deadlock.
	// transitionsMu is a separate, always-fast, never-callback-holding lock
	// specifically so that recording a transition from inside the h.mu
	// critical section can never itself block (a blocking record while
	// holding h.mu, if the consumer needed h.mu to make progress, would
	// deadlock).
	//
	// pendingByUser is keyed by userID with at most one entry per user —
	// deliberately a map, not a queue — so it stays bounded by the number
	// of distinct users with a currently-undispatched transition, never by
	// the number of connect/disconnect events. If user X churns (connects,
	// disconnects, reconnects...) faster than runTransitions can dispatch,
	// each new commit simply overwrites X's single pending entry with its
	// current online/offline value rather than appending another one; any
	// transient intermediate state that gets superseded before dispatch is
	// never individually observable outside the Hub anyway (presence is
	// current state, not an event log — docs/ARCHITECTURE.md "Persistent
	// and Transient Events": "Transient real-time events may include:
	// Online presence changes ... without being retained as permanent
	// application records"), so collapsing them to the latest value loses
	// nothing that matters and can only ever make the eventually-delivered
	// callback reflect a *more* current state, never a stale one. The
	// resulting worst-case size — one entry per user account ever active
	// since the last full drain — is bounded by the Server's registered
	// account count, an operator-controlled quantity that grows slowly
	// through registration, categorically different from (and immune to)
	// an attacker or a single client driving unbounded growth through
	// reconnect churn or slow downstream (DB) callbacks.
	transitionsMu  sync.Mutex
	pendingByUser  map[string]bool
	transitionWake chan struct{}
}

// NewHub constructs an empty Hub. ctx is the Server's long-lived lifetime
// context; it is threaded through to OnUserOnline/OnUserOffline so that
// background work they trigger (presence broadcast, which needs to query
// Project membership) is cancelled on Server shutdown rather than using an
// unbounded context.Background() at every call site. ctx cancellation also
// stops the internal goroutine that dispatches transition callbacks.
func NewHub(ctx context.Context) *Hub {
	h := &Hub{
		ctx:            ctx,
		byUser:         make(map[string]map[*Client]struct{}),
		pendingByUser:  make(map[string]bool),
		transitionWake: make(chan struct{}, 1),
	}
	go h.runTransitions()
	return h
}

// Register adds c to the Hub. If this is userID's first live connection,
// OnUserOnline eventually fires (asynchronously, on the Hub's internal
// transition-dispatch goroutine — see recordTransition) after the
// registration is committed.
func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	set, ok := h.byUser[c.UserID]
	if !ok {
		set = make(map[*Client]struct{})
		h.byUser[c.UserID] = set
	}
	wasEmpty := len(set) == 0
	set[c] = struct{}{}
	if wasEmpty {
		h.recordTransition(c.UserID, true)
	}
	h.mu.Unlock()
}

// Unregister removes c from the Hub. It is idempotent: unregistering a
// Client that is not (or no longer) registered is a no-op. If removing c
// leaves userID with zero live connections, OnUserOffline eventually fires
// (asynchronously — see recordTransition) after the removal is committed.
//
// Unregister does not close c itself — callers that want the underlying
// connection torn down should call c.Close(), typically from the same
// transport-layer defer that calls Unregister (see internal/httpapi's read
// pump), so bookkeeping and teardown are ordered consistently from one
// place.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	set, ok := h.byUser[c.UserID]
	if !ok {
		h.mu.Unlock()
		return
	}
	if _, present := set[c]; !present {
		h.mu.Unlock()
		return
	}
	delete(set, c)
	becameEmpty := len(set) == 0
	if becameEmpty {
		delete(h.byUser, c.UserID)
		h.recordTransition(c.UserID, false)
	}
	h.mu.Unlock()
}

// recordTransition records userID's latest online/offline outcome and
// wakes the dispatch goroutine. Must be called while holding h.mu (so the
// record order exactly matches the commit order established by h.mu's
// serialization of Register/Unregister) but must never itself block:
// transitionsMu only ever guards a plain bounded map write and is never
// held across a callback, and the wake send is non-blocking, so a caller
// holding h.mu here can never end up waiting on the very goroutine that
// might need h.mu (via IsOnline/SendToUser inside a callback) to make
// progress.
//
// Writing pendingByUser[userID] = online unconditionally overwrites any
// not-yet-dispatched entry for the same user (coalescing), which is what
// keeps this map bounded — see the field's doc comment on Hub.
func (h *Hub) recordTransition(userID string, online bool) {
	h.transitionsMu.Lock()
	h.pendingByUser[userID] = online
	h.transitionsMu.Unlock()

	select {
	case h.transitionWake <- struct{}{}:
	default:
		// A wake is already pending; the dispatch goroutine hasn't
		// consumed it yet and will drain every recorded transition
		// (including this one) whenever it next runs.
	}
}

// runTransitions is the Hub's single sequential dispatcher of transition
// callbacks. Processing pendingByUser from exactly one goroutine is what
// guarantees OnUserOnline/OnUserOffline are observed in the same relative
// order the corresponding connection-count transitions were actually
// committed (per-user; see recordTransition's coalescing for why at most
// one entry per user can ever be in flight at a time). Callbacks run with
// no lock held.
func (h *Hub) runTransitions() {
	for {
		h.transitionsMu.Lock()
		pending := h.pendingByUser
		h.pendingByUser = make(map[string]bool)
		h.transitionsMu.Unlock()

		for userID, online := range pending {
			if online {
				if h.OnUserOnline != nil {
					h.OnUserOnline(h.ctx, userID)
				}
			} else if h.OnUserOffline != nil {
				h.OnUserOffline(h.ctx, userID)
			}
		}

		if len(pending) > 0 {
			// More transitions may have been recorded while we were
			// dispatching this batch; check again immediately rather than
			// waiting on transitionWake, which only guarantees a signal
			// is pending, not that it fires promptly.
			continue
		}

		select {
		case <-h.ctx.Done():
			return
		case <-h.transitionWake:
		}
	}
}

// IsOnline reports whether userID has at least one live connection.
func (h *Hub) IsOnline(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.byUser[userID]) > 0
}

// SendToUser delivers env to every live connection belonging to userID.
// Delivery is best-effort and non-blocking per connection (see
// Client.Enqueue); a user with no live connections simply receives nothing,
// which is expected for transient real-time events
// (docs/ARCHITECTURE.md "Persistent and Transient Events").
func (h *Hub) SendToUser(userID string, env Envelope) {
	h.mu.RLock()
	set := h.byUser[userID]
	clients := make([]*Client, 0, len(set))
	for c := range set {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		c.Enqueue(env)
	}
}

// CloseByTokenHash closes every currently registered connection whose
// session token hashes to tokenHash. Used to immediately drop WebSocket
// connections belonging to a session the Server just revoked (explicit
// logout), per docs/ARCHITECTURE.md "Logout and Revocation": "A revoked or
// expired session must no longer authorize ... WebSocket connections."
func (h *Hub) CloseByTokenHash(tokenHash string) {
	for _, c := range h.snapshot() {
		if c.TokenHash == tokenHash {
			c.Close()
		}
	}
}

// Sweep closes every currently registered connection whose session is no
// longer active according to isActive, which receives each connection's
// TokenHash (never a raw token). This is the fallback mechanism that
// eventually disconnects sessions invalidated by means other than an
// explicit logout the Hub was told about directly — most importantly
// ordinary session expiry (docs/ARCHITECTURE.md "Session Security":
// sessions "expire according to Server authentication policy", and that
// must eventually apply to already-open WebSocket connections too).
func (h *Hub) Sweep(isActive func(tokenHash string) bool) {
	for _, c := range h.snapshot() {
		if !isActive(c.TokenHash) {
			c.Close()
		}
	}
}

// Shutdown closes every currently registered connection, for clean Server
// shutdown (docs/ARCHITECTURE.md's expectation of "clean server shutdown /
// connection cleanup where relevant").
func (h *Hub) Shutdown() {
	for _, c := range h.snapshot() {
		c.Close()
	}
}

// snapshot returns every currently registered Client. Copying out of the
// map under the lock, then acting on the copy without holding it, keeps
// long-running per-connection work (network writes, closing sockets) from
// blocking unrelated Register/Unregister calls.
func (h *Hub) snapshot() []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var all []*Client
	for _, set := range h.byUser {
		for c := range set {
			all = append(all, c)
		}
	}
	return all
}
