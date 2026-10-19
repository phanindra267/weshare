package notifications

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/auth"
	"github.com/phanindra267/weshare/services/api/internal/httpx"
)

// Handler exposes the /notifications HTTP surface.
type Handler struct {
	svc *Service
}

// NewHandler builds a notifications Handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes mounts the notifications routes under the given group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	g := rg.Group("/notifications", requireAuth)

	g.GET("", h.List)
	g.GET("/unread-count", h.UnreadCount)
	g.POST("/read", h.MarkRead)
	g.DELETE("", h.DeleteAll)
	g.DELETE("/:id", h.Delete)
}

// writeError maps a domain error onto the standard error envelope.
func writeError(c *gin.Context, err error) {
	var verr *ValidationError
	switch {
	case errors.As(err, &verr):
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": map[string]string{verr.Field: verr.Message}}))
	case errors.Is(err, ErrNotFound):
		httpx.Error(c, httpx.NotFound("Notification not found."))
	default:
		httpx.Error(c, httpx.AsAppError(err))
	}
}

// listParams reads the optional `limit` and `cursor` query parameters.
func listParams(c *gin.Context) (limit int, cursor string, ok bool) {
	fields := map[string]string{}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			fields["limit"] = "limit must be a positive integer."
		} else {
			limit = n
		}
	}
	if len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": fields}))
		return 0, "", false
	}
	return limit, strings.TrimSpace(c.Query("cursor")), true
}

// List handles GET /notifications.
func (h *Handler) List(c *gin.Context) {
	limit, cursor, ok := listParams(c)
	if !ok {
		return
	}

	user, _ := auth.CurrentUser(c)
	items, next, err := h.svc.List(c.Request.Context(), user.ID, cursor, limit)
	if err != nil {
		writeError(c, err)
		return
	}
	httpx.OK(c, httpx.NewPaginated(toResponses(items), next))
}

// UnreadCount handles GET /notifications/unread-count.
func (h *Handler) UnreadCount(c *gin.Context) {
	user, _ := auth.CurrentUser(c)
	count, err := h.svc.UnreadCount(c.Request.Context(), user.ID)
	if err != nil {
		writeError(c, err)
		return
	}
	httpx.OK(c, gin.H{"unread_count": count})
}

// MarkRead handles POST /notifications/read.
func (h *Handler) MarkRead(c *gin.Context) {
	var body MarkReadRequest
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}

	ids := make([]uuid.UUID, 0, len(body.IDs))
	for _, raw := range body.IDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
				gin.H{"fields": map[string]string{"ids": "contains an invalid id."}}))
			return
		}
		ids = append(ids, id)
	}

	user, _ := auth.CurrentUser(c)
	remaining, err := h.svc.MarkRead(c.Request.Context(), user.ID, ids)
	if err != nil {
		writeError(c, err)
		return
	}
	httpx.OK(c, gin.H{"unread_count": remaining})
}

// Delete handles DELETE /notifications/:id.
func (h *Handler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, httpx.NotFound("Notification not found."))
		return
	}

	user, _ := auth.CurrentUser(c)
	if err := h.svc.Delete(c.Request.Context(), user.ID, id); err != nil {
		writeError(c, err)
		return
	}
	httpx.NoContent(c)
}

// DeleteAll handles DELETE /notifications.
func (h *Handler) DeleteAll(c *gin.Context) {
	user, _ := auth.CurrentUser(c)
	if err := h.svc.DeleteAll(c.Request.Context(), user.ID); err != nil {
		writeError(c, err)
		return
	}
	httpx.NoContent(c)
}
