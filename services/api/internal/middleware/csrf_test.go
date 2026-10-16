package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/phanindra267/weshare/services/api/internal/config"
)

func newCSRFTestRouter(cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CSRF(cfg))
	r.POST("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func csrfConfig() *config.Config {
	return &config.Config{
		SessionCookieName: "weshare_session",
		CSRFCookieName:    "weshare_csrf",
		CSRFEnabled:       true,
	}
}

func TestCSRF_ExemptWhenNoSession(t *testing.T) {
	r := newCSRFTestRouter(csrfConfig())
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 without session, got %d", rec.Code)
	}
}

func TestCSRF_SafeMethodAllowedWithSession(t *testing.T) {
	r := newCSRFTestRouter(csrfConfig())
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.AddCookie(&http.Cookie{Name: "weshare_session", Value: "s"})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET, got %d", rec.Code)
	}
}

func TestCSRF_BlocksSessionWithoutToken(t *testing.T) {
	r := newCSRFTestRouter(csrfConfig())
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.AddCookie(&http.Cookie{Name: "weshare_session", Value: "s"})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without CSRF token, got %d", rec.Code)
	}
}

func TestCSRF_AllowsMatchingToken(t *testing.T) {
	r := newCSRFTestRouter(csrfConfig())
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.AddCookie(&http.Cookie{Name: "weshare_session", Value: "s"})
	req.AddCookie(&http.Cookie{Name: "weshare_csrf", Value: "token-123"})
	req.Header.Set("X-CSRF-Token", "token-123")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with matching token, got %d", rec.Code)
	}
}

func TestCSRF_RejectsMismatchedToken(t *testing.T) {
	r := newCSRFTestRouter(csrfConfig())
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.AddCookie(&http.Cookie{Name: "weshare_session", Value: "s"})
	req.AddCookie(&http.Cookie{Name: "weshare_csrf", Value: "token-123"})
	req.Header.Set("X-CSRF-Token", "different")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for mismatched token, got %d", rec.Code)
	}
}

func TestCSRF_Disabled(t *testing.T) {
	cfg := csrfConfig()
	cfg.CSRFEnabled = false
	r := newCSRFTestRouter(cfg)
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.AddCookie(&http.Cookie{Name: "weshare_session", Value: "s"})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when disabled, got %d", rec.Code)
	}
}
