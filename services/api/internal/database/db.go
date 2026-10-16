// Package database manages the PostgreSQL connection pool and migrations.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/phanindra267/weshare/services/api/internal/config"
)

// DB owns the process-wide connection pool.
type DB struct {
	Pool *pgxpool.Pool
}

// Connect builds a validated pgx pool and verifies connectivity.
func Connect(ctx context.Context, cfg *config.Config) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	poolCfg.MaxConns = cfg.DBMaxConns
	poolCfg.MinConns = cfg.DBMinConns
	poolCfg.MaxConnLifetime = cfg.DBMaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.DBMaxConnIdleTime
	poolCfg.ConnConfig.ConnectTimeout = cfg.DBConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.DBConnectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	return &DB{Pool: pool}, nil
}

// Ping verifies the database is reachable within the given timeout.
func (db *DB) Ping(ctx context.Context, timeout time.Duration) error {
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return db.Pool.Ping(pingCtx)
}

// Close releases all pooled connections.
func (db *DB) Close() {
	if db.Pool != nil {
		db.Pool.Close()
	}
}
