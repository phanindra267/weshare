package calls

import "errors"

// Domain errors returned by the calls service. Handlers translate these into
// HTTP responses; the realtime transport maps them onto socket error frames.
var (
	// ErrNotConnected is returned when the caller and a callee are not accepted
	// contacts, or one has blocked the other. It is also used when a target is
	// not reachable, so call existence is not disclosed to strangers.
	ErrNotConnected = errors.New("users are not connected")
	// ErrBusy is returned when the caller is already on another call.
	ErrBusy = errors.New("you are already in a call")
	// ErrSelfCall is returned when a call has no recipient other than the caller.
	ErrSelfCall = errors.New("cannot call yourself")
	// ErrCallNotFound is returned when a call id does not resolve to an active call.
	ErrCallNotFound = errors.New("call not found")
	// ErrNotParticipant is returned when a user acts on a call they are not part of.
	ErrNotParticipant = errors.New("not a participant of this call")
	// ErrCallFull is returned when a call has reached the participant limit.
	ErrCallFull = errors.New("call has reached its participant limit")
)

// ValidationError carries a field-level message so the transport can render the
// standard VALIDATION_ERROR shape.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }
