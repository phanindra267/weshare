package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/phanindra267/weshare/services/api/internal/config"
	"github.com/phanindra267/weshare/services/api/internal/httpx"
)

// CSRF enforces the double-submit cookie pattern for authenticated,
// state-changing requests. A valid session cookie must be accompanied by a
// matching X-CSRF-Token header equal to the value of the CSRF cookie. Safe
// methods and unauthenticated requests (login/register) are exempt, which
// matches the threat model: CSRF only matters for ambient credentials.
func CSRF(cfg *config.Config) gin.HandlerFunc {
	tokenHeader := "X-CSRF-Token"

	return func(c *gin.Context) {
		if !cfg.CSRFEnabled || isSafeMethod(c.Request.Method) {
			c.Next()
			return
		}

		// Only enforce when a session cookie is present.
		if _, err := c.Cookie(cfg.SessionCookieName); err != nil {
			c.Next()
			return
		}

		cookieToken, err := c.Cookie(cfg.CSRFCookieName)
		if err != nil || cookieToken == "" {
			httpx.Error(c, httpx.Forbidden("Missing CSRF token."))
			c.Abort()
			return
		}

		headerToken := c.GetHeader(tokenHeader)
		if headerToken == "" ||
			subtle.ConstantTimeCompare([]byte(headerToken), []byte(cookieToken)) != 1 {
			httpx.Error(c, httpx.Forbidden("Invalid CSRF token."))
			c.Abort()
			return
		}

		c.Next()
	}
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}
