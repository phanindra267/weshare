package realtime

import (
	"errors"

	"github.com/phanindra267/weshare/services/api/internal/calls"
	"github.com/phanindra267/weshare/services/api/internal/messaging"
)

// Error codes emitted on the socket `error` event. They mirror the REST error
// codes so a client can handle both transports with one mapping.
const (
	CodeValidation = "VALIDATION_ERROR"
	CodeNotFound   = "NOT_FOUND"
	CodeForbidden  = "FORBIDDEN"
	CodeRateLimit  = "RATE_LIMITED"
	CodeInternal   = "INTERNAL_ERROR"
)

// classifyError maps a messaging domain error onto a socket error code and a
// client-safe message.
func classifyError(err error) (code, message string) {
	var invalid *messaging.ValidationError
	switch {
	case errors.As(err, &invalid):
		return CodeValidation, "Please fix the highlighted fields."
	case errors.Is(err, messaging.ErrConversationNotFound),
		errors.Is(err, messaging.ErrNotMember):
		return CodeNotFound, "Conversation not found."
	case errors.Is(err, messaging.ErrPeerNotFound):
		return CodeNotFound, "That user does not exist."
	case errors.Is(err, messaging.ErrSelfConversation),
		errors.Is(err, messaging.ErrInvalidSequence):
		return CodeValidation, "The request is invalid."
	case errors.Is(err, messaging.ErrBlocked):
		return CodeForbidden, "You cannot message this user."
	case errors.Is(err, messaging.ErrRateLimited):
		return CodeRateLimit, "You are sending messages too quickly."
	case errors.Is(err, messaging.ErrEmptyMessage),
		errors.Is(err, messaging.ErrMessageTooLarge),
		errors.Is(err, messaging.ErrUnsupportedType):
		return CodeValidation, "The message is invalid."
	default:
		return CodeInternal, "An unexpected error occurred."
	}
}

// classifyCallError maps a calls domain error onto a socket error code and a
// client-safe message.
func classifyCallError(err error) (code, message string) {
	var invalid *calls.ValidationError
	switch {
	case errors.As(err, &invalid):
		return CodeValidation, "Please fix the highlighted fields."
	case errors.Is(err, calls.ErrCallNotFound):
		return CodeNotFound, "That call is no longer active."
	case errors.Is(err, calls.ErrNotParticipant):
		return CodeForbidden, "You are not part of this call."
	case errors.Is(err, calls.ErrNotConnected):
		return CodeForbidden, "You can only call your contacts."
	case errors.Is(err, calls.ErrBusy):
		return CodeForbidden, "You are already in a call."
	case errors.Is(err, calls.ErrSelfCall):
		return CodeValidation, "The request is invalid."
	case errors.Is(err, calls.ErrCallFull):
		return CodeValidation, "This call is full."
	default:
		return CodeInternal, "An unexpected error occurred."
	}
}
