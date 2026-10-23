// Package messaging implements one-to-one messaging: conversations, history,
// delivery/read state, and the service shared by the REST surface and the
// realtime WebSocket transport.
package messaging

import "errors"

// Domain errors returned by the messaging service. Handlers translate these
// onto HTTP responses; persistence-level errors are wrapped and logged.
var (
	// ErrConversationNotFound is returned when a conversation id does not
	// resolve for the caller.
	ErrConversationNotFound = errors.New("conversation not found")
	// ErrNotMember is returned when the caller does not belong to a
	// conversation. It exists so membership failures cannot be used to probe
	// for ids that belong to other people.
	ErrNotMember = errors.New("not a member of this conversation")
	// ErrPeerNotFound is returned when starting a conversation with a user who
	// does not exist.
	ErrPeerNotFound = errors.New("user not found")
	// ErrSelfConversation is returned when a user tries to message themselves.
	ErrSelfConversation = errors.New("cannot message yourself")
	// ErrBlocked is returned when either side has blocked the other, or when
	// the pair is not connected.
	ErrBlocked = errors.New("you cannot message this user")
	// ErrEmptyMessage is returned for a message with no content.
	ErrEmptyMessage = errors.New("message is empty")
	// ErrMessageTooLarge is returned when content exceeds the size bound.
	ErrMessageTooLarge = errors.New("message is too large")
	// ErrUnsupportedType is returned for a message_type outside the closed set.
	ErrUnsupportedType = errors.New("unsupported message type")
	// ErrRateLimited is returned when a user sends messages too quickly.
	ErrRateLimited = errors.New("too many messages")
	// ErrInvalidSequence is returned when a read position is not usable.
	ErrInvalidSequence = errors.New("invalid sequence")
	// ErrNotGroup is returned when a group-only operation targets a direct
	// conversation.
	ErrNotGroup = errors.New("not a group conversation")
	// ErrForbidden is returned when the caller's role does not permit the
	// action they attempted.
	ErrForbidden = errors.New("not permitted")
	// ErrNotConnected is returned when adding a user the caller is not
	// connected to as an accepted contact.
	ErrNotConnected = errors.New("not connected")
	// ErrOwnerImmutable is returned when an action would change the owner.
	ErrOwnerImmutable = errors.New("the owner cannot be changed this way")
	// ErrMessageNotFound is returned when a message id does not resolve within
	// a conversation (Phase 4).
	ErrMessageNotFound = errors.New("message not found")
	// ErrEditWindowClosed is returned when editing a message after the allowed
	// window has elapsed (Phase 4).
	ErrEditWindowClosed = errors.New("this message can no longer be edited")
	// ErrNotEditableType is returned when editing a message whose type has no
	// editable body (media and system messages) (Phase 4).
	ErrNotEditableType = errors.New("this message cannot be edited")
	// ErrCannotEditDeleted is returned when editing an already deleted message.
	ErrCannotEditDeleted = errors.New("this message was deleted")
	// ErrCannotForwardDeleted is returned when forwarding a deleted message.
	ErrCannotForwardDeleted = errors.New("this message was deleted")
	// ErrNotReactable is returned when reacting to a message that cannot carry
	// reactions (system messages).
	ErrNotReactable = errors.New("this message cannot be reacted to")
	// ErrEmptySearch is returned when an in-chat search query is empty.
	ErrEmptySearch = errors.New("search query is empty")
	// ErrInvalidEmoji is returned when a reaction emoji is out of bounds.
	ErrInvalidEmoji = errors.New("invalid reaction")
	// ErrPinForbidden is returned when the caller's role does not permit
	// pinning in a group.
	ErrPinForbidden = errors.New("not permitted to pin messages")
	// ErrNotForwardable is returned when forwarding a system message.
	ErrNotForwardable = errors.New("this message cannot be forwarded")
)

// Conversation kinds.
const (
	ConversationKindDirect = "direct"
	ConversationKindGroup  = "group"
)

// Member roles within a group conversation.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// roleRank orders roles for "at least this privileged" checks.
func roleRank(role string) int {
	switch role {
	case RoleOwner:
		return 2
	case RoleAdmin:
		return 1
	default:
		return 0
	}
}

// ValidationError carries per-field problems so a handler can render a 422
// with a `details.fields` map, matching the rest of the API.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string { return "validation failed" }

// Message types accepted by the API (kept in sync with the CHECK constraint
// on messages.message_type).
const (
	MessageTypeText   = "text"
	MessageTypeImage  = "image"
	MessageTypeFile   = "file"
	MessageTypeAudio  = "audio"
	MessageTypeVideo  = "video"
	MessageTypeSystem = "system"
)

// Message statuses reported to a viewer for one message.
const (
	StatusSent      = "sent"
	StatusDelivered = "delivered"
	StatusRead      = "read"
	StatusReceived  = "received"
)
