package media

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/httpx"
)

// attachmentURLPrefix is the public path an uploaded attachment is served from.
const attachmentURLPrefix = "/api/v1/media/attachments/"

// Handler serves stored media over HTTP.
type Handler struct {
	storage            *Storage
	maxAttachmentBytes int64
}

// NewHandler builds a media Handler. maxAttachmentBytes bounds attachment
// uploads and must be positive.
func NewHandler(storage *Storage, maxAttachmentBytes int64) *Handler {
	return &Handler{storage: storage, maxAttachmentBytes: maxAttachmentBytes}
}

// RegisterRoutes mounts media routes. requireAuth guards access so assets are
// only visible to authenticated users.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	g := rg.Group("/media", requireAuth)
	g.GET("/avatars/:name", h.Avatar)
	g.POST("/attachments", h.UploadAttachment)
	g.GET("/attachments/:name", h.Attachment)
}

// Avatar serves an avatar file by name.
func (h *Handler) Avatar(c *gin.Context) {
	h.serve(c, "avatars", c.Param("name"))
}

// Attachment serves a stored chat attachment by name.
func (h *Handler) Attachment(c *gin.Context) {
	h.serve(c, "attachments", c.Param("name"))
}

// serve streams a stored object within a category.
func (h *Handler) serve(c *gin.Context, category, name string) {
	f, err := h.storage.Open(category, name)
	if err != nil {
		httpx.Error(c, httpx.NotFound("File not found."))
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		httpx.Error(c, httpx.NotFound("File not found."))
		return
	}

	ct := ContentTypeForExt(filepath.Ext(name))
	if ct == "" {
		ct = "application/octet-stream"
	}
	c.Header("Content-Type", ct)
	c.Header("Cache-Control", "private, max-age=3600")
	c.Status(200)
	_, _ = io.Copy(c.Writer, f)
}

// UploadAttachmentResponse is the reply to a successful attachment upload.
type UploadAttachmentResponse struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
}

// UploadAttachment handles POST /media/attachments (multipart form field
// "file"). Images and audio are classified from their bytes; everything else is
// stored as a generic file.
func (h *Handler) UploadAttachment(c *gin.Context) {
	// Bound the request body before parsing to avoid memory abuse.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxAttachmentBytes+1024)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.Error(c, httpx.PayloadTooLarge("That file is too large."))
			return
		}
		httpx.Error(c, httpx.Validation("A file is required.",
			gin.H{"fields": map[string]string{"file": "Attach a file under the \"file\" field."}}))
		return
	}
	if fileHeader.Size > h.maxAttachmentBytes {
		httpx.Error(c, httpx.PayloadTooLarge("That file is too large."))
		return
	}

	f, err := fileHeader.Open()
	if err != nil {
		httpx.Error(c, httpx.Internal(err))
		return
	}
	defer f.Close()

	name := uuid.NewString()
	rel, contentType, kind, size, err := h.storage.SaveAttachment(name, f, h.maxAttachmentBytes, fileHeader.Filename)
	if err != nil {
		if errors.Is(err, ErrTooLarge) {
			httpx.Error(c, httpx.PayloadTooLarge("That file is too large."))
			return
		}
		httpx.Error(c, httpx.Internal(err))
		return
	}

	httpx.Created(c, UploadAttachmentResponse{
		URL:         attachmentURLPrefix + name + Ext(rel),
		ContentType: contentType,
		Kind:        kind,
		Name:        fileHeader.Filename,
		Size:        size,
	})
}
