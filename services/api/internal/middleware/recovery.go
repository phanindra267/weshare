package middleware

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/phanindra267/weshare/services/api/internal/httpx"
)

// Recovery converts panics into a logged 500 response instead of crashing the
// process, and never leaks stack traces to clients.
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		logger.Error("panic recovered",
			slog.String("request_id", httpx.RequestIDFromContext(c.Request.Context())),
			slog.String("path", c.Request.URL.Path),
			slog.Any("panic", recovered),
		)
		httpx.Error(c, httpx.Internal(nil))
	})
}
