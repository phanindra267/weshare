// Package config loads and validates process configuration from environment
// variables. Invalid configuration is reported as an error at startup rather
// than causing a panic or a confusing failure later.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment identifies the deployment context.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvTest        Environment = "test"
	EnvProduction  Environment = "production"
)

// Config is the fully validated application configuration.
type Config struct {
	Environment Environment
	Version     string

	HTTPAddr        string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration

	DatabaseURL          string
	DBMaxConns           int32
	DBMinConns           int32
	DBMaxConnLifetime    time.Duration
	DBMaxConnIdleTime    time.Duration
	DBConnectTimeout     time.Duration
	DBHealthCheckTimeout time.Duration

	AutoMigrate bool

	CORSAllowedOrigins []string

	// TrustedProxies lists the proxy CIDRs/IPs whose X-Forwarded-For header may
	// be trusted for client-IP purposes (logging, rate limiting). Empty by
	// default, which means X-Forwarded-For is ignored and the direct peer
	// address is used. This must stay empty unless the app runs behind a proxy
	// we control, otherwise rate limits keyed on client IP are spoofable.
	TrustedProxies []string

	// Authentication / session settings.
	SessionCookieName string
	CSRFCookieName    string
	SessionTTL        time.Duration
	CookieSecure      bool
	CookieSameSite    string // Lax | Strict | None
	CookiePath        string
	CSRFEnabled       bool
	MaxLoginAttempts  int
	LockoutDuration   time.Duration
	MinPasswordLength int

	// Auth endpoint rate limits, keyed on client IP. Configurable so test
	// suites that create many accounts can raise them without weakening the
	// production defaults.
	LoginRateLimit     int
	LoginRateWindow    time.Duration
	RegisterRateLimit  int
	RegisterRateWindow time.Duration
	PasswordRateLimit  int
	PasswordRateWindow time.Duration

	// Presence: a user counts as online until this long after their last
	// heartbeat. Derived on read rather than trusting the stored flag.
	PresenceStaleAfter time.Duration

	// Media storage.
	StorageDir         string
	MaxAvatarBytes     int64
	MaxAttachmentBytes int64

	// WebRTC calling. ICEServers is empty by default (host candidates only,
	// which is sufficient on localhost); set ICE_SERVERS to a JSON array to
	// add STUN/TURN. CallMaxParticipants bounds a single call's mesh size.
	ICEServers          []ICEServer
	CallMaxParticipants int

	LogLevel  string
	LogFormat string

	RequestIDHeader string
}

// IsProduction reports whether the app runs with production settings.
func (c *Config) IsProduction() bool { return c.Environment == EnvProduction }

// IsOnline reports whether a user counts as online given their last heartbeat
// and the configured staleness window. Presence is derived on read so a client
// that stops sending heartbeats is not shown as online forever.
func (c *Config) IsOnline(lastSeen *time.Time) bool {
	if lastSeen == nil {
		return false
	}
	return time.Since(*lastSeen) <= c.PresenceStaleAfter
}

// Load reads configuration from the environment and validates it.
func Load(version string) (*Config, error) {
	cfg := &Config{
		Environment: Environment(getenv("APP_ENV", string(EnvDevelopment))),
		Version:     version,

		HTTPAddr:        getenv("HTTP_ADDR", ":8080"),
		ReadTimeout:     getDuration("HTTP_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:    getDuration("HTTP_WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:     getDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout: getDuration("HTTP_SHUTDOWN_TIMEOUT", 15*time.Second),

		DatabaseURL:          os.Getenv("DATABASE_URL"),
		DBMaxConns:           int32(getInt("DB_MAX_CONNS", 10)),
		DBMinConns:           int32(getInt("DB_MIN_CONNS", 1)),
		DBMaxConnLifetime:    getDuration("DB_MAX_CONN_LIFETIME", time.Hour),
		DBMaxConnIdleTime:    getDuration("DB_MAX_CONN_IDLE_TIME", 30*time.Minute),
		DBConnectTimeout:     getDuration("DB_CONNECT_TIMEOUT", 10*time.Second),
		DBHealthCheckTimeout: getDuration("DB_HEALTHCHECK_TIMEOUT", 3*time.Second),

		AutoMigrate: getBool("AUTO_MIGRATE", true),

		CORSAllowedOrigins: splitList(getenv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
		TrustedProxies:     splitList(getenv("TRUSTED_PROXIES", "")),

		SessionCookieName: getenv("SESSION_COOKIE_NAME", "weshare_session"),
		CSRFCookieName:    getenv("CSRF_COOKIE_NAME", "weshare_csrf"),
		SessionTTL:        getDuration("SESSION_TTL", 168*time.Hour),
		CookieSameSite:    strings.TrimSpace(getenv("COOKIE_SAMESITE", "lax")),
		CookiePath:        getenv("COOKIE_PATH", "/"),
		CSRFEnabled:       getBool("CSRF_ENABLED", true),
		MaxLoginAttempts:  getInt("MAX_LOGIN_ATTEMPTS", 5),
		LockoutDuration:   getDuration("LOCKOUT_DURATION", 15*time.Minute),
		MinPasswordLength: getInt("MIN_PASSWORD_LENGTH", 8),

		LoginRateLimit:     getInt("LOGIN_RATE_LIMIT", 30),
		LoginRateWindow:    getDuration("LOGIN_RATE_WINDOW", 15*time.Minute),
		RegisterRateLimit:  getInt("REGISTER_RATE_LIMIT", 10),
		RegisterRateWindow: getDuration("REGISTER_RATE_WINDOW", time.Hour),
		PasswordRateLimit:  getInt("PASSWORD_RATE_LIMIT", 10),
		PasswordRateWindow: getDuration("PASSWORD_RATE_WINDOW", time.Hour),

		PresenceStaleAfter: getDuration("PRESENCE_STALE_AFTER", 60*time.Second),

		StorageDir:         getenv("STORAGE_DIR", "./storage"),
		MaxAvatarBytes:     int64(getInt("MAX_AVATAR_BYTES", 2*1024*1024)),
		MaxAttachmentBytes: int64(getInt("MAX_ATTACHMENT_BYTES", 10*1024*1024)),

		CallMaxParticipants: getInt("CALL_MAX_PARTICIPANTS", 4),

		LogLevel:  strings.ToLower(getenv("LOG_LEVEL", "info")),
		LogFormat: strings.ToLower(getenv("LOG_FORMAT", "json")),

		RequestIDHeader: getenv("REQUEST_ID_HEADER", "X-Request-Id"),
	}

	// Cookies are Secure by default in production; require an explicit
	// opt-out (COOKIE_SECURE=false) rather than an opt-in elsewhere.
	cfg.CookieSecure = getBool("COOKIE_SECURE", cfg.IsProduction())

	iceServers, err := parseICEServers(os.Getenv("ICE_SERVERS"))
	if err != nil {
		return nil, fmt.Errorf("invalid ICE_SERVERS: %w", err)
	}
	cfg.ICEServers = iceServers

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	var problems []string

	switch c.Environment {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		problems = append(problems,
			fmt.Sprintf("APP_ENV must be one of development|test|production, got %q", c.Environment))
	}

	if c.DatabaseURL == "" {
		problems = append(problems, "DATABASE_URL is required")
	} else if u, err := url.Parse(c.DatabaseURL); err != nil {
		problems = append(problems, fmt.Sprintf("DATABASE_URL is not a valid URL: %v", err))
	} else if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		problems = append(problems,
			fmt.Sprintf("DATABASE_URL scheme must be postgres or postgresql, got %q", u.Scheme))
	}

	if c.DBMaxConns < 1 {
		problems = append(problems, "DB_MAX_CONNS must be >= 1")
	}
	if c.DBMinConns < 0 || c.DBMinConns > c.DBMaxConns {
		problems = append(problems, "DB_MIN_CONNS must be between 0 and DB_MAX_CONNS")
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		problems = append(problems, fmt.Sprintf("LOG_LEVEL must be debug|info|warn|error, got %q", c.LogLevel))
	}
	switch c.LogFormat {
	case "json", "text":
	default:
		problems = append(problems, fmt.Sprintf("LOG_FORMAT must be json|text, got %q", c.LogFormat))
	}

	switch strings.ToLower(c.CookieSameSite) {
	case "lax":
		c.CookieSameSite = "Lax"
	case "strict":
		c.CookieSameSite = "Strict"
	case "none":
		c.CookieSameSite = "None"
	default:
		problems = append(problems,
			fmt.Sprintf("COOKIE_SAMESITE must be Lax|Strict|None, got %q", c.CookieSameSite))
	}
	if c.CookieSameSite == "None" && !c.CookieSecure {
		problems = append(problems,
			"COOKIE_SAMESITE=None requires COOKIE_SECURE=true")
	}
	if c.SessionTTL <= 0 {
		problems = append(problems, "SESSION_TTL must be a positive duration")
	}
	if c.MinPasswordLength < 8 {
		problems = append(problems, "MIN_PASSWORD_LENGTH must be at least 8")
	}
	if c.MaxLoginAttempts < 1 {
		problems = append(problems, "MAX_LOGIN_ATTEMPTS must be at least 1")
	}
	if c.LockoutDuration <= 0 {
		problems = append(problems, "LOCKOUT_DURATION must be a positive duration")
	}
	for _, rl := range []struct {
		name  string
		limit int
		win   time.Duration
	}{
		{"LOGIN_RATE", c.LoginRateLimit, c.LoginRateWindow},
		{"REGISTER_RATE", c.RegisterRateLimit, c.RegisterRateWindow},
		{"PASSWORD_RATE", c.PasswordRateLimit, c.PasswordRateWindow},
	} {
		if rl.limit < 1 {
			problems = append(problems, rl.name+"_LIMIT must be at least 1")
		}
		if rl.win <= 0 {
			problems = append(problems, rl.name+"_WINDOW must be a positive duration")
		}
	}
	if c.PresenceStaleAfter <= 0 {
		problems = append(problems, "PRESENCE_STALE_AFTER must be a positive duration")
	}

	if strings.TrimSpace(c.StorageDir) == "" {
		problems = append(problems, "STORAGE_DIR must not be empty")
	}
	if c.MaxAvatarBytes <= 0 {
		problems = append(problems, "MAX_AVATAR_BYTES must be positive")
	}
	if c.MaxAttachmentBytes <= 0 {
		problems = append(problems, "MAX_ATTACHMENT_BYTES must be positive")
	}

	if c.CallMaxParticipants < 2 {
		problems = append(problems, "CALL_MAX_PARTICIPANTS must be at least 2")
	}
	for i, server := range c.ICEServers {
		if len(server.URLs) == 0 {
			problems = append(problems,
				fmt.Sprintf("ICE_SERVERS[%d] must have at least one url", i))
		}
		for _, u := range server.URLs {
			if !strings.HasPrefix(u, "stun:") &&
				!strings.HasPrefix(u, "turn:") &&
				!strings.HasPrefix(u, "turns:") {
				problems = append(problems,
					fmt.Sprintf("ICE_SERVERS[%d] url %q must use stun:, turn:, or turns:", i, u))
			}
		}
	}

	if c.IsProduction() && len(c.CORSAllowedOrigins) == 0 {
		problems = append(problems, "CORS_ALLOWED_ORIGINS must be set in production")
	}
	for _, origin := range c.CORSAllowedOrigins {
		if origin == "*" && c.IsProduction() {
			problems = append(problems, "CORS_ALLOWED_ORIGINS must not be '*' in production")
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid configuration:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

// --- helpers -------------------------------------------------------------

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func getDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}

func getBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return b
}

func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
