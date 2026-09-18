// Package realtime implements the KMJG Hub Server's authenticated WebSocket
// connection registry, per docs/ARCHITECTURE.md "Real-Time Communication"
// and "Presence and Work Status Architecture": "Online presence is
// determined automatically by the Server based on active authenticated
// Client connections."
//
// This package is deliberately transport-agnostic: it knows nothing about
// gorilla/websocket, HTTP upgrades, or heartbeat framing (those live in
// internal/httpapi, per docs/ARCHITECTURE.md "Server Internal Architecture":
// "HTTP handlers and WebSocket handlers should translate network requests
// and events into application operations rather than contain the core
// business rules themselves"). That keeps Hub and Client testable without a
// real network server, and keeps the connection registry reusable for any
// future event type (Tasks, Project Chat) beyond presence.
package realtime

import "encoding/json"

// ProtocolVersion identifies the current event envelope shape, per
// docs/ARCHITECTURE.md "Real-Time Protocol Versioning": "WebSocket
// communication must also have an identifiable protocol version or
// equivalent compatibility mechanism." Bumping this is a breaking-change
// signal for Clients; this checkpoint does not need multiple versions to
// coexist, just the field to exist.
const ProtocolVersion = 1

// Envelope is the wire shape of every event carried over the KMJG Hub
// WebSocket connection, matching docs/ARCHITECTURE.md's example:
//
//	{"type": "...", "data": {...}}
//
// Every event is explicitly typed and validated rather than routed through
// a generic untyped bus, per this checkpoint's protocol requirements, while
// the envelope itself stays generic enough that Tasks or Project Chat can
// introduce their own "type"/"data" pairs later without a redesign.
type Envelope struct {
	V    int             `json:"v"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// NewEnvelope marshals data as the payload of a Type event at the current
// ProtocolVersion.
func NewEnvelope(eventType string, data any) (Envelope, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{V: ProtocolVersion, Type: eventType, Data: raw}, nil
}
