// Package http wires the Gin engine: global middleware, probes, and the
// versioned API surface. Handlers are registered on sub-routers by feature
// packages as phases are implemented.
package http

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/phanindra267/weshare/services/api/internal/auth"
	"github.com/phanindra267/weshare/services/api/internal/config"
	"github.com/phanindra267/weshare/services/api/internal/contacts"
	"github.com/phanindra267/weshare/services/api/internal/database"
	"github.com/phanindra267/weshare/services/api/internal/health"
	"github.com/phanindra267/weshare/services/api/internal/httpx"
	"github.com/phanindra267/weshare/services/api/internal/media"
	"github.com/phanindra267/weshare/services/api/internal/middleware"
	"github.com/phanindra267/weshare/services/api/internal/users"
)

// Options are the dependencies required to build the router.
type Options struct {
	Config   *config.Config
	Logger   *slog.Logger
	DB       *database.DB
	Health   *health.Handler
	Auth     *auth.Handler
	Users    *users.Handler
	Media    *media.Handler
	Contacts *contacts.Handler
}

// New builds the configured Gin engine.
func New(opts Options) *gin.Engine {
	cfg, logger := opts.Config, opts.Logger
	if !cfg.IsProduction() {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// gin.New (not gin.Default) so we control every middleware.
	r := gin.New()
	r.RedirectTrailingSlash = false
	r.RemoveExtraSlash = true

	// Only believe X-Forwarded-For from proxies we explicitly trust. gin's
	// default is to trust every proxy, which would let any client spoof their
	// IP and defeat per-IP rate limiting. An empty list means the direct peer
	// address is always used.
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		logger.Warn("invalid trusted proxies; falling back to none", "error", err)
		_ = r.SetTrustedProxies(nil)
	}

	// Recover first so it can catch panics from anything below it.
	r.Use(middleware.Recovery(logger))
	r.Use(middleware.RequestID(cfg.RequestIDHeader))
	r.Use(middleware.RequestLogger(logger))
	r.Use(middleware.ErrorHandler())
	r.Use(middleware.CORS(cfg.CORSAllowedOrigins))

	r.NoRoute(func(c *gin.Context) {
		httpx.Error(c, httpx.NotFound("No route matches this request."))
	})
	r.NoMethod(func(c *gin.Context) {
		httpx.Error(c, httpx.NotFound("No route matches this request."))
	})

	// Probes live outside the versioned API and are intentionally unwrapped.
	r.GET("/healthz", opts.Health.Live)
	r.GET("/readyz", opts.Health.Ready)

	api := r.Group("/api/v1")
	// CSRF protection applies to state-changing requests carrying a session.
	api.Use(middleware.CSRF(cfg))
	registerAPIRoutes(api, opts)

	return r
}

// registerAPIRoutes mounts the versioned API surface. Feature routers are
// attached here as they are implemented in later phases.
func registerAPIRoutes(api *gin.RouterGroup, opts Options) {
	api.GET("/ping", func(c *gin.Context) {
		httpx.OK(c, gin.H{
			"pong": true,
			"time": time.Now().UTC(),
		})
	})

	if opts.Auth != nil {
		opts.Auth.RegisterRoutes(api)
	}
	if opts.Users != nil && opts.Auth != nil {
		opts.Users.RegisterRoutes(api, opts.Auth.RequireAuth())
	}
	if opts.Media != nil && opts.Auth != nil {
		opts.Media.RegisterRoutes(api, opts.Auth.RequireAuth())
	}
	if opts.Contacts != nil && opts.Auth != nil {
		opts.Contacts.RegisterRoutes(api, opts.Auth.RequireAuth())
	}
}
