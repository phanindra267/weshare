package notifications

// EventCreated is published to the recipient's live sockets when a notification
// is stored. It is part of the wire contract documented in
// packages/contracts/websocket-events.md.
const EventCreated = "notification.created"

// CreatedEvent is the payload of a notification.created frame. `Notification`
// is the standard notification shape, with the actor's public profile embedded.
type CreatedEvent struct {
	Type         string               `json:"type"`
	Notification NotificationResponse `json:"notification"`
}
