package contacts

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/auth"
	"github.com/phanindra267/weshare/services/api/internal/config"
	"github.com/phanindra267/weshare/services/api/internal/httpx"
)

// Handler exposes the /contacts HTTP surface.
type Handler struct {
	svc *Service
	cfg *config.Config
	// requestLimiter throttles request creation per client IP. The limiter type
	// lives in auth for now; a shared home is a later refactor.
	requestLimiter *auth.Limiter
}

// NewHandler builds a contacts Handler.
func NewHandler(svc *Service, cfg *config.Config) *Handler {
	return &Handler{
		svc:            svc,
		cfg:            cfg,
		requestLimiter: auth.NewLimiter(30, 15*time.Minute),
	}
}

// RegisterRoutes mounts the contacts routes under the given group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	g := rg.Group("/contacts", requireAuth)

	g.GET("", h.ListContacts)
	g.GET("/requests", h.ListRequests)
	g.POST("/requests", rateLimit(h.requestLimiter), h.SendRequest)
	g.POST("/requests/:id/accept", h.AcceptRequest)
	g.POST("/requests/:id/decline", h.DeclineRequest)
	g.DELETE("/:id", h.RemoveContact)
	g.POST("/:id/block", h.BlockUser)
	g.DELETE("/:id/block", h.UnblockUser)
}

// rateLimit rejects requests over the limiter's allowance with 429. Keys are
// client IPs; this mirrors the (unexported) helper in auth.
func rateLimit(l *auth.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.Allow(c.ClientIP()) {
			httpx.Error(c, httpx.RateLimited("Too many requests. Please try again later."))
			c.Abort()
			return
		}
		c.Next()
	}
}

// online reports presence for a peer using the configured staleness window.
func (h *Handler) online(lastSeen *time.Time) bool { return h.cfg.IsOnline(lastSeen) }

// writeError maps a domain error onto the standard error envelope.
func writeError(c *gin.Context, err error, notFoundMessage string) {
	switch {
	case errors.Is(err, ErrUserNotFound), errors.Is(err, ErrRequestNotFound):
		httpx.Error(c, httpx.NotFound(notFoundMessage))
	case errors.Is(err, ErrNotRecipient):
		httpx.Error(c, httpx.Forbidden("This request does not belong to you."))
	case errors.Is(err, ErrBlocked):
		httpx.Error(c, httpx.Forbidden("You cannot connect with this user."))
	case errors.Is(err, ErrAlreadyFriends):
		httpx.Error(c, httpx.Conflict("You are already connected."))
	case errors.Is(err, ErrRequestAlreadySent):
		httpx.Error(c, httpx.Conflict("A contact request is already pending."))
	case errors.Is(err, ErrRequestNotPending):
		httpx.Error(c, httpx.Conflict("This request is no longer pending."))
	case errors.Is(err, ErrNotFriends):
		httpx.Error(c, httpx.NotFound("You are not connected with this user."))
	case errors.Is(err, ErrNotBlocked):
		httpx.Error(c, httpx.NotFound("This user is not blocked."))
	case errors.Is(err, ErrCannotAddSelf):
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": map[string]string{"user_id": "You cannot do that with your own account."}}))
	default:
		httpx.Error(c, httpx.AsAppError(err))
	}
}

// parseParam reads a UUID path parameter, answering with a 404 when malformed.
func parseParam(c *gin.Context, name, notFoundMessage string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		httpx.Error(c, httpx.NotFound(notFoundMessage))
		return uuid.Nil, false
	}
	return id, true
}

// listParams reads the optional `limit` and `cursor` query parameters.
func listParams(c *gin.Context) (limit int, after *uuid.UUID, ok bool) {
	fields := map[string]string{}

	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			fields["limit"] = "limit must be a positive integer."
		} else {
			limit = n
		}
	}
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			fields["cursor"] = "cursor must be a valid id."
		} else {
			after = &id
		}
	}

	if len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": fields}))
		return 0, nil, false
	}
	return limit, after, true
}

// ListContacts handles GET /contacts.
func (h *Handler) ListContacts(c *gin.Context) {
	limit, after, ok := listParams(c)
	if !ok {
		return
	}

	user, _ := auth.CurrentUser(c)
	items, next, err := h.svc.ListContacts(c.Request.Context(), user.ID, after, limit)
	if err != nil {
		writeError(c, err, "Contacts could not be loaded.")
		return
	}

	resp := make([]ContactResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toContactResponse(item, h.online(item.LastSeenAt)))
	}
	httpx.OK(c, httpx.NewPaginated(resp, next))
}

// ListRequests handles GET /contacts/requests?direction=incoming|outgoing.
func (h *Handler) ListRequests(c *gin.Context) {
	limit, after, ok := listParams(c)
	if !ok {
		return
	}

	incoming := true
	switch strings.ToLower(strings.TrimSpace(c.Query("direction"))) {
	case "", "incoming":
		incoming = true
	case "outgoing":
		incoming = false
	default:
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": map[string]string{"direction": "direction must be incoming or outgoing."}}))
		return
	}

	user, _ := auth.CurrentUser(c)
	items, next, err := h.svc.ListRequests(c.Request.Context(), user.ID, incoming, after, limit)
	if err != nil {
		writeError(c, err, "Requests could not be loaded.")
		return
	}

	resp := make([]ContactRequestResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toRequestResponse(item, incoming, h.online(item.LastSeenAt)))
	}
	httpx.OK(c, httpx.NewPaginated(resp, next))
}

// SendRequest handles POST /contacts/requests.
func (h *Handler) SendRequest(c *gin.Context) {
	var body sendRequestInput
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}
	if fields := body.Validate(); len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": fields}))
		return
	}
	target, _ := uuid.Parse(body.UserID)

	user, _ := auth.CurrentUser(c)
	req, peer, err := h.svc.SendRequest(c.Request.Context(), user.ID, target)
	if err != nil {
		writeError(c, err, "User not found.")
		return
	}

	resp := toSingleRequest(req, peer, DirectionOutgoing, h.online(peer.LastSeenAt))
	if req.Status == StatusAccepted {
		// A mutual add became a connection, so the resource already exists.
		httpx.OK(c, resp)
		return
	}
	httpx.Created(c, resp)
}

// AcceptRequest handles POST /contacts/requests/:id/accept.
func (h *Handler) AcceptRequest(c *gin.Context) {
	id, ok := parseParam(c, "id", "Contact request not found.")
	if !ok {
		return
	}

	user, _ := auth.CurrentUser(c)
	req, peer, err := h.svc.AcceptRequest(c.Request.Context(), id, user.ID)
	if err != nil {
		writeError(c, err, "Contact request not found.")
		return
	}
	httpx.OK(c, toSingleRequest(req, peer, DirectionIncoming, h.online(peer.LastSeenAt)))
}

// DeclineRequest handles POST /contacts/requests/:id/decline.
func (h *Handler) DeclineRequest(c *gin.Context) {
	id, ok := parseParam(c, "id", "Contact request not found.")
	if !ok {
		return
	}

	user, _ := auth.CurrentUser(c)
	if err := h.svc.DeclineRequest(c.Request.Context(), id, user.ID); err != nil {
		writeError(c, err, "Contact request not found.")
		return
	}
	httpx.NoContent(c)
}

// RemoveContact handles DELETE /contacts/:id where :id is the peer's user id.
func (h *Handler) RemoveContact(c *gin.Context) {
	peer, ok := parseParam(c, "id", "Contact not found.")
	if !ok {
		return
	}

	user, _ := auth.CurrentUser(c)
	if err := h.svc.RemoveContact(c.Request.Context(), user.ID, peer); err != nil {
		writeError(c, err, "Contact not found.")
		return
	}
	httpx.NoContent(c)
}

// BlockUser handles POST /contacts/:id/block where :id is the peer's user id.
func (h *Handler) BlockUser(c *gin.Context) {
	peer, ok := parseParam(c, "id", "User not found.")
	if !ok {
		return
	}

	user, _ := auth.CurrentUser(c)
	if err := h.svc.BlockUser(c.Request.Context(), user.ID, peer); err != nil {
		writeError(c, err, "User not found.")
		return
	}
	httpx.NoContent(c)
}

// UnblockUser handles DELETE /contacts/:id/block.
func (h *Handler) UnblockUser(c *gin.Context) {
	peer, ok := parseParam(c, "id", "User not found.")
	if !ok {
		return
	}

	user, _ := auth.CurrentUser(c)
	if err := h.svc.UnblockUser(c.Request.Context(), user.ID, peer); err != nil {
		writeError(c, err, "User not found.")
		return
	}
	httpx.NoContent(c)
}
