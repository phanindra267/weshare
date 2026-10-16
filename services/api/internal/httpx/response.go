package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorBody is the standard error response payload.
type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	Details   any    `json:"details,omitempty"`
}

// ErrorEnvelope wraps ErrorBody under the "error" key.
type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// DataEnvelope wraps successful single-resource responses under "data".
type DataEnvelope struct {
	Data any `json:"data"`
}

// JSON writes a raw JSON response with the given status.
func JSON(c *gin.Context, status int, payload any) {
	c.JSON(status, payload)
}

// Data writes a 200 (or given status) response wrapped in the data envelope.
func Data(c *gin.Context, status int, data any) {
	c.JSON(status, DataEnvelope{Data: data})
}

// OK writes a 200 data-envelope response.
func OK(c *gin.Context, data any) {
	Data(c, http.StatusOK, data)
}

// Created writes a 201 data-envelope response.
func Created(c *gin.Context, data any) {
	Data(c, http.StatusCreated, data)
}

// NoContent writes a 204 response.
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// Error renders err using the standard error envelope. It records the error
// for the logging middleware via c.Errors.
func Error(c *gin.Context, err error) {
	appErr := AsAppError(err)

	// Preserve the underlying cause for structured logs.
	if cause := appErr.Unwrap(); cause != nil {
		_ = c.Error(cause)
	} else {
		_ = c.Error(appErr)
	}

	c.AbortWithStatusJSON(appErr.Status, ErrorEnvelope{
		Error: ErrorBody{
			Code:      appErr.Code,
			Message:   appErr.Message,
			RequestID: RequestIDFromContext(c.Request.Context()),
			Details:   appErr.Details,
		},
	})
}

// BindJSON decodes the request body into dst and returns a validation
// AppError on failure so callers can simply `return httpx.BindJSON(...)`.
func BindJSON(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		return Validation("The request body is invalid.", gin.H{
			"reason": err.Error(),
		})
	}
	return nil
}

// Paginated is the cursor-pagination envelope.
type Paginated[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
}

// NewPaginated normalises nil slices to empty arrays for stable JSON shapes.
func NewPaginated[T any](items []T, nextCursor string) Paginated[T] {
	if items == nil {
		items = []T{}
	}
	return Paginated[T]{Items: items, NextCursor: nextCursor}
}
