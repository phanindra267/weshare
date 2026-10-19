package notifications

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRegisterRoutes_MountsNotificationRoutes guards against routing conflicts
// between the static "/notifications/unread-count" and "/notifications/read"
// segments and the "/notifications/:id" parameter segment (gin panics on some
// conflicts). Every path must reach the auth guard (401), not the no-route
// handler (404).
func TestRegisterRoutes_MountsNotificationRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	grp := r.Group("/api/v1")
	requireAuth := func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) }

	NewHandler(NewService(newFakeStore(), nil)).RegisterRoutes(grp, requireAuth)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/notifications"},
		{http.MethodGet, "/api/v1/notifications/unread-count"},
		{http.MethodPost, "/api/v1/notifications/read"},
		{http.MethodDelete, "/api/v1/notifications"},
		{http.MethodDelete, "/api/v1/notifications/abc"},
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
