package config

import (
	"strings"
	"testing"
	"time"
)

// clearEnv neutralizes every variable Load reads so tests are hermetic.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"APP_ENV", "HTTP_ADDR", "HTTP_READ_TIMEOUT", "HTTP_WRITE_TIMEOUT",
		"HTTP_IDLE_TIMEOUT", "HTTP_SHUTDOWN_TIMEOUT", "DATABASE_URL",
		"DB_MAX_CONNS", "DB_MIN_CONNS", "DB_MAX_CONN_LIFETIME",
		"DB_MAX_CONN_IDLE_TIME", "DB_CONNECT_TIMEOUT", "DB_HEALTHCHECK_TIMEOUT",
		"AUTO_MIGRATE", "CORS_ALLOWED_ORIGINS", "TRUSTED_PROXIES", "LOG_LEVEL", "LOG_FORMAT",
		"REQUEST_ID_HEADER", "SESSION_COOKIE_NAME", "CSRF_COOKIE_NAME",
		"SESSION_TTL", "COOKIE_SAMESITE", "COOKIE_PATH", "CSRF_ENABLED",
		"COOKIE_SECURE", "MAX_LOGIN_ATTEMPTS", "LOCKOUT_DURATION",
		"MIN_PASSWORD_LENGTH", "STORAGE_DIR", "MAX_AVATAR_BYTES",
		"PRESENCE_STALE_AFTER",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/weshare?sslmode=disable")

	cfg, err := Load("test-version")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Version != "test-version" {
		t.Errorf("Version = %q, want test-version", cfg.Version)
	}
	if cfg.Environment != EnvDevelopment {
		t.Errorf("Environment = %q, want development", cfg.Environment)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.RequestIDHeader != "X-Request-Id" {
		t.Errorf("RequestIDHeader = %q, want X-Request-Id", cfg.RequestIDHeader)
	}
	if cfg.DBMaxConns != 10 || cfg.DBMinConns != 1 {
		t.Errorf("pool sizes = (%d,%d), want (10,1)", cfg.DBMaxConns, cfg.DBMinConns)
	}
	if !cfg.AutoMigrate {
		t.Errorf("AutoMigrate = false, want true by default")
	}
	if len(cfg.CORSAllowedOrigins) != 1 || cfg.CORSAllowedOrigins[0] != "http://localhost:3000" {
		t.Errorf("CORSAllowedOrigins = %v", cfg.CORSAllowedOrigins)
	}
	if cfg.PresenceStaleAfter != 60*time.Second {
		t.Errorf("PresenceStaleAfter = %v, want 60s", cfg.PresenceStaleAfter)
	}
	if len(cfg.TrustedProxies) != 0 {
		t.Errorf("TrustedProxies = %v, want empty by default", cfg.TrustedProxies)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	clearEnv(t)

	_, err := Load("test")
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Errorf("error = %q, want mention of DATABASE_URL", err.Error())
	}
}

func TestLoadRejectsInvalidDatabaseScheme(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "mysql://root@localhost:3306/weshare")

	_, err := Load("test")
	if err == nil || !strings.Contains(err.Error(), "scheme must be postgres") {
		t.Fatalf("error = %v, want scheme validation failure", err)
	}
}

func TestLoadRejectsUnknownEnvironment(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db")
	t.Setenv("APP_ENV", "staging")

	_, err := Load("test")
	if err == nil || !strings.Contains(err.Error(), "APP_ENV") {
		t.Fatalf("error = %v, want APP_ENV validation failure", err)
	}
}

func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db")
	t.Setenv("LOG_LEVEL", "verbose")

	_, err := Load("test")
	if err == nil || !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Fatalf("error = %v, want LOG_LEVEL validation failure", err)
	}
}

func TestLoadRejectsWildcardCORSInProduction(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db")
	t.Setenv("APP_ENV", "production")
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")

	_, err := Load("test")
	if err == nil || !strings.Contains(err.Error(), "must not be '*'") {
		t.Fatalf("error = %v, want wildcard CORS rejection", err)
	}
}

func TestLoadParsesDurations(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db")
	t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "42s")

	cfg, err := Load("test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ShutdownTimeout != 42*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 42s", cfg.ShutdownTimeout)
	}
}
