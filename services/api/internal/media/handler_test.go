package media

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRegisterRoutes_MountsAttachmentRoutes verifies the attachment upload and
// download routes resolve to the auth guard (401) rather than 404.
func TestRegisterRoutes_MountsAttachmentRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	storage, err := NewStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	r := gin.New()
	grp := r.Group("/api/v1")
	requireAuth := func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) }
	NewHandler(storage, 1024).RegisterRoutes(grp, requireAuth)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/media/attachments"},
		{http.MethodGet, "/api/v1/media/attachments/abc.png"},
		{http.MethodGet, "/api/v1/media/avatars/abc.png"},
		{http.MethodGet, "/api/v1/media/download/550e8400-e29b-41d4-a716-446655440000"},
		{http.MethodDelete, "/api/v1/media/attachments/550e8400-e29b-41d4-a716-446655440000"},
	}

	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}
