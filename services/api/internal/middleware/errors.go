package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/phanindra267/weshare/services/api/internal/httpx"
)

// ErrorHandler centralizes error rendering. Handlers may either call
// httpx.Error directly or attach errors via c.Error and return; any unrendered
// error is converted to the standard envelope here.
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}
		httpx.Error(c, c.Errors.Last().Err)
	}
}
