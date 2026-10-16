// Package contacts implements the social graph: friend requests, accepted
// friendships, blocks, and presence heartbeats.
package contacts

import "errors"

// Domain errors returned by the contacts service. Handlers translate these
// into HTTP responses via httpx; persistence-level errors are wrapped and
// logged rather than exposed.
var (
	// ErrUserNotFound is returned when the target of an action does not exist.
	ErrUserNotFound = errors.New("user not found")
	// ErrCannotAddSelf is returned when a user tries to befriend themselves.
	ErrCannotAddSelf = errors.New("cannot add yourself")
	// ErrRequestNotFound is returned when a request id does not resolve.
	ErrRequestNotFound = errors.New("contact request not found")
	// ErrRequestAlreadySent is returned when a pending request already exists.
	ErrRequestAlreadySent = errors.New("contact request already sent")
	// ErrAlreadyFriends is returned when the pair is already connected.
	ErrAlreadyFriends = errors.New("already connected")
	// ErrBlocked is returned when either side has blocked the other.
	ErrBlocked = errors.New("user is blocked")
	// ErrNotRecipient is returned when someone other than the addressee acts
	// on a request.
	ErrNotRecipient = errors.New("not the recipient of this request")
	// ErrRequestNotPending is returned when a request is no longer actionable.
	ErrRequestNotPending = errors.New("contact request is not pending")
	// ErrNotFriends is returned when removing a non-contact.
	ErrNotFriends = errors.New("not connected")
	// ErrNotBlocked is returned when unblocking someone who is not blocked.
	ErrNotBlocked = errors.New("user is not blocked")
)
