package search

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/phanindra267/weshare/services/api/internal/config"
)

// TestRegisterRoutes_MountsSearchRoutes verifies the search routes resolve to
// the auth guard (401) rather than the no-route handler (404).
func TestRegisterRoutes_MountsSearchRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	grp := r.Group("/api/v1")
	requireAuth := func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) }

	NewHandler(NewService(&fakeStore{}), &config.Config{}).RegisterRoutes(grp, requireAuth)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/search/messages"},
		{http.MethodGet, "/api/v1/search/users"},
		{http.MethodGet, "/api/v1/search/conversations"},
		{http.MethodGet, "/api/v1/search/groups"},
		{http.MethodGet, "/api/v1/discover/people"},
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
