package feed

// EventMomentCreated is published to a user's friends over the realtime socket
// when the user posts a moment. It is part of the wire contract documented in
// packages/contracts/websocket-events.md.
const EventMomentCreated = "moment.created"

// MomentCreatedEvent is the payload of a moment.created event. `Moment` is the
// standard moment shape, with `liked_by_me` referring to the viewer.
type MomentCreatedEvent struct {
	Type   string         `json:"type"`
	Moment MomentResponse `json:"moment"`
}
