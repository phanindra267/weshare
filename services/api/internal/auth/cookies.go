package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// newCSRFToken returns a random 256-bit double-submit CSRF token.
func newCSRFToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate csrf token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// sameSite maps the validated config value to net/http's enum.
func (s *Service) sameSite() http.SameSite {
	switch s.cfg.CookieSameSite {
	case "Strict":
		return http.SameSiteStrictMode
	case "None":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

// setSessionCookies writes the HTTP-only session cookie and the
// JavaScript-readable double-submit CSRF cookie.
func (s *Service) setSessionCookies(c *gin.Context, token, csrf string) {
	maxAge := int(s.cfg.SessionTTL.Seconds())

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     s.cfg.SessionCookieName,
		Value:    token,
		Path:     s.cfg.CookiePath,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: s.sameSite(),
	})

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     s.cfg.CSRFCookieName,
		Value:    csrf,
		Path:     s.cfg.CookiePath,
		MaxAge:   maxAge,
		HttpOnly: false, // must be readable by the SPA to echo in the header
		Secure:   s.cfg.CookieSecure,
		SameSite: s.sameSite(),
	})
}

// clearSessionCookies expires both cookies.
func (s *Service) clearSessionCookies(c *gin.Context) {
	for _, name := range []string{s.cfg.SessionCookieName, s.cfg.CSRFCookieName} {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     s.cfg.CookiePath,
			MaxAge:   -1,
			HttpOnly: name == s.cfg.SessionCookieName,
			Secure:   s.cfg.CookieSecure,
			SameSite: s.sameSite(),
		})
	}
}

// sessionToken reads the raw session token from the request cookie.
func (s *Service) sessionToken(c *gin.Context) string {
	v, err := c.Cookie(s.cfg.SessionCookieName)
	if err != nil {
		return ""
	}
	return v
}
