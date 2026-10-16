package database

import (
	"context"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/phanindra267/weshare/services/api/migrations"
)

// migration is a single versioned up/down pair.
type migration struct {
	version int64
	name    string
	upSQL   string
	downSQL string
	hasDown bool
}

var migrationFilePattern = regexp.MustCompile(`^(\d+)_([a-zA-Z0-9_]+)\.(up|down)\.sql$`)

const migrationsTableDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    BIGINT PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`

// loadMigrations parses the embedded SQL files into an ordered slice.
func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	byVersion := map[int64]*migration{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := migrationFilePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		version, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse version in %q: %w", entry.Name(), err)
		}
		content, err := migrations.FS.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", entry.Name(), err)
		}

		m := byVersion[version]
		if m == nil {
			m = &migration{version: version, name: matches[2]}
			byVersion[version] = m
		}
		switch matches[3] {
		case "up":
			m.upSQL = string(content)
		case "down":
			m.downSQL = string(content)
			m.hasDown = true
		}
	}

	out := make([]migration, 0, len(byVersion))
	for _, m := range byVersion {
		if strings.TrimSpace(m.upSQL) == "" {
			return nil, fmt.Errorf("migration %d_%s is missing an up file", m.version, m.name)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// ensureMigrationsTable creates the bookkeeping table if needed.
func (db *DB) ensureMigrationsTable(ctx context.Context) error {
	_, err := db.Pool.Exec(ctx, migrationsTableDDL)
	return err
}

// appliedVersions returns the set of already-applied migration versions.
func (db *DB) appliedVersions(ctx context.Context) (map[int64]string, error) {
	rows, err := db.Pool.Query(ctx, `SELECT version, name FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := map[int64]string{}
	for rows.Next() {
		var version int64
		var name string
		if err := rows.Scan(&version, &name); err != nil {
			return nil, err
		}
		applied[version] = name
	}
	return applied, rows.Err()
}

// MigrateUp applies all pending migrations in ascending order. Each migration
// runs inside its own transaction and is recorded on success, so a failure
// leaves the database at a consistent version. It returns the versions applied.
func (db *DB) MigrateUp(ctx context.Context) ([]int64, error) {
	if err := db.ensureMigrationsTable(ctx); err != nil {
		return nil, fmt.Errorf("ensure schema_migrations: %w", err)
	}

	all, err := loadMigrations()
	if err != nil {
		return nil, err
	}
	applied, err := db.appliedVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("read applied versions: %w", err)
	}

	var appliedNow []int64
	for _, m := range all {
		if _, ok := applied[m.version]; ok {
			continue
		}
		if err := db.applyMigration(ctx, m); err != nil {
			return appliedNow, err
		}
		appliedNow = append(appliedNow, m.version)
	}
	return appliedNow, nil
}

func (db *DB) applyMigration(ctx context.Context, m migration) error {
	return pgx.BeginFunc(ctx, db.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, m.upSQL); err != nil {
			return fmt.Errorf("apply migration %d_%s: %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
			m.version, m.name); err != nil {
			return fmt.Errorf("record migration %d_%s: %w", m.version, m.name, err)
		}
		return nil
	})
}

// MigrateDown rolls back up to `steps` applied migrations (0 means all),
// newest first. Each rollback runs in its own transaction.
func (db *DB) MigrateDown(ctx context.Context, steps int) ([]int64, error) {
	if err := db.ensureMigrationsTable(ctx); err != nil {
		return nil, fmt.Errorf("ensure schema_migrations: %w", err)
	}

	all, err := loadMigrations()
	if err != nil {
		return nil, err
	}
	byVersion := make(map[int64]migration, len(all))
	for _, m := range all {
		byVersion[m.version] = m
	}

	applied, err := db.appliedVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("read applied versions: %w", err)
	}

	versions := make([]int64, 0, len(applied))
	for v := range applied {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] > versions[j] })

	if steps > 0 && len(versions) > steps {
		versions = versions[:steps]
	}

	var rolledBack []int64
	for _, v := range versions {
		m := byVersion[v]
		if !m.hasDown {
			return rolledBack, fmt.Errorf("migration %d_%s has no down file", m.version, m.name)
		}
		if err := db.rollbackMigration(ctx, m); err != nil {
			return rolledBack, err
		}
		rolledBack = append(rolledBack, v)
	}
	return rolledBack, nil
}

func (db *DB) rollbackMigration(ctx context.Context, m migration) error {
	return pgx.BeginFunc(ctx, db.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, m.downSQL); err != nil {
			return fmt.Errorf("rollback migration %d_%s: %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM schema_migrations WHERE version = $1`, m.version); err != nil {
			return fmt.Errorf("delete migration record %d_%s: %w", m.version, m.name, err)
		}
		return nil
	})
}

// MigrationStatus describes the current schema version.
type MigrationStatus struct {
	Current int64
	Applied []int64
	Pending []int64
}

// Status reports applied and pending migrations.
func (db *DB) Status(ctx context.Context) (MigrationStatus, error) {
	all, err := loadMigrations()
	if err != nil {
		return MigrationStatus{}, err
	}
	if err := db.ensureMigrationsTable(ctx); err != nil {
		return MigrationStatus{}, err
	}
	applied, err := db.appliedVersions(ctx)
	if err != nil {
		return MigrationStatus{}, err
	}

	status := MigrationStatus{}
	for _, m := range all {
		if _, ok := applied[m.version]; ok {
			status.Applied = append(status.Applied, m.version)
			if m.version > status.Current {
				status.Current = m.version
			}
		} else {
			status.Pending = append(status.Pending, m.version)
		}
	}
	return status, nil
}
