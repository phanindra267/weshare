package config

import "testing"

func TestParseICEServers(t *testing.T) {
	t.Run("empty yields nil", func(t *testing.T) {
		got, err := parseICEServers("  ")
		if err != nil || got != nil {
			t.Fatalf("got %v, %v", got, err)
		}
	})

	t.Run("urls as array", func(t *testing.T) {
		got, err := parseICEServers(`[{"urls":["stun:a:3478","turn:b:3478"],"username":"u","credential":"c"}]`)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 1 || len(got[0].URLs) != 2 || got[0].Username != "u" || got[0].Credential != "c" {
			t.Fatalf("unexpected: %+v", got)
		}
	})

	t.Run("urls as single string", func(t *testing.T) {
		got, err := parseICEServers(`[{"urls":"stun:stun.l.google.com:19302"}]`)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 1 || got[0].URLs[0] != "stun:stun.l.google.com:19302" {
			t.Fatalf("unexpected: %+v", got)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		if _, err := parseICEServers("{not json"); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestICEServersValidation(t *testing.T) {
	base := func() *Config {
		return &Config{
			Environment:         EnvDevelopment,
			DatabaseURL:         "postgres://user:pass@localhost:5432/db",
			DBMaxConns:          10,
			DBMinConns:          1,
			LogLevel:            "info",
			LogFormat:           "json",
			CookieSameSite:      "Lax",
			SessionTTL:          1,
			MinPasswordLength:   8,
			MaxLoginAttempts:    5,
			LockoutDuration:     1,
			LoginRateLimit:      1,
			LoginRateWindow:     1,
			RegisterRateLimit:   1,
			RegisterRateWindow:  1,
			PasswordRateLimit:   1,
			PasswordRateWindow:  1,
			PresenceStaleAfter:  1,
			StorageDir:          "./storage",
			MaxAvatarBytes:      1,
			MaxAttachmentBytes:  1,
			CallMaxParticipants: 4,
		}
	}

	t.Run("valid tls turn", func(t *testing.T) {
		cfg := base()
		cfg.ICEServers = []ICEServer{{URLs: []string{"turns:turn.example.org:5349"}}}
		if err := cfg.validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rejects empty urls", func(t *testing.T) {
		cfg := base()
		cfg.ICEServers = []ICEServer{{}}
		if err := cfg.validate(); err == nil {
			t.Fatal("expected an error for an empty urls list")
		}
	})

	t.Run("rejects unknown scheme", func(t *testing.T) {
		cfg := base()
		cfg.ICEServers = []ICEServer{{URLs: []string{"http://example.org"}}}
		if err := cfg.validate(); err == nil {
			t.Fatal("expected an error for an unknown scheme")
		}
	})

	t.Run("rejects too few participants", func(t *testing.T) {
		cfg := base()
		cfg.CallMaxParticipants = 1
		if err := cfg.validate(); err == nil {
			t.Fatal("expected an error for CALL_MAX_PARTICIPANTS < 2")
		}
	})
}
