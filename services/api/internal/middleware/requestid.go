// Package middleware holds Gin middleware: request IDs, structured request
// logging, panic recovery, CORS, and centralized error rendering.
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/httpx"
)

// RequestID assigns each request a unique ID (honoring an inbound header),
// exposes it on the response, and stores it in the request context for logs
// and error responses.
func RequestID(header string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(header)
		if id == "" {
			id = uuid.NewString()
		}
		c.Writer.Header().Set(header, id)
		ctx := httpx.WithRequestID(c.Request.Context(), id)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
