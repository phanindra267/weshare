package feed

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/phanindra267/weshare/services/api/internal/config"
)

// TestRegisterRoutes_MountsDiscoveryRoutes verifies the Phase 8 discovery
// surfaces resolve to the auth guard (401) rather than the no-route handler
// (404), and that they coexist with the /moments/:id parameter route.
func TestRegisterRoutes_MountsDiscoveryRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	grp := r.Group("/api/v1")
	requireAuth := func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) }

	NewHandler(testService(newFakeStore(), nil), &config.Config{}).
		RegisterRoutes(grp, requireAuth)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/search/posts"},
		{http.MethodGet, "/api/v1/discover/moments"},
		{http.MethodGet, "/api/v1/moments"},
		{http.MethodGet, "/api/v1/moments/00000000-0000-0000-0000-000000000001"},
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
