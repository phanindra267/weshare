package users

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/auth"
	"github.com/phanindra267/weshare/services/api/internal/config"
	"github.com/phanindra267/weshare/services/api/internal/httpx"
	"github.com/phanindra267/weshare/services/api/internal/media"
)

const (
	maxDisplayNameLength = 64
	maxBioLength         = 500
	maxStatusLength      = 140
	avatarURLPrefix      = "/api/v1/media/avatars/"
)

// Handler exposes profile endpoints.
type Handler struct {
	store   *Store
	storage *media.Storage
	cfg     *config.Config
}

// NewHandler builds a users Handler.
func NewHandler(store *Store, storage *media.Storage, cfg *config.Config) *Handler {
	return &Handler{store: store, storage: storage, cfg: cfg}
}

// RegisterRoutes mounts the profile routes.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	me := rg.Group("/me", requireAuth)
	me.GET("", h.Me)
	me.PATCH("", h.UpdateProfile)
	me.POST("/avatar", h.UploadAvatar)

	rg.POST("/presence/heartbeat", requireAuth, h.Heartbeat)
	rg.GET("/users/:id", requireAuth, h.GetProfile)
}

// ProfileResponse is the public profile shape.
type ProfileResponse struct {
	ID            string  `json:"id"`
	Username      *string `json:"username"`
	DisplayName   string  `json:"display_name"`
	Bio           *string `json:"bio"`
	AvatarURL     *string `json:"avatar_url"`
	StatusMessage *string `json:"status_message"`
	Online        bool    `json:"online"`
	LastSeenAt    *string `json:"last_seen_at"`
	CreatedAt     string  `json:"created_at"`
}

// toResponse maps a profile. Presence is derived from last_seen_at against the
// configured staleness window rather than trusting the stored flag, so a client
// that stops sending heartbeats is not shown as online forever.
func (h *Handler) toResponse(p Profile) ProfileResponse {
	resp := ProfileResponse{
		ID:            p.ID.String(),
		Username:      p.Username,
		DisplayName:   p.DisplayName,
		Bio:           p.Bio,
		AvatarURL:     p.AvatarURL,
		StatusMessage: p.StatusMessage,
		Online:        h.cfg.IsOnline(p.LastSeenAt),
		CreatedAt:     p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
	if p.LastSeenAt != nil {
		s := p.LastSeenAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		resp.LastSeenAt = &s
	}
	return resp
}

// Me handles GET /me.
func (h *Handler) Me(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	p, err := h.store.GetByID(c.Request.Context(), user.ID)
	if err != nil {
		httpx.Error(c, httpx.AsAppError(err))
		return
	}
	httpx.OK(c, h.toResponse(p))
}

// GetProfile handles GET /users/:id.
func (h *Handler) GetProfile(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.Error(c, httpx.NotFound("User not found."))
		return
	}
	p, err := h.store.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(c, httpx.NotFound("User not found."))
			return
		}
		httpx.Error(c, httpx.AsAppError(err))
		return
	}
	httpx.OK(c, h.toResponse(p))
}

type updateRequest struct {
	DisplayName   *string `json:"display_name"`
	Bio           *string `json:"bio"`
	StatusMessage *string `json:"status_message"`
}

// UpdateProfile handles PATCH /me.
func (h *Handler) UpdateProfile(c *gin.Context) {
	var body updateRequest
	if err := httpx.BindJSON(c, &body); err != nil {
		httpx.Error(c, err)
		return
	}

	fields := map[string]string{}
	upd := Update{}

	if body.DisplayName != nil {
		v := strings.TrimSpace(*body.DisplayName)
		switch {
		case v == "":
			fields["display_name"] = "Display name cannot be empty."
		case len(v) > maxDisplayNameLength:
			fields["display_name"] = "Display name must be at most 64 characters."
		default:
			upd.DisplayName = &v
		}
	}
	if body.Bio != nil {
		v := strings.TrimSpace(*body.Bio)
		if len(v) > maxBioLength {
			fields["bio"] = "Bio must be at most 500 characters."
		} else {
			upd.Bio = &v
		}
	}
	if body.StatusMessage != nil {
		v := strings.TrimSpace(*body.StatusMessage)
		if len(v) > maxStatusLength {
			fields["status_message"] = "Status must be at most 140 characters."
		} else {
			upd.StatusMessage = &v
		}
	}
	if len(fields) > 0 {
		httpx.Error(c, httpx.Validation("Please fix the highlighted fields.", gin.H{"fields": fields}))
		return
	}

	user, _ := auth.CurrentUser(c)
	p, err := h.store.UpdateProfile(c.Request.Context(), user.ID, upd)
	if err != nil {
		httpx.Error(c, httpx.AsAppError(err))
		return
	}
	httpx.OK(c, h.toResponse(p))
}

// UploadAvatar handles POST /me/avatar (multipart form field "avatar").
func (h *Handler) UploadAvatar(c *gin.Context) {
	user, _ := auth.CurrentUser(c)

	// Bound the request body before parsing to avoid memory abuse.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.cfg.MaxAvatarBytes+1024)

	fileHeader, err := c.FormFile("avatar")
	if err != nil {
		httpx.Error(c, httpx.Validation("An avatar image is required.",
			gin.H{"fields": map[string]string{"avatar": "Attach an image file."}}))
		return
	}
	if fileHeader.Size > h.cfg.MaxAvatarBytes {
		httpx.Error(c, httpx.Validation("Image is too large.",
			gin.H{"fields": map[string]string{"avatar": "Image must be 2 MB or smaller."}}))
		return
	}

	f, err := fileHeader.Open()
	if err != nil {
		httpx.Error(c, httpx.Internal(err))
		return
	}
	defer f.Close()

	name := user.ID.String() + "-" + uuid.NewString()[:8]
	rel, err := h.storage.SaveAvatar(name, f, h.cfg.MaxAvatarBytes)
	if err != nil {
		switch {
		case errors.Is(err, media.ErrTooLarge):
			httpx.Error(c, httpx.Validation("Image is too large.",
				gin.H{"fields": map[string]string{"avatar": "Image must be 2 MB or smaller."}}))
		case errors.Is(err, media.ErrUnsupportedImage):
			httpx.Error(c, httpx.Validation("Unsupported image type.",
				gin.H{"fields": map[string]string{"avatar": "Use a JPEG, PNG, or WebP image."}}))
		default:
			httpx.Error(c, httpx.Internal(err))
		}
		return
	}

	url := avatarURLPrefix + name + media.Ext(rel)
	p, err := h.store.SetAvatarURL(c.Request.Context(), user.ID, url)
	if err != nil {
		_ = h.storage.Remove(rel)
		httpx.Error(c, httpx.AsAppError(err))
		return
	}

	// Best-effort cleanup of the previous stored avatar.
	if user.AvatarURL != nil && strings.HasPrefix(*user.AvatarURL, avatarURLPrefix) {
		old := strings.TrimPrefix(*user.AvatarURL, avatarURLPrefix)
		_ = h.storage.Remove("avatars/" + old)
	}

	httpx.OK(c, h.toResponse(p))
}

// PresenceResponse is the reply to a presence heartbeat.
type PresenceResponse struct {
	Online     bool   `json:"online"`
	LastSeenAt string `json:"last_seen_at"`
}

// Heartbeat handles POST /presence/heartbeat: it records activity so other
// clients can derive presence from last_seen_at plus the staleness window.
func (h *Handler) Heartbeat(c *gin.Context) {
	user, _ := auth.CurrentUser(c)
	lastSeen, err := h.store.TouchPresence(c.Request.Context(), user.ID)
	if err != nil {
		httpx.Error(c, httpx.AsAppError(err))
		return
	}
	httpx.OK(c, PresenceResponse{
		Online:     true,
		LastSeenAt: lastSeen.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}
