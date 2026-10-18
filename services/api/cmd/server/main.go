// Command server runs the WeShare HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/phanindra267/weshare/services/api/internal/auth"
	"github.com/phanindra267/weshare/services/api/internal/calls"
	"github.com/phanindra267/weshare/services/api/internal/config"
	"github.com/phanindra267/weshare/services/api/internal/contacts"
	"github.com/phanindra267/weshare/services/api/internal/database"
	"github.com/phanindra267/weshare/services/api/internal/feed"
	"github.com/phanindra267/weshare/services/api/internal/health"
	apihttp "github.com/phanindra267/weshare/services/api/internal/http"
	"github.com/phanindra267/weshare/services/api/internal/logging"
	"github.com/phanindra267/weshare/services/api/internal/media"
	"github.com/phanindra267/weshare/services/api/internal/messaging"
	"github.com/phanindra267/weshare/services/api/internal/realtime"
	"github.com/phanindra267/weshare/services/api/internal/users"
)

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load a local .env when present (development convenience). Real
	// deployments inject environment variables directly.
	_ = godotenv.Load()

	cfg, err := config.Load(version)
	if err != nil {
		return err
	}

	logger := logging.New(os.Stdout, cfg.LogLevel, cfg.LogFormat)
	logger.Info("starting weshare api",
		"version", cfg.Version,
		"env", cfg.Environment,
		"addr", cfg.HTTPAddr,
	)

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelStartup()

	db, err := database.Connect(startupCtx, cfg)
	if err != nil {
		logger.Error("database connection failed", "error", err.Error())
		return err
	}
	defer db.Close()
	logger.Info("database connected")

	if cfg.AutoMigrate {
		applied, err := db.MigrateUp(startupCtx)
		if err != nil {
			logger.Error("migration failed", "error", err.Error())
			return err
		}
		if len(applied) > 0 {
			logger.Info("migrations applied", "versions", applied)
		} else {
			logger.Info("database schema up to date")
		}
	}

	authService := auth.NewService(auth.NewStore(db), cfg)

	storage, err := media.NewStorage(cfg.StorageDir)
	if err != nil {
		logger.Error("media storage init failed", "error", err.Error(), "dir", cfg.StorageDir)
		return err
	}
	logger.Info("media storage ready", "dir", storage.Root())

	// Realtime hub fans events out over WebSockets and doubles as the messaging
	// publisher, so the messaging service stays transport-agnostic.
	contactsStore := contacts.NewStore(db)
	hub := realtime.NewHub()
	messagingService := messaging.NewService(messaging.NewStore(db), hub)
	// The same hub fans out new Moments to a poster's friends.
	feedService := feed.NewService(feed.NewStore(db), hub)
	// ...and relays WebRTC call signalling between a call's participants.
	callsService := calls.NewService(calls.NewStore(db), hub, cfg.CallMaxParticipants)

	attachmentStore := media.NewAttachmentStore(db)
	mediaHandler := media.NewHandlerWithStore(storage, attachmentStore, cfg.MaxAttachmentBytes)

	router := apihttp.New(apihttp.Options{
		Config:    cfg,
		Logger:    logger,
		DB:        db,
		Health:    health.NewHandler(db, cfg),
		Auth:      auth.NewHandler(authService),
		Users:     users.NewHandler(users.NewStore(db), storage, cfg),
		Media:     mediaHandler,
		Contacts:  contacts.NewHandler(contacts.NewService(contactsStore), cfg),
		Messaging: messaging.NewHandler(messagingService, cfg),
		Realtime:  realtime.NewHandler(messagingService, callsService, hub, contactsStore, cfg, logger),
		Feed:      feed.NewHandler(feedService, cfg),
		Calls:     calls.NewHandler(callsService, cfg),
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Background sweeper for abandoned / orphaned pending uploads (older than 24 hours).
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				purgeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				paths, err := attachmentStore.PurgeOrphaned(purgeCtx, 24*time.Hour)
				cancel()
				if err != nil {
					logger.Warn("purge orphaned attachments failed", "error", err.Error())
				} else if len(paths) > 0 {
					for _, p := range paths {
						_ = storage.Remove(p)
					}
					logger.Info("purged orphaned attachments", "count", len(paths))
				}
			}
		}
	}()

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err.Error())
		return err
	}
	logger.Info("shutdown complete")
	return nil
}
