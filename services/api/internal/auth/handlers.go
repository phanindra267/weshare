package auth

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/httpx"
)

// Handler exposes the /auth HTTP surface.
type Handler struct {
	svc *Service
}

// NewHandler builds an auth Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RequireAuth exposes the session middleware so other feature routers can
// guard their own routes.
func (h *Handler) RequireAuth() gin.HandlerFunc { return h.svc.RequireAuth() }

// userResponse maps a user, deriving presence from last_seen_at against the
// configured staleness window instead of trusting the stored boolean.
func (h *Handler) userResponse(u User, includeEmail bool) UserResponse {
	resp := toUserResponse(u, includeEmail)
	resp.Online = h.svc.Config().IsOnline(u.LastSeenAt)
	return resp
}

// RegisterRoutes mounts the auth routes under the given group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/auth")

	g.POST("/register", rateLimit(h.svc.registerLimiter, clientIP), h.Register)
	g.POST("/login", rateLimit(h.svc.loginLimiter, clientIP), h.Login)

	authed := g.Group("", h.svc.RequireAuth())
	authed.POST("/logout", h.Logout)
	authed.GET("/me", h.Me)
	authed.POST("/password",
		rateLimit(h.svc.passwordLimiter, clientIP), h.ChangePassword)
	authed.GET("/sessions", h.ListSessions)
	authed.DELETE("/sessions", h.RevokeOtherSessions)
	authed.DELETE("/sessions/:id", h.RevokeSession)
}

type registerRequest struct {
	Email       string `json:"email"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

// Register handles POST /auth/register.
func (h *Handler) Register(c *gin.Context) {
	var body registerRequest
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}

	in := RegisterInput{
		Email:       body.Email,
		Username:    body.Username,
		Password:    body.Password,
		DisplayName: body.DisplayName,
	}
	if fields := in.Validate(h.svc.Config().MinPasswordLength); len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.", gin.H{"fields": fields}))
		return
	}

	result, err := h.svc.Register(c.Request.Context(), in, meta(c))
	if err != nil {
		switch {
		case errors.Is(err, ErrEmailTaken):
			httpx.Error(c, httpx.Conflict("That email is already registered."))
		case errors.Is(err, ErrUsernameTaken):
			httpx.Error(c, httpx.Conflict("That username is already taken."))
		default:
			httpx.Error(c, httpx.AsAppError(err))
		}
		return
	}

	h.svc.setSessionCookies(c, result.Token, result.CSRF)
	httpx.Created(c, h.userResponse(result.User, true))
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login handles POST /auth/login.
func (h *Handler) Login(c *gin.Context) {
	var body loginRequest
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}

	in := LoginInput{Email: body.Email, Password: body.Password}
	if fields := in.Validate(); len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.", gin.H{"fields": fields}))
		return
	}

	result, err := h.svc.Login(c.Request.Context(), in, meta(c))
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			httpx.Error(c, httpx.Unauthorized("Invalid email or password."))
		case errors.Is(err, ErrAccountLocked):
			httpx.Error(c, httpx.RateLimited("Too many failed attempts. Try again later."))
		default:
			httpx.Error(c, httpx.AsAppError(err))
		}
		return
	}

	h.svc.setSessionCookies(c, result.Token, result.CSRF)
	httpx.OK(c, h.userResponse(result.User, true))
}

// Logout handles POST /auth/logout.
func (h *Handler) Logout(c *gin.Context) {
	if err := h.svc.Logout(c.Request.Context(), h.svc.sessionToken(c)); err != nil {
		httpx.Error(c, httpx.AsAppError(err))
		return
	}
	h.svc.clearSessionCookies(c)
	httpx.NoContent(c)
}

// Me handles GET /auth/me.
func (h *Handler) Me(c *gin.Context) {
	user, ok := CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	httpx.OK(c, h.userResponse(user, true))
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword handles POST /auth/password.
func (h *Handler) ChangePassword(c *gin.Context) {
	var body changePasswordRequest
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}

	fields := map[string]string{}
	switch {
	case body.CurrentPassword == "":
		fields["current_password"] = "Current password is required."
	case body.NewPassword == "":
		fields["new_password"] = "New password is required."
	case len(body.NewPassword) < h.svc.Config().MinPasswordLength:
		fields["new_password"] = "Password must be at least " +
			strconv.Itoa(h.svc.Config().MinPasswordLength) + " characters."
	case len(body.NewPassword) > maxPasswordLength:
		fields["new_password"] = "Password is too long."
	}
	if len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.", gin.H{"fields": fields}))
		return
	}

	user, _ := CurrentUser(c)
	session, _ := CurrentSession(c)

	err := h.svc.ChangePassword(c.Request.Context(), user.ID, session.ID, body.CurrentPassword, body.NewPassword)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			httpx.Error(c, httpx.Validation("Current password is incorrect.",
				gin.H{"fields": map[string]string{"current_password": "Incorrect password."}}))
		default:
			httpx.Error(c, httpx.AsAppError(err))
		}
		return
	}

	httpx.NoContent(c)
}

// ListSessions handles GET /auth/sessions.
func (h *Handler) ListSessions(c *gin.Context) {
	user, _ := CurrentUser(c)
	session, _ := CurrentSession(c)

	sessions, err := h.svc.ListSessions(c.Request.Context(), user.ID, session.ID)
	if err != nil {
		httpx.Error(c, httpx.AsAppError(err))
		return
	}

	items := make([]SessionResponse, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, toSessionResponse(s))
	}
	httpx.OK(c, httpx.NewPaginated(items, ""))
}

// RevokeSession handles DELETE /auth/sessions/:id.
func (h *Handler) RevokeSession(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, httpx.NotFound("Session not found."))
		return
	}

	user, _ := CurrentUser(c)
	if err := h.svc.RevokeSession(c.Request.Context(), id, user.ID); err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			httpx.Error(c, httpx.NotFound("Session not found."))
			return
		}
		httpx.Error(c, httpx.AsAppError(err))
		return
	}
	httpx.NoContent(c)
}

// RevokeOtherSessions handles DELETE /auth/sessions.
func (h *Handler) RevokeOtherSessions(c *gin.Context) {
	user, _ := CurrentUser(c)
	session, _ := CurrentSession(c)

	if err := h.svc.RevokeOtherSessions(c.Request.Context(), user.ID, session.ID); err != nil {
		httpx.Error(c, httpx.AsAppError(err))
		return
	}
	httpx.NoContent(c)
}

func meta(c *gin.Context) RequestMeta {
	return RequestMeta{
		UserAgent: c.Request.UserAgent(),
		IP:        c.ClientIP(),
	}
}

func clientIP(c *gin.Context) string { return c.ClientIP() }
