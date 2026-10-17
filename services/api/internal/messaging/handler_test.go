package messaging

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/phanindra267/weshare/services/api/internal/config"
)

// TestRegisterRoutes_MountsConversationAndGroupRoutes guards against a routing
// conflict between the static "/conversations/group" segment and the
// "/conversations/:id" parameter segment (gin panics on some conflicts). Each
// path must resolve to the auth guard (401) rather than the no-route handler
// (404).
func TestRegisterRoutes_MountsConversationAndGroupRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	grp := r.Group("/api/v1")
	requireAuth := func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) }

	NewHandler(NewService(newFakeStore(), nil), &config.Config{}).RegisterRoutes(grp, requireAuth)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/conversations"},
		{http.MethodPost, "/api/v1/conversations"},
		{http.MethodPost, "/api/v1/conversations/group"},
		{http.MethodGet, "/api/v1/conversations/abc"},
		{http.MethodPatch, "/api/v1/conversations/abc"},
		{http.MethodGet, "/api/v1/conversations/abc/messages"},
		{http.MethodPost, "/api/v1/conversations/abc/messages"},
		{http.MethodPost, "/api/v1/conversations/abc/read"},
		{http.MethodGet, "/api/v1/conversations/abc/members"},
		{http.MethodPost, "/api/v1/conversations/abc/members"},
		{http.MethodDelete, "/api/v1/conversations/abc/members/def"},
		{http.MethodPost, "/api/v1/conversations/abc/members/def/role"},
		{http.MethodPost, "/api/v1/conversations/abc/leave"},
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
