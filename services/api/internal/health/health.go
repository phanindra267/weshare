// Package health exposes liveness and readiness probes.
package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/phanindra267/weshare/services/api/internal/config"
)

// Pinger is the readiness dependency. *database.DB satisfies it; tests supply
// a stub.
type Pinger interface {
	Ping(ctx context.Context, timeout time.Duration) error
}

// Handler serves health and readiness checks.
type Handler struct {
	db      Pinger
	cfg     *config.Config
	started time.Time
}

// NewHandler builds a health Handler.
func NewHandler(db Pinger, cfg *config.Config) *Handler {
	return &Handler{db: db, cfg: cfg, started: time.Now()}
}

// Live reports that the process is running. It never touches dependencies, so
// it stays cheap and reliable for container liveness probes.
func (h *Handler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":         "ok",
		"service":        "weshare-api",
		"version":        h.cfg.Version,
		"uptime_seconds": time.Since(h.started).Seconds(),
	})
}

// Ready reports whether the service can serve traffic, which requires a
// reachable database. It returns 503 when a dependency is unavailable.
func (h *Handler) Ready(c *gin.Context) {
	checks := gin.H{"database": "ok"}
	ready := true

	if h.db == nil {
		checks["database"] = "error"
		ready = false
	} else if err := h.db.Ping(c.Request.Context(), h.cfg.DBHealthCheckTimeout); err != nil {
		checks["database"] = "error"
		ready = false
	}

	if !ready {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not_ready",
			"checks": checks,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
		"checks": checks,
	})
}
