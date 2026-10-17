package media

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/phanindra267/weshare/services/api/internal/auth"
	"github.com/phanindra267/weshare/services/api/internal/httpx"
)

// attachmentURLPrefix is the public path an uploaded attachment is served from.
const attachmentURLPrefix = "/api/v1/media/attachments/"

// Handler serves stored media over HTTP.
type Handler struct {
	storage            *Storage
	attachments        *AttachmentStore
	maxAttachmentBytes int64
}

// NewHandler builds a media Handler without an attachment store.
func NewHandler(storage *Storage, maxAttachmentBytes int64) *Handler {
	return &Handler{storage: storage, maxAttachmentBytes: maxAttachmentBytes}
}

// NewHandlerWithStore builds a media Handler with PostgreSQL attachment metadata tracking.
func NewHandlerWithStore(storage *Storage, attachments *AttachmentStore, maxAttachmentBytes int64) *Handler {
	return &Handler{storage: storage, attachments: attachments, maxAttachmentBytes: maxAttachmentBytes}
}

// RegisterRoutes mounts media routes. requireAuth guards access so assets are
// only visible to authenticated users.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, requireAuth gin.HandlerFunc) {
	g := rg.Group("/media", requireAuth)
	g.GET("/avatars/:name", h.Avatar)
	g.POST("/attachments", h.UploadAttachment)
	g.GET("/attachments/:name", h.Attachment)
	g.GET("/download/:id", h.DownloadAttachment)
	g.DELETE("/attachments/:id", h.DeleteAttachment)
}

// Avatar serves an avatar file by name.
func (h *Handler) Avatar(c *gin.Context) {
	h.serve(c, "avatars", c.Param("name"))
}

// Attachment serves a stored chat attachment by filename. If tracked in
// PostgreSQL, authorization is verified against the requesting user.
func (h *Handler) Attachment(c *gin.Context) {
	name := c.Param("name")
	if h.attachments != nil {
		if user, ok := auth.CurrentUser(c); ok {
			storagePath := filepath.Join("attachments", name)
			if h.attachments.IsTracked(c.Request.Context(), storagePath) {
				allowed, err := h.attachments.CanAccessPath(c.Request.Context(), user.ID, storagePath)
				if err != nil || !allowed {
					httpx.Error(c, httpx.Forbidden("You do not have access to this attachment."))
					return
				}
			}
		}
	}
	h.serve(c, "attachments", name)
}

// DownloadAttachment serves an attachment file by its database UUID with a
// Content-Disposition header and strict authorization check.
func (h *Handler) DownloadAttachment(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || h.attachments == nil {
		httpx.Error(c, httpx.NotFound("Attachment not found."))
		return
	}
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	allowed, err := h.attachments.CanAccess(c.Request.Context(), user.ID, id)
	if err != nil {
		httpx.Error(c, httpx.Internal(err))
		return
	}
	if !allowed {
		httpx.Error(c, httpx.Forbidden("You do not have access to this attachment."))
		return
	}
	rec, err := h.attachments.ByID(c.Request.Context(), id)
	if errors.Is(err, ErrAttachmentNotFound) {
		httpx.Error(c, httpx.NotFound("Attachment not found."))
		return
	}
	if err != nil {
		httpx.Error(c, httpx.Internal(err))
		return
	}
	h.serveByPath(c, rec.StoragePath, rec.OriginalName, rec.ContentType)
}

// DeleteAttachment deletes an attachment row and file (uploader only).
func (h *Handler) DeleteAttachment(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || h.attachments == nil {
		httpx.Error(c, httpx.NotFound("Attachment not found."))
		return
	}
	user, ok := auth.CurrentUser(c)
	if !ok {
		httpx.Error(c, httpx.Unauthorized(""))
		return
	}
	storagePath, err := h.attachments.Delete(c.Request.Context(), id, user.ID)
	if errors.Is(err, ErrAttachmentForbidden) {
		httpx.Error(c, httpx.Forbidden("You may only delete your own attachments."))
		return
	}
	if err != nil {
		httpx.Error(c, httpx.Internal(err))
		return
	}
	_ = h.storage.Remove(storagePath)
	httpx.NoContent(c)
}

func sanitizeFilename(name string) string {
	return strings.ReplaceAll(name, `"`, `'`)
}

// serveByPath streams a stored attachment file by its storage path.
func (h *Handler) serveByPath(c *gin.Context, storagePath, originalName, contentType string) {
	category := filepath.Dir(storagePath)
	name := filepath.Base(storagePath)
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

	if contentType == "" {
		contentType = ContentTypeForExt(filepath.Ext(name))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "private, max-age=3600")
	if originalName != "" {
		c.Header("Content-Disposition", `attachment; filename="`+sanitizeFilename(originalName)+`"`)
	}
	c.Status(200)
	_, _ = io.Copy(c.Writer, f)
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
	ID          string `json:"id,omitempty"`
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
}

// UploadAttachment handles POST /media/attachments (multipart form field
// "file"). Images, videos, and audio are classified from their bytes; everything
// else is stored as a generic file. If the attachment store is configured, metadata
// is recorded in PostgreSQL.
func (h *Handler) UploadAttachment(c *gin.Context) {
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

	resp := UploadAttachmentResponse{
		URL:         attachmentURLPrefix + name + Ext(rel),
		ContentType: contentType,
		Kind:        kind,
		Name:        fileHeader.Filename,
		Size:        size,
	}

	if h.attachments != nil {
		if user, ok := auth.CurrentUser(c); ok {
			rec, err := h.attachments.InsertPending(
				c.Request.Context(),
				user.ID,
				rel,
				contentType,
				kind,
				fileHeader.Filename,
				size,
			)
			if err == nil {
				resp.ID = rec.ID.String()
			}
		}
	}

	httpx.Created(c, resp)
}
