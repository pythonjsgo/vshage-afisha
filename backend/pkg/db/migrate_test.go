package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
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

	// Unique service prefix + table name so the test never collides with the
	// real schema (or a concurrent test run) on a shared DB.
	svc := fmt.Sprintf("migtest_%d", time.Now().UnixNano())
	tbl := svc + "_tbl"

	dir := t.TempDir()
	write := func(name, sql string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(sql), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("001_a.sql", "CREATE TABLE IF NOT EXISTS "+tbl+" (id INT PRIMARY KEY);")
	write("002_b.sql", "ALTER TABLE "+tbl+" ADD COLUMN IF NOT EXISTS note TEXT;")

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl)
		_, _ = pool.Exec(ctx, "DELETE FROM schema_migrations WHERE version LIKE $1", svc+"/%")
	})

	if err := RunMigrations(ctx, pool, svc, dir); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Both migrations must be recorded, namespaced under the service prefix.
	var n int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM schema_migrations WHERE version = ANY($1)",
		[]string{svc + "/001_a.sql", svc + "/002_b.sql"},
	).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("want 2 recorded migrations under %q prefix, got %d", svc, n)
	}

	// The ALTER must have landed.
	var hasNote bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			WHERE table_name = $1 AND column_name = 'note'
		)`, tbl).Scan(&hasNote); err != nil {
		t.Fatalf("check column: %v", err)
	}
	if !hasNote {
		t.Fatal("migration 002 did not add the note column")
	}

	// Idempotency: a second run is a no-op and must not error.
	if err := RunMigrations(ctx, pool, svc, dir); err != nil {
		t.Fatalf("second run (idempotency): %v", err)
	}

	// A different service prefix must NOT see the first service's rows — even
	// with an identically named file. This is the cross-service collision the
	// prefix exists to prevent.
	other := svc + "_other"
	otherTbl := other + "_tbl"
	odir := t.TempDir()
	if err := os.WriteFile(filepath.Join(odir, "001_a.sql"),
		[]byte("CREATE TABLE IF NOT EXISTS "+otherTbl+" (id INT PRIMARY KEY);"), 0o644); err != nil {
		t.Fatalf("write other migration: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DROP TABLE IF EXISTS "+otherTbl)
		_, _ = pool.Exec(ctx, "DELETE FROM schema_migrations WHERE version LIKE $1", other+"/%")
	})
	if err := RunMigrations(ctx, pool, other, odir); err != nil {
		t.Fatalf("other-service run: %v", err)
	}
	var otherExists bool
	if err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)", otherTbl,
	).Scan(&otherExists); err != nil {
		t.Fatalf("check other table: %v", err)
	}
	if !otherExists {
		t.Fatal("a same-named 001_a.sql under a different service prefix was wrongly skipped")
	}
}

// TestRunMigrations_BadDir confirms a missing directory surfaces as an error
// rather than a panic. No DB needed.
func TestRunMigrations_BadDir(t *testing.T) {
	err := RunMigrations(context.Background(), nil, "afisha", filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("want error for missing migrations dir, got nil")
	}
}

// TestRunMigrations_EmptyService rejects a blank prefix — recording an
// unprefixed version in the shared table is exactly what we want to avoid.
func TestRunMigrations_EmptyService(t *testing.T) {
	err := RunMigrations(context.Background(), nil, "", t.TempDir())
	if err == nil {
		t.Fatal("want error for empty service prefix, got nil")
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
