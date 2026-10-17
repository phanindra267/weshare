// Package feed implements Moments: short friends-only posts with media, likes,
// and comments. It is the social layer that sits on top of the contacts graph.
package feed

import "errors"

// Domain errors returned by the feed service. Handlers translate these into
// HTTP responses via httpx; persistence errors are wrapped for logs instead.
var (
	// ErrMomentNotFound is returned when a moment id does not resolve, or when
	// the caller is not allowed to see it (the two are deliberately
	// indistinguishable so a private moment cannot be probed for existence).
	ErrMomentNotFound = errors.New("moment not found")
	// ErrCommentNotFound is returned when a comment id does not resolve.
	ErrCommentNotFound = errors.New("comment not found")
	// ErrNotAuthor is returned when a caller tries to delete someone else's
	// moment.
	ErrNotAuthor = errors.New("not the author of this moment")
	// ErrCannotView is returned when a caller may not read a moment's comments.
	ErrCannotView = errors.New("cannot view this moment")
	// ErrEmptyMoment is returned when a moment has neither text nor media.
	ErrEmptyMoment = errors.New("a moment needs text or media")
)

// ValidationError carries a field-level message so the handler can render the
// standard VALIDATION_ERROR shape with a `details` object.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }
