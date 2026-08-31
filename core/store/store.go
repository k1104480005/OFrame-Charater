// Package store provides the local application database: a SQLite schema with
// versioned migrations, backing the recoverable task queue, settings, and call
// statistics of later phases (design D7, tasks 6.x / 4.6).
//
// The schema and migration framework are driver-agnostic over database/sql.
// The planned driver is the pure-Go modernc.org/sqlite, registered as
// "sqlite"; it is added once the Go module proxy is reachable. Until then the
// framework is fully unit-tested with a self-contained stub driver (see
// migrate_test.go) and the real-SQLite smoke test skips with an explanation.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	// Register the pure-Go SQLite driver (modernc.org/sqlite) as "sqlite" so
	// store.Migrate / store.Open work without callers importing the driver
	// (design D7: the local task queue database).
	_ "modernc.org/sqlite"
)

// Migration is one versioned schema migration. Version must be a positive
// integer unique across the list; migrations apply in ascending version order.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// Migrations is the ordered, immutable list of application schema migrations.
// Append-only: never renumber or edit an applied migration — add a new one.
var Migrations = []Migration{
	{Version: 1, Name: "init", SQL: schemaInit},
	// v2: task_results — the idempotent success-result cache (tasks 6.4/4.8:
	// 相同任务成功结果缓存, 不重复计费). keyed by a deterministic task
	// fingerprint; the result payload is opaque JSON.
	{Version: 2, Name: "task_results", SQL: schemaTaskResults},
	// v3: task_ext — the persistent-session columns on the tasks table
	// (payload for re-execution, success result for the dedup cache, and the
	// dedup fingerprint). Added with ALTER so migration v1 stays immutable.
	{Version: 3, Name: "task_ext", SQL: schemaTaskExt},
	// v4: task_owner — the owning identity package of each task row
	// (package_path, empty for legacy rows). The identity page only resumes a
	// base-character generation task whose owner matches the open package, so
	// package B can never display package A's in-flight generation.
	{Version: 4, Name: "task_owner", SQL: schemaTaskOwner},
}

// schemaTaskOwner adds the owning-identity-package column to the tasks table.
const schemaTaskOwner = `
ALTER TABLE tasks ADD COLUMN package_path TEXT NOT NULL DEFAULT '';
`

// schemaTaskExt adds the persistent-session columns to the tasks table.
const schemaTaskExt = `
ALTER TABLE tasks ADD COLUMN payload TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN result TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN fingerprint TEXT NOT NULL DEFAULT '';
`

// schemaTaskResults defines the success-result cache table used for idempotent
// deduplication: the fingerprint identifies an identical task, the result
// payload is reused without issuing a new external call.
const schemaTaskResults = `
CREATE TABLE IF NOT EXISTS task_results (
    fingerprint TEXT PRIMARY KEY,
    result      TEXT NOT NULL,
    created_at  TEXT NOT NULL
);
`

// schemaInit defines the initial schema: the recoverable task queue table
// (provider parameters, expected call count, status, progress, error, retry
// count — tasks spec 6.1) and a key/value meta table.
const schemaInit = `
CREATE TABLE IF NOT EXISTS tasks (
    id                  TEXT PRIMARY KEY,
    kind                TEXT NOT NULL,
    provider            TEXT NOT NULL DEFAULT '',
    provider_params     TEXT NOT NULL DEFAULT '{}',
    expected_call_count INTEGER NOT NULL DEFAULT 0,
    status              TEXT NOT NULL,
    progress            REAL NOT NULL DEFAULT 0,
    error               TEXT NOT NULL DEFAULT '',
    retry_count         INTEGER NOT NULL DEFAULT 0,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`

// schemaMigrationsDDL is the bookkeeping table recording applied versions.
const schemaMigrationsDDL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TEXT NOT NULL
);
`

// ValidateMigrations checks that migrations have positive unique versions,
// non-empty names and SQL, and are ordered by ascending version.
func ValidateMigrations(ms []Migration) error {
	seen := make(map[int]string, len(ms))
	for i, m := range ms {
		if m.Version <= 0 {
			return fmt.Errorf("store: migration %d has invalid version %d", i, m.Version)
		}
		if strings.TrimSpace(m.Name) == "" {
			return fmt.Errorf("store: migration %d has an empty name", i)
		}
		if strings.TrimSpace(m.SQL) == "" {
			return fmt.Errorf("store: migration %d (%s) has empty SQL", i, m.Name)
		}
		if prev, dup := seen[m.Version]; dup {
			return fmt.Errorf("store: duplicate migration version %d (%s / %s)", m.Version, prev, m.Name)
		}
		seen[m.Version] = m.Name
		if i > 0 && m.Version <= ms[i-1].Version {
			return fmt.Errorf("store: migrations must be ordered by ascending version (%d after %d)", m.Version, ms[i-1].Version)
		}
	}
	return nil
}

// PendingMigrations returns the migrations whose versions are not present in
// applied, preserving the list order.
func PendingMigrations(applied map[int]bool, all []Migration) []Migration {
	var out []Migration
	for _, m := range all {
		if !applied[m.Version] {
			out = append(out, m)
		}
	}
	return out
}

// splitStatements splits a migration script into individual statements on
// top-level semicolons. The application's schema contains no quoted
// semicolons; a simple scanner is sufficient and keeps the framework
// dependency-free.
func splitStatements(sql string) []string {
	var out []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	for _, r := range sql {
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case r == ';' && !inSingle && !inDouble:
			if s := strings.TrimSpace(cur.String()); s != "" {
				out = append(out, s)
			}
			cur.Reset()
			continue
		}
		cur.WriteRune(r)
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// Migrator applies schema migrations to a *sql.DB in version order, recording
// applied versions in schema_migrations.
type Migrator struct {
	DB         *sql.DB
	Migrations []Migration
}

// NewMigrator wraps db with the application migrations.
func NewMigrator(db *sql.DB) *Migrator {
	return &Migrator{DB: db, Migrations: Migrations}
}

// CurrentVersion returns the highest applied migration version (0 if none).
func (m *Migrator) CurrentVersion(ctx context.Context) (int, error) {
	var v sql.NullInt64
	if err := m.DB.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations").Scan(&v); err != nil {
		return 0, fmt.Errorf("store: read current version: %w", err)
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}

// Apply applies all pending migrations in ascending order. Each migration runs
// in its own transaction together with the schema_migrations record, so a
// failed migration rolls back completely. Apply is idempotent.
func (m *Migrator) Apply(ctx context.Context) error {
	if err := ValidateMigrations(m.Migrations); err != nil {
		return err
	}
	if _, err := m.DB.ExecContext(ctx, schemaMigrationsDDL); err != nil {
		return fmt.Errorf("store: create schema_migrations: %w", err)
	}
	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return err
	}
	for _, mig := range PendingMigrations(applied, m.Migrations) {
		if err := m.applyOne(ctx, mig); err != nil {
			return fmt.Errorf("store: apply migration %d (%s): %w", mig.Version, mig.Name, err)
		}
	}
	return nil
}

func (m *Migrator) applyOne(ctx context.Context, mig Migration) error {
	tx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, stmt := range splitStatements(mig.SQL) {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
		mig.Version, mig.Name, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *Migrator) appliedVersions(ctx context.Context) (map[int]bool, error) {
	rows, err := m.DB.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("store: read applied migrations: %w", err)
	}
	defer rows.Close()
	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// Open opens (creating if needed) the local application database with the given
// driver and verifies connectivity. driverName is the database/sql driver
// name; the planned driver is "sqlite" (modernc.org/sqlite).
func Open(driverName, dsn string) (*sql.DB, error) {
	if !slices.Contains(sql.Drivers(), driverName) {
		return nil, fmt.Errorf("store: sql driver %q is not registered (install modernc.org/sqlite or register a SQLite driver)", driverName)
	}
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: cannot reach database: %w", err)
	}
	return db, nil
}

// Migrate opens the database and applies all pending migrations. It returns
// the ready *sql.DB.
func Migrate(driverName, dsn string) (*sql.DB, error) {
	db, err := Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	if err := NewMigrator(db).Apply(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// ErrDriverUnavailable reports that no SQLite driver is registered.
var ErrDriverUnavailable = errors.New("store: no sqlite driver available")
