package http_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/phanindra267/weshare/services/api/internal/config"
	"github.com/phanindra267/weshare/services/api/internal/health"
	apihttp "github.com/phanindra267/weshare/services/api/internal/http"
	"github.com/phanindra267/weshare/services/api/internal/logging"
)

type okPinger struct{}

func (okPinger) Ping(context.Context, time.Duration) error { return nil }

func newRouter(t *testing.T) *gin.Engine {
	t.Helper()
	cfg := &config.Config{
		Environment:          config.EnvTest,
		Version:              "test",
		RequestIDHeader:      "X-Request-Id",
		CORSAllowedOrigins:   []string{"http://localhost:3000"},
		DBHealthCheckTimeout: time.Second,
	}
	logger := logging.New(io.Discard, "error", "json")
	return apihttp.New(apihttp.Options{
		Config: cfg,
		Logger: logger,
		Health: health.NewHandler(okPinger{}, cfg),
	})
}

func do(r *gin.Engine, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestHealthEndpoint(t *testing.T) {
	r := newRouter(t)
	rec := do(r, http.MethodGet, "/healthz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("X-Request-Id") == "" {
		t.Error("expected X-Request-Id response header")
	}
}

func TestPingEndpointUsesDataEnvelope(t *testing.T) {
	r := newRouter(t)
	rec := do(r, http.MethodGet, "/api/v1/ping", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Data struct {
			Pong bool `json:"pong"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Data.Pong {
		t.Error("expected data.pong to be true")
	}
}

func TestUnknownRouteReturnsErrorEnvelope(t *testing.T) {
	r := newRouter(t)
	rec := do(r, http.MethodGet, "/api/v1/does-not-exist", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "NOT_FOUND" {
		t.Errorf("code = %q, want NOT_FOUND", body.Error.Code)
	}
	if body.Error.RequestID == "" {
		t.Error("expected request_id in error envelope")
	}
}

func TestInboundRequestIDIsHonored(t *testing.T) {
	r := newRouter(t)
	rec := do(r, http.MethodGet, "/healthz", map[string]string{"X-Request-Id": "abc-123"})
	if got := rec.Header().Get("X-Request-Id"); got != "abc-123" {
		t.Errorf("X-Request-Id = %q, want abc-123", got)
	}
}

func TestCORSPreflightForAllowedOrigin(t *testing.T) {
	r := newRouter(t)
	rec := do(r, http.MethodOptions, "/api/v1/ping", map[string]string{
		"Origin":                        "http://localhost:3000",
		"Access-Control-Request-Method": "GET",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("Allow-Origin = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials = %q, want true", got)
	}
}

func TestCORSDisallowedOriginGetsNoHeader(t *testing.T) {
	r := newRouter(t)
	rec := do(r, http.MethodGet, "/api/v1/ping", map[string]string{
		"Origin": "http://evil.example",
	})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Allow-Origin header for disallowed origin, got %q", got)
	}
}
