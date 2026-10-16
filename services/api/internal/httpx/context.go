// Package httpx contains transport-level helpers: the standard JSON response
// envelope, the application error type, and request-scoped context values.
//
// Handlers never write raw status codes for errors; they return an *AppError
// which the central error middleware renders consistently.
package httpx

import (
	"context"
)

type contextKey string

const requestIDKey contextKey = "request_id"

// WithRequestID stores the request ID in the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFromContext returns the request ID, or "" when absent.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}
