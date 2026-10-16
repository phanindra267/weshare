package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS applies an allow-list based CORS policy. Credentials are permitted so
// that HTTP-only session cookies work; therefore a wildcard origin is only
// honored when it is the sole configured value (and never in production, as
// enforced by config validation).
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowAll := len(allowedOrigins) == 1 && allowedOrigins[0] == "*"

	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[strings.ToLower(strings.TrimSpace(origin))] = struct{}{}
	}

	const (
		methods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
		headers = "Content-Type, Authorization, X-Request-Id, X-CSRF-Token"
	)

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			_, ok := allowed[strings.ToLower(origin)]
			switch {
			case allowAll && !strings.Contains(origin, "*"):
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Add("Vary", "Origin")
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			case ok:
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Add("Vary", "Origin")
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			}
		}

		if c.Request.Method == http.MethodOptions {
			if c.GetHeader("Access-Control-Request-Method") != "" {
				c.Writer.Header().Set("Access-Control-Allow-Methods", methods)
				c.Writer.Header().Set("Access-Control-Allow-Headers", headers)
				c.Writer.Header().Set("Access-Control-Max-Age", "600")
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
