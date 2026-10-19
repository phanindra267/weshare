package search

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/phanindra267/weshare/services/api/internal/auth"
	"github.com/phanindra267/weshare/services/api/internal/config"
	"github.com/phanindra267/weshare/services/api/internal/httpx"
)

// Handler exposes the /search HTTP surface.
type Handler struct {
	svc *Service
	cfg *config.Config
}

// NewHandler builds a search Handler.
func NewHandler(svc *Service, cfg *config.Config) *Handler {
	return &Handler{svc: svc, cfg: cfg}
}

// RegisterRoutes mounts the search and discovery routes under the given group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	g := rg.Group("/search", requireAuth)
	g.GET("/messages", h.Messages)
	g.GET("/users", h.Users)
	g.GET("/conversations", h.Conversations)
	g.GET("/groups", h.Groups)

	d := rg.Group("/discover", requireAuth)
	d.GET("/people", h.DiscoverPeople)
}

func writeError(c *gin.Context, err error) {
	var verr *ValidationError
	if errors.As(err, &verr) {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": map[string]string{verr.Field: verr.Message}}))
		return
	}
	httpx.Error(c, httpx.AsAppError(err))
}

// limitParam parses an optional positive `limit` query parameter. A value of 0
// means "unset", letting the service apply its default.
func limitParam(c *gin.Context) (int, bool) {
	raw := strings.TrimSpace(c.Query("limit"))
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": map[string]string{"limit": "limit must be a positive integer."}}))
		return 0, false
	}
	return n, true
}

// Messages handles GET /search/messages.
func (h *Handler) Messages(c *gin.Context) {
	limit, ok := limitParam(c)
	if !ok {
		return
	}

	user, _ := auth.CurrentUser(c)
	hits, next, err := h.svc.Messages(c.Request.Context(), user.ID,
		c.Query("q"), c.Query("cursor"), limit)
	if err != nil {
		writeError(c, err)
		return
	}
	httpx.OK(c, httpx.NewPaginated(toMessageHitResponses(hits), next))
}

// Users handles GET /search/users.
func (h *Handler) Users(c *gin.Context) {
	limit, ok := limitParam(c)
	if !ok {
		return
	}

	user, _ := auth.CurrentUser(c)
	hits, next, err := h.svc.Profiles(c.Request.Context(), user.ID,
		c.Query("q"), c.Query("cursor"), limit)
	if err != nil {
		writeError(c, err)
		return
	}
	httpx.OK(c, httpx.NewPaginated(toProfileHitResponses(hits, h.cfg.IsOnline), next))
}

// Conversations handles GET /search/conversations.
func (h *Handler) Conversations(c *gin.Context) {
	h.conversations(c, "")
}

// Groups handles GET /search/groups (conversations restricted to groups).
func (h *Handler) Groups(c *gin.Context) {
	h.conversations(c, "group")
}

func (h *Handler) conversations(c *gin.Context, forcedKind string) {
	limit, ok := limitParam(c)
	if !ok {
		return
	}
	kind := forcedKind
	if kind == "" {
		kind = c.Query("kind")
	}

	user, _ := auth.CurrentUser(c)
	hits, next, err := h.svc.Conversations(c.Request.Context(), user.ID,
		c.Query("q"), kind, c.Query("cursor"), limit)
	if err != nil {
		writeError(c, err)
		return
	}
	httpx.OK(c, httpx.NewPaginated(toConversationHitResponses(hits), next))
}

// DiscoverPeople handles GET /discover/people.
func (h *Handler) DiscoverPeople(c *gin.Context) {
	limit, ok := limitParam(c)
	if !ok {
		return
	}
	user, _ := auth.CurrentUser(c)
	hits, next, err := h.svc.DiscoverPeople(c.Request.Context(), user.ID,
		c.Query("cursor"), limit)
	if err != nil {
		writeError(c, err)
		return
	}
	httpx.OK(c, httpx.NewPaginated(toProfileHitResponses(hits, h.cfg.IsOnline), next))
}
