package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// Machine-readable error codes returned in the `error.code` field.
const (
	CodeValidation     = "VALIDATION_ERROR"
	CodeUnauthorized   = "UNAUTHORIZED"
	CodeForbidden      = "FORBIDDEN"
	CodeNotFound       = "NOT_FOUND"
	CodeConflict       = "CONFLICT"
	CodeRateLimited    = "RATE_LIMITED"
	CodePayloadTooBig  = "PAYLOAD_TOO_LARGE"
	CodeUnsupported    = "UNSUPPORTED_MEDIA_TYPE"
	CodeInternal       = "INTERNAL_ERROR"
	CodeNotImplemented = "NOT_IMPLEMENTED"
)

// AppError is a transport-aware error. Client-safe fields (Code, Message,
// Details) are serialized; the wrapped cause is logged but never exposed.
type AppError struct {
	Status  int
	Code    string
	Message string
	Details any
	cause   error
}

func (e *AppError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error { return e.cause }

// Wrap attaches an internal cause for logging without exposing it to clients.
func (e *AppError) Wrap(cause error) *AppError {
	e.cause = cause
	return e
}

// WithDetails attaches client-visible structured details.
func (e *AppError) WithDetails(details any) *AppError {
	e.Details = details
	return e
}

func newAppError(status int, code, message string) *AppError {
	return &AppError{Status: status, Code: code, Message: message}
}

// BadRequest returns a 400 validation/generic client error.
func BadRequest(message string) *AppError {
	return newAppError(http.StatusBadRequest, CodeValidation, message)
}

// Validation returns a 422 validation error with details.
func Validation(message string, details any) *AppError {
	return newAppError(http.StatusUnprocessableEntity, CodeValidation, message).WithDetails(details)
}

// Unauthorized returns a 401 error.
func Unauthorized(message string) *AppError {
	if message == "" {
		message = "Authentication is required."
	}
	return newAppError(http.StatusUnauthorized, CodeUnauthorized, message)
}

// Forbidden returns a 403 error.
func Forbidden(message string) *AppError {
	if message == "" {
		message = "You do not have permission to perform this action."
	}
	return newAppError(http.StatusForbidden, CodeForbidden, message)
}

// NotFound returns a 404 error.
func NotFound(message string) *AppError {
	if message == "" {
		message = "The requested resource was not found."
	}
	return newAppError(http.StatusNotFound, CodeNotFound, message)
}

// Conflict returns a 409 error.
func Conflict(message string) *AppError {
	return newAppError(http.StatusConflict, CodeConflict, message)
}

// RateLimited returns a 429 error.
func RateLimited(message string) *AppError {
	if message == "" {
		message = "Too many requests. Please slow down."
	}
	return newAppError(http.StatusTooManyRequests, CodeRateLimited, message)
}

// Internal returns a 500 error. The cause is for logs only.
func Internal(cause error) *AppError {
	return newAppError(http.StatusInternalServerError, CodeInternal,
		"An unexpected error occurred.").Wrap(cause)
}

// NotImplemented marks a feature that is deliberately not yet available.
func NotImplemented(message string) *AppError {
	if message == "" {
		message = "This feature is not available yet."
	}
	return newAppError(http.StatusNotImplemented, CodeNotImplemented, message)
}

// AsAppError extracts an *AppError from err, or wraps it as an internal error.
func AsAppError(err error) *AppError {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return newAppError(http.StatusRequestTimeout, "REQUEST_TIMEOUT",
			"The request took too long to complete.")
	}
	return Internal(err)
}
