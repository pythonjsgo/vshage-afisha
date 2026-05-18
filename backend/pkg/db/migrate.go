package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunMigrations applies every *.sql file in migrationsDir, in filename order,
// that has not already been recorded in the schema_migrations table. Each
// migration runs inside its own transaction. Idempotent: re-running is a
// no-op once a migration is recorded.
//
// This mirrors core-api's runner so the schema-migration mechanism is the
// same across the Вшаге Go services.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir %q: %w", migrationsDir, err)
	}

	// Tracking table — created lazily so a fresh DB bootstraps itself.
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	// os.ReadDir already returns sorted entries, but be explicit: migration
	// order is filename order and must not depend on filesystem quirks.
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, version := range names {
		var exists bool
		if err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", version,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if exists {
			continue
		}

		sqlBytes, err := os.ReadFile(filepath.Join(migrationsDir, version))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", version, err)
		}

		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("execute migration %s: %w", version, err)
		}

		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (version) VALUES ($1)", version,
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", version, err)
		}

		log.Printf("migration applied: %s", version)
	}
	return nil
}

// FindMigrationsDir locates the migrations directory. It first tries a path
// relative to this source file (works with `go run` and `go test`), then a
// path relative to the working directory (the Docker image copies the dir to
// /pkg/db/migrations and the binary runs with CWD=/).
func FindMigrationsDir() string {
	if _, filename, _, ok := runtime.Caller(0); ok {
		// this file is backend/pkg/db/migrate.go → ../../pkg/db/migrations
		dir := filepath.Join(filepath.Dir(filename), "migrations")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}

	for _, c := range []string{
		"/pkg/db/migrations",
		"pkg/db/migrations",
		"../../pkg/db/migrations",
	} {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}

	return "/pkg/db/migrations"
}
