package feed

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/auth"
	"github.com/phanindra267/weshare/services/api/internal/config"
	"github.com/phanindra267/weshare/services/api/internal/httpx"
)

// Handler exposes the /moments HTTP surface.
type Handler struct {
	svc *Service
	cfg *config.Config
}

// NewHandler builds a feed Handler.
func NewHandler(svc *Service, cfg *config.Config) *Handler {
	return &Handler{svc: svc, cfg: cfg}
}

// RegisterRoutes mounts the moments routes under the given group.
//
// Every sibling segment under /moments is either the collection itself or the
// `:id` parameter, so the router has no static/param ambiguity.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	g := rg.Group("/moments", requireAuth)

	g.GET("", h.Feed)
	g.POST("", h.Create)
	g.GET("/:id", h.Get)
	g.PATCH("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
	g.POST("/:id/hide", h.Hide)
	g.POST("/:id/report", h.Report)
	g.POST("/:id/likes", h.Like)
	g.DELETE("/:id/likes", h.Unlike)
	g.GET("/:id/comments", h.Comments)
	g.POST("/:id/comments", h.AddComment)
	g.DELETE("/:id/comments/:commentId", h.DeleteComment)

	// Phase 8 discovery surfaces query moments but live outside the /moments
	// collection so they cannot clash with the /moments/:id parameter route.
	rg.GET("/search/posts", requireAuth, h.SearchPosts)
	rg.GET("/discover/moments", requireAuth, h.DiscoverMoments)
}

// Feed lists moments from the caller and their friends, newest first.
func (h *Handler) Feed(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	moments, next, err := h.svc.Feed(c.Request.Context(), user.ID, c.Query("cursor"), limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPaginated(ToMomentResponses(moments), next))
}

// Create publishes a new moment.
func (h *Handler) Create(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	var body CreateMomentRequest
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}
	media := make([]MediaInput, 0, len(body.Media))
	for _, item := range body.Media {
		media = append(media, MediaInput{URL: item.URL, MediaType: item.MediaType})
	}
	visibility := body.Visibility
	if visibility == "" {
		visibility = c.Query("visibility")
	}
	moment, err := h.svc.Create(c.Request.Context(), toAuthor(user), CreateMomentInput{
		Body:            body.Body,
		Visibility:      visibility,
		ExcludedUserIDs: body.ExcludedUserIDs,
		Media:           media,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Created(c, toMomentResponse(moment))
}

// Get returns a single moment.
func (h *Handler) Get(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	id, ok := h.momentID(c)
	if !ok {
		return
	}
	moment, err := h.svc.Get(c.Request.Context(), user.ID, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.OK(c, toMomentResponse(moment))
}

// Update modifies a moment authored by the caller.
func (h *Handler) Update(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	id, ok := h.momentID(c)
	if !ok {
		return
	}
	var body UpdateMomentRequest
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}
	moment, err := h.svc.Update(c.Request.Context(), user.ID, id, UpdateMomentInput{
		Body:       body.Body,
		Visibility: body.Visibility,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.OK(c, toMomentResponse(moment))
}

// Delete soft-deletes a moment the caller authored.
func (h *Handler) Delete(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	id, ok := h.momentID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), user.ID, id); err != nil {
		h.fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// Hide removes a moment from the caller's feed.
func (h *Handler) Hide(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	id, ok := h.momentID(c)
	if !ok {
		return
	}
	if err := h.svc.Hide(c.Request.Context(), user.ID, id); err != nil {
		h.fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// Report files a report against a moment.
func (h *Handler) Report(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	id, ok := h.momentID(c)
	if !ok {
		return
	}
	var body ReportMomentRequest
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}
	details := ""
	if body.Details != nil {
		details = *body.Details
	}
	if err := h.svc.Report(c.Request.Context(), user.ID, id, ReportInput{
		Reason:  body.Reason,
		Details: details,
	}); err != nil {
		h.fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// Like adds a like and returns the resulting count.
func (h *Handler) Like(c *gin.Context) {
	h.react(c, true)
}

// Unlike removes a like and returns the resulting count.
func (h *Handler) Unlike(c *gin.Context) {
	h.react(c, false)
}

func (h *Handler) react(c *gin.Context, like bool) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	id, ok := h.momentID(c)
	if !ok {
		return
	}
	var (
		count int64
		err   error
	)
	if like {
		count, err = h.svc.Like(c.Request.Context(), user.ID, id)
	} else {
		count, err = h.svc.Unlike(c.Request.Context(), user.ID, id)
	}
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"like_count": count, "liked": like})
}

// Comments lists a moment's comments, oldest first.
func (h *Handler) Comments(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	id, ok := h.momentID(c)
	if !ok {
		return
	}
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	comments, next, err := h.svc.Comments(c.Request.Context(), user.ID, id, c.Query("cursor"), limit)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPaginated(toCommentResponses(comments), next))
}

// AddComment appends a comment to a moment.
func (h *Handler) AddComment(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	id, ok := h.momentID(c)
	if !ok {
		return
	}
	var body AddCommentRequest
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}
	var replyToID *uuid.UUID
	if body.ReplyToID != nil && strings.TrimSpace(*body.ReplyToID) != "" {
		if parsed, err := uuid.Parse(strings.TrimSpace(*body.ReplyToID)); err == nil {
			replyToID = &parsed
		}
	}
	comment, err := h.svc.AddComment(c.Request.Context(), user.ID, id, body.Body, replyToID)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Created(c, toCommentResponse(comment))
}

// DeleteComment removes a comment authored by the caller (or on the caller's
// moment).
func (h *Handler) DeleteComment(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	if _, ok := h.momentID(c); !ok {
		return
	}
	commentID, err := uuid.Parse(c.Param("commentId"))
	if err != nil {
		httpx.Error(c, httpx.NotFound("Comment not found."))
		return
	}
	if err := h.svc.DeleteComment(c.Request.Context(), user.ID, commentID); err != nil {
		h.fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// SearchPosts handles GET /search/posts: text search over moments visible to
// the caller.
func (h *Handler) SearchPosts(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	moments, next, err := h.svc.SearchPosts(c.Request.Context(), user.ID,
		c.Query("q"), c.Query("cursor"), queryLimit(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPaginated(ToMomentResponses(moments), next))
}

// DiscoverMoments handles GET /discover/moments: the newest public moments.
func (h *Handler) DiscoverMoments(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	moments, next, err := h.svc.Discover(c.Request.Context(), user.ID,
		c.Query("cursor"), queryLimit(c))
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPaginated(ToMomentResponses(moments), next))
}

// queryLimit parses the optional `limit` query parameter; 0 means "unset".
func queryLimit(c *gin.Context) int {
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			return n
		}
	}
	return 0
}

// momentID parses the :id parameter, rendering a 404 when it is not a UUID.
func (h *Handler) momentID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, httpx.NotFound("Moment not found."))
		return uuid.Nil, false
	}
	return id, true
}

// fail translates domain errors into HTTP responses.
func (h *Handler) fail(c *gin.Context, err error) {
	var invalid *ValidationError
	switch {
	case errors.As(err, &invalid):
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.",
			gin.H{"fields": map[string]string{invalid.Field: invalid.Message}}))
	case errors.Is(err, ErrMomentNotFound), errors.Is(err, ErrCommentNotFound):
		httpx.Error(c, httpx.NotFound("The requested post was not found."))
	case errors.Is(err, ErrNotAuthor):
		httpx.Error(c, httpx.Forbidden("Only the author can modify this post."))
	case errors.Is(err, ErrEmptyMoment):
		httpx.Error(c, httpx.Validation("Add some text or a photo to share.",
			gin.H{"body": "a moment needs text or media"}))
	case errors.Is(err, ErrCannotView):
		httpx.Error(c, httpx.NotFound("The requested post was not found."))
	default:
		httpx.Error(c, httpx.AsAppError(err))
	}
}

// toAuthor adapts the authenticated principal to the feed Author value.
func toAuthor(u auth.User) Author {
	return Author{
		ID:          u.ID,
		DisplayName: u.DisplayName,
		Username:    u.Username,
		AvatarURL:   u.AvatarURL,
	}
}
