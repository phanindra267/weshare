package media

import (
	"errors"
	"io"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/phanindra267/weshare/services/api/internal/httpx"
)

// Handler serves stored media over HTTP.
type Handler struct {
	storage *Storage
}

// NewHandler builds a media Handler.
func NewHandler(storage *Storage) *Handler {
	return &Handler{storage: storage}
}

// RegisterRoutes mounts media routes. requireAuth guards access so assets are
// only visible to authenticated users.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	g := rg.Group("/media", requireAuth)
	g.GET("/avatars/:name", h.Avatar)
}

var contentTypeByExt = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
}

// Avatar serves an avatar file by name.
func (h *Handler) Avatar(c *gin.Context) {
	name := c.Param("name")
	f, err := h.storage.Open("avatars", name)
	if err != nil {
		if errors.Is(err, ErrInvalidName) {
			httpx.Error(c, httpx.NotFound("Image not found."))
			return
		}
		httpx.Error(c, httpx.NotFound("Image not found."))
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		httpx.Error(c, httpx.NotFound("Image not found."))
		return
	}

	ct := contentTypeByExt[strings.ToLower(filepath.Ext(name))]
	if ct == "" {
		ct = "application/octet-stream"
	}
	c.Header("Content-Type", ct)
	c.Header("Cache-Control", "private, max-age=3600")
	c.Status(200)
	_, _ = io.Copy(c.Writer, f)
}
