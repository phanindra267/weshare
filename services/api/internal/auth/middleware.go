package auth

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/phanindra267/weshare/services/api/internal/httpx"
)

const (
	userContextKey    = "auth.current_user"
	sessionContextKey = "auth.current_session"
)

// CurrentUser returns the authenticated user stored by the auth middleware.
func CurrentUser(c *gin.Context) (User, bool) {
	v, ok := c.Get(userContextKey)
	if !ok {
		return User{}, false
	}
	u, ok := v.(User)
	return u, ok
}

// CurrentSession returns the authenticated session stored by the middleware.
func CurrentSession(c *gin.Context) (Session, bool) {
	v, ok := c.Get(sessionContextKey)
	if !ok {
		return Session{}, false
	}
	s, ok := v.(Session)
	return s, ok
}

// OptionalAuth attaches the user/session when a valid session cookie is
// present and otherwise continues unauthenticated.
func (s *Service) OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		s.authenticate(c)
		c.Next()
	}
}

// RequireAuth rejects the request with 401 unless a valid session is present.
func (s *Service) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !s.authenticate(c) {
			httpx.Error(c, httpx.Unauthorized(""))
			c.Abort()
			return
		}
		c.Next()
	}
}

// authenticate resolves the session cookie and stores the user/session on the
// gin context. It reports whether a user was attached.
func (s *Service) authenticate(c *gin.Context) bool {
	token := s.sessionToken(c)
	if token == "" {
		return false
	}

	session, err := s.store.LookupSession(c.Request.Context(), token)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrInvalidToken) {
			// Stale cookie: clear it so the browser stops sending it.
			s.clearSessionCookies(c)
		}
		return false
	}

	user, err := s.store.GetUserByID(c.Request.Context(), session.UserID)
	if err != nil {
		return false
	}

	c.Set(userContextKey, user)
	c.Set(sessionContextKey, session)
	return true
}
