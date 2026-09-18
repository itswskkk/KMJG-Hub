package realtime

import "sync"

// outboundBufferSize bounds how many not-yet-written events a single
// connection may have queued before it is treated as a slow or broken
// consumer and disconnected, per docs/ARCHITECTURE.md concerns about a slow
// Client blocking the Server or an unbounded buffer growing without limit.
const outboundBufferSize = 32

// Client is one authenticated WebSocket connection registered with a Hub.
// Multiple Clients may share the same UserID at once (docs/ARCHITECTURE.md
// "Multiple Client Connections": "A user may have more than one active
// Client connection.").
type Client struct {
	// UserID is the authenticated user this connection belongs to. Set once
	// at construction from the Server's own session lookup — never from
	// anything the Client claims about itself.
	UserID string
	// TokenHash is the session's hashed bearer token (as produced by
	// auth.HashSessionToken), kept only so the Hub can later find and close
	// this connection by session (logout, revocation, expiry) or run
	// periodic session-liveness checks. The raw token is never stored here
	// or logged, per docs/ARCHITECTURE.md "Session Security".
	TokenHash string

	outbound  chan Envelope
	stop      chan struct{}
	closeOnce sync.Once
	closeConn func()
}

// NewClient constructs a Client for an already-authenticated connection.
// closeConn is supplied by the transport layer (internal/httpapi) and
// physically closes the underlying network connection; it is called at most
// once no matter how many times Close is invoked or from how many
// goroutines (session sweep, explicit logout, Server shutdown, and the
// transport's own disconnect handling may all reach it for the same
// Client).
func NewClient(userID, tokenHash string, closeConn func()) *Client {
	return &Client{
		UserID:    userID,
		TokenHash: tokenHash,
		outbound:  make(chan Envelope, outboundBufferSize),
		stop:      make(chan struct{}),
		closeConn: closeConn,
	}
}

// Outbound is the channel the transport's write pump should range over to
// learn what to send next.
func (c *Client) Outbound() <-chan Envelope { return c.outbound }

// StopSignal is closed exactly once, when this Client is being torn down,
// so a transport write pump blocked on a select can stop promptly instead
// of waiting for its next outbound event or ping tick.
func (c *Client) StopSignal() <-chan struct{} { return c.stop }

// Enqueue delivers env to this Client without blocking the caller (a
// presence broadcast reaching many Clients must never stall on one slow
// reader). If the outbound queue is already full, this Client is treated as
// a slow or broken consumer and disconnected rather than allowed to block
// the sender or grow the queue without bound.
func (c *Client) Enqueue(env Envelope) {
	select {
	case c.outbound <- env:
	case <-c.stop:
	default:
		c.Close()
	}
}

// Close tears down this connection: pending writers are signaled to stop
// and the underlying transport connection is closed, which in turn
// unblocks any in-progress read so the transport's own cleanup can run.
// Close is idempotent and safe to call concurrently.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.stop)
		if c.closeConn != nil {
			c.closeConn()
		}
	})
}
