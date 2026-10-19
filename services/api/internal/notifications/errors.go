package notifications

import (
	"errors"

	"github.com/google/uuid"
)

// ErrNotFound indicates the notification does not exist or does not belong to
// the caller. The two are deliberately indistinguishable so the endpoint cannot
// be used to probe for other users' notification ids.
var ErrNotFound = errors.New("notification not found")

// ValidationError reports one invalid field on an input.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + " " + e.Message }

// validateInput checks the fields required to store a notification.
func validateInput(in Input) *ValidationError {
	if in.UserID == uuid.Nil {
		return &ValidationError{Field: "user_id", Message: "is required"}
	}
	switch in.Type {
	case TypeContactRequest, TypeContactAccepted, TypeMomentLike,
		TypeMomentComment, TypeMissedCall:
	default:
		return &ValidationError{Field: "type", Message: "is not a known notification type"}
	}
	return nil
}
