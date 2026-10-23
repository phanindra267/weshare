package messaging

// Event type names shared by the WebSocket transport and REST fan-out. They are
// the wire contract documented in packages/contracts/websocket-events.md.
const (
	EventHello          = "hello"
	EventPong           = "pong"
	EventMessageCreated = "message.created"
	EventMessageAck     = "message.ack"
	EventMessageReceipt = "message.receipt"
	EventTyping         = "typing"
	EventPresence       = "presence"
	EventSync           = "sync.result"
	EventError          = "error"
	// Group lifecycle events (Phase 4).
	EventConversationUpdated = "conversation.updated"
	EventMemberChanged       = "member.changed"
	// Advanced-messaging events (Phase 4).
	EventMessageUpdated  = "message.updated"
	EventMessageDeleted  = "message.deleted"
	EventMessageReaction = "message.reaction"
	EventMessagePinned   = "message.pinned"
)

// Client-originated event types accepted on the realtime socket.
const (
	ClientPing        = "ping"
	ClientMessageSend = "message.send"
	ClientTyping      = "typing"
	ClientRead        = "read"
	ClientSync        = "sync"
)

// HelloEvent is the first frame a client receives after the socket opens,
// letting it confirm identity and (re)sync.
type HelloEvent struct {
	Type       string `json:"type"`
	UserID     string `json:"user_id"`
	ServerTime string `json:"server_time"`
}

// MessageCreatedEvent announces a new message to every member, including the
// sender (so a second tab or a reconnected client converges).
type MessageCreatedEvent struct {
	Type           string          `json:"type"`
	EventID        string          `json:"event_id"`
	ConversationID string          `json:"conversation_id"`
	RequestID      string          `json:"request_id,omitempty"`
	Message        MessageResponse `json:"message"`
}

// MessageAckEvent is the direct acknowledgement returned to the sender's socket
// for a `message.send`, echoing the client id for optimistic-UI reconciliation.
type MessageAckEvent struct {
	Type           string          `json:"type"`
	ConversationID string          `json:"conversation_id"`
	RequestID      string          `json:"request_id,omitempty"`
	Message        MessageResponse `json:"message"`
}

// MessageReceiptEvent reports a delivery or read advance by one member. Senders
// use it to move their ticks from sent → delivered → read.
type MessageReceiptEvent struct {
	Type           string `json:"type"`
	ConversationID string `json:"conversation_id"`
	UserID         string `json:"user_id"`
	Status         string `json:"status"`
	UpToSequence   int64  `json:"up_to_sequence"`
}

// TypingEvent relays a transient typing indicator; it is never persisted.
type TypingEvent struct {
	Type           string `json:"type"`
	ConversationID string `json:"conversation_id"`
	UserID         string `json:"user_id"`
	Typing         bool   `json:"typing"`
}

// PresenceEvent reports a contact coming online or going offline.
type PresenceEvent struct {
	Type   string `json:"type"`
	UserID string `json:"user_id"`
	Online bool   `json:"online"`
}

// ErrorEvent reports a per-request failure over the socket without closing it.
type ErrorEvent struct {
	Type      string `json:"type"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// SyncResultEvent answers a `sync` request with messages newer than the client's
// last-seen sequence, so a reconnecting client can catch up without the REST
// history endpoint.
type SyncResultEvent struct {
	Type           string            `json:"type"`
	ConversationID string            `json:"conversation_id"`
	RequestID      string            `json:"request_id,omitempty"`
	Messages       []MessageResponse `json:"messages"`
	NextCursor     string            `json:"next_cursor"`
}

// ConversationUpdatedEvent reports that a group's metadata changed (title,
// avatar) or that its membership count moved, so clients can refresh the list
// and open header without re-fetching everything.
type ConversationUpdatedEvent struct {
	Type           string  `json:"type"`
	ConversationID string  `json:"conversation_id"`
	Title          *string `json:"title"`
	AvatarURL      *string `json:"avatar_url"`
	MemberCount    int64   `json:"member_count"`
}

// Member action values carried by MemberChangedEvent.
const (
	MemberActionAdded   = "added"
	MemberActionRemoved = "removed"
	MemberActionLeft    = "left"
	MemberActionRole    = "role"
)

// MemberChangedEvent reports a membership change in a group: a member was added,
// removed, left, or had their role changed.
type MemberChangedEvent struct {
	Type           string `json:"type"`
	ConversationID string `json:"conversation_id"`
	UserID         string `json:"user_id"`
	Role           string `json:"role"`
	Action         string `json:"action"`
}

// MessageUpdatedEvent announces an edit to a message's body.
type MessageUpdatedEvent struct {
	Type           string          `json:"type"`
	ConversationID string          `json:"conversation_id"`
	Message        MessageResponse `json:"message"`
}

// MessageDeletedEvent announces that a message was tombstoned. The embedded
// message carries the redacted (deleted) representation.
type MessageDeletedEvent struct {
	Type           string          `json:"type"`
	ConversationID string          `json:"conversation_id"`
	MessageID      string          `json:"message_id"`
	Message        MessageResponse `json:"message"`
}

// MessageReactionEvent reports the full reaction set for one message after a
// change, so clients can replace rather than reconcile.
type MessageReactionEvent struct {
	Type           string             `json:"type"`
	ConversationID string             `json:"conversation_id"`
	MessageID      string             `json:"message_id"`
	ActorID        string             `json:"actor_id"`
	Reactions      []ReactionResponse `json:"reactions"`
}

// MessagePinnedEvent reports a pin or unpin of a message.
type MessagePinnedEvent struct {
	Type           string `json:"type"`
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	ActorID        string `json:"actor_id"`
	Pinned         bool   `json:"pinned"`
}
