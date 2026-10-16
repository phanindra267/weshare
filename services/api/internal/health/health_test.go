package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/phanindra267/weshare/services/api/internal/config"
)

type stubPinger struct {
	err   error
	calls int
}

func (s *stubPinger) Ping(_ context.Context, _ time.Duration) error {
	s.calls++
	return s.err
}

func newTestConfig() *config.Config {
	return &config.Config{
		Version:              "test",
		DBHealthCheckTimeout: time.Second,
	}
}

func TestLiveAlwaysReportsOk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(&stubPinger{}, newTestConfig())

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/healthz", nil)

	h.Live(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	if body["service"] != "weshare-api" {
		t.Errorf("service = %v", body["service"])
	}
	if _, ok := body["uptime_seconds"]; !ok {
		t.Error("uptime_seconds missing")
	}
}

func TestReadyIsOkWhenDatabasePings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pinger := &stubPinger{}
	h := NewHandler(pinger, newTestConfig())

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/readyz", nil)

	h.Ready(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Status string         `json:"status"`
		Checks map[string]any `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ready" {
		t.Errorf("status = %q, want ready", body.Status)
	}
	if body.Checks["database"] != "ok" {
		t.Errorf("checks.database = %v, want ok", body.Checks["database"])
	}
	if pinger.calls != 1 {
		t.Errorf("Ping calls = %d, want 1", pinger.calls)
	}
}

func TestReadyIs503WhenDatabaseUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(&stubPinger{err: errors.New("db down")}, newTestConfig())

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/readyz", nil)

	h.Ready(c)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "not_ready" {
		t.Errorf("status = %q, want not_ready", body.Status)
	}
}
