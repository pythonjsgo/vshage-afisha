package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestRunMigrations_Integration exercises the real runner against a Postgres
// instance. It is skipped unless TEST_DATABASE_URL is set (same convention as
// the events integration test). The DB user must be able to CREATE TABLE.
func TestRunMigrations_Integration(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := NewPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	// Two migrations in a temp dir. Use a unique table name so the test does
	// not collide with the real schema on a shared DB.
	dir := t.TempDir()
	write := func(name, sql string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(sql), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("001_a.sql", "CREATE TABLE IF NOT EXISTS migrate_test_tbl (id INT PRIMARY KEY);")
	write("002_b.sql", "ALTER TABLE migrate_test_tbl ADD COLUMN IF NOT EXISTS note TEXT;")

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS migrate_test_tbl")
		_, _ = pool.Exec(ctx, "DELETE FROM schema_migrations WHERE version IN ('001_a.sql','002_b.sql')")
	})

	if err := RunMigrations(ctx, pool, dir); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Both migrations must be recorded.
	var n int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM schema_migrations WHERE version IN ('001_a.sql','002_b.sql')",
	).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("want 2 recorded migrations, got %d", n)
	}

	// The ALTER must have landed.
	var hasNote bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'migrate_test_tbl' AND column_name = 'note'
		)`).Scan(&hasNote); err != nil {
		t.Fatalf("check column: %v", err)
	}
	if !hasNote {
		t.Fatal("migration 002 did not add the note column")
	}

	// Idempotency: a second run is a no-op and must not error.
	if err := RunMigrations(ctx, pool, dir); err != nil {
		t.Fatalf("second run (idempotency): %v", err)
	}
}

// TestRunMigrations_BadDir confirms a missing directory surfaces as an error
// rather than a panic. No DB needed.
func TestRunMigrations_BadDir(t *testing.T) {
	err := RunMigrations(context.Background(), nil, filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("want error for missing migrations dir, got nil")
	}
}

// TestFindMigrationsDir verifies the source-relative resolution finds the real
// migrations directory when running from the repo (go test).
func TestFindMigrationsDir(t *testing.T) {
	dir := FindMigrationsDir()
	if _, err := os.Stat(filepath.Join(dir, "001_init.sql")); err != nil {
		t.Fatalf("FindMigrationsDir returned %q which has no 001_init.sql: %v", dir, err)
	}
}
