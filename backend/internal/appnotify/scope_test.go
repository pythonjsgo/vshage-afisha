package appnotify

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
)

func TestRegistrationDeliveryRechecksEventScope(t *testing.T) {
	dsn := os.Getenv("AFISHA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AFISHA_TEST_DATABASE_URL is required")
	}
	ctx := context.Background()
	boot, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = boot.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS appnotify_scope_test`); err != nil {
		t.Fatal(err)
	}
	boot.Close()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = "appnotify_scope_test"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, `DROP TABLE IF EXISTS notifications,events,profiles;
 CREATE TABLE profiles(id uuid PRIMARY KEY,status text,is_admin boolean);
 CREATE TABLE events(id uuid PRIMARY KEY,organizer_id uuid,notify_organizer_on_registration boolean DEFAULT false);
 CREATE TABLE notifications(profile_id uuid,type text,title text,body text,data jsonb);
 INSERT INTO profiles VALUES('00000000-0000-0000-0000-000000000001','active',false),('00000000-0000-0000-0000-000000000002','active',true);
 INSERT INTO events VALUES('00000000-0000-0000-0000-000000000003','00000000-0000-0000-0000-000000000001',true),('00000000-0000-0000-0000-000000000004','00000000-0000-0000-0000-000000000001',false);`)
	if err != nil {
		t.Fatal(err)
	}
	s := &Sender{pool: pool}
	const owner = "00000000-0000-0000-0000-000000000001"
	const admin = "00000000-0000-0000-0000-000000000002"
	const selected = "00000000-0000-0000-0000-000000000003"
	const other = "00000000-0000-0000-0000-000000000004"
	if err = s.Deliver(ctx, owner, "Test", "Selected", selected); err != nil {
		t.Fatal(err)
	}
	if err = s.Deliver(ctx, owner, "Test", "Other", other); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE events SET notify_organizer_on_registration=false WHERE id=$1`, selected); err != nil {
		t.Fatal(err)
	}
	if err = s.Deliver(ctx, owner, "Test", "Previously queued", selected); err != nil {
		t.Fatal(err)
	}
	if err = s.Deliver(ctx, admin, "Test", "Other", other); err != nil {
		t.Fatal(err)
	}
	var ownerCount, adminCount int
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FILTER(WHERE profile_id=$1),COUNT(*) FILTER(WHERE profile_id=$2) FROM notifications`, owner, admin).Scan(&ownerCount, &adminCount); err != nil {
		t.Fatal(err)
	}
	if ownerCount != 1 || adminCount != 1 {
		t.Fatalf("notifications: owner=%d admin=%d", ownerCount, adminCount)
	}
}
