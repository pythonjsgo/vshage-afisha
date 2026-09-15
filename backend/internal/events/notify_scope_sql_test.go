package events

import (
	"context"
	"reflect"
	"testing"
)

func TestRegistrationPushTargetsSQLRequireEventOptIn(t *testing.T) {
	pool := boardPool(t)
	ctx := context.Background()
	const owner = "00000000-0000-0000-0000-000000000001"
	const admin = "00000000-0000-0000-0000-000000000002"
	const selected = "00000000-0000-0000-0000-000000000003"
	const other = "00000000-0000-0000-0000-000000000004"
	if _, err := pool.Exec(ctx, `ALTER TABLE profiles ADD COLUMN is_admin boolean DEFAULT false,ADD COLUMN status text DEFAULT 'active';
 INSERT INTO profiles(id,name,is_admin) VALUES('00000000-0000-0000-0000-000000000001','Owner',false),('00000000-0000-0000-0000-000000000002','Admin',true);
 INSERT INTO events(id,title,start_time,status,organizer_id) VALUES('00000000-0000-0000-0000-000000000003','Selected',now(),'published','00000000-0000-0000-0000-000000000001'),('00000000-0000-0000-0000-000000000004','Other',now(),'published','00000000-0000-0000-0000-000000000001')`); err != nil {
		t.Fatal(err)
	}
	check := func(id string, want []string) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		got, err := pushTargets(ctx, tx, mailEvent{ID: id, OrganizerID: owner})
		if err != nil {
			t.Fatal(err)
		}
		asSet := map[string]bool{}
		for _, v := range got {
			asSet[v] = true
		}
		expected := map[string]bool{}
		for _, v := range want {
			expected[v] = true
		}
		if !reflect.DeepEqual(asSet, expected) {
			t.Fatalf("event %s recipients %v, want %v", id, got, want)
		}
	}
	// Missing migration must not restore the former owner-wide subscription.
	check(selected, []string{admin})
	if _, err := pool.Exec(ctx, `ALTER TABLE events ADD COLUMN notify_organizer_on_registration boolean NOT NULL DEFAULT false`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE events SET notify_organizer_on_registration=true WHERE id=$1`, selected); err != nil {
		t.Fatal(err)
	}
	check(selected, []string{admin, owner})
	check(other, []string{admin})
	if _, err := pool.Exec(ctx, `UPDATE events SET notify_organizer_on_registration=false WHERE id=$1`, selected); err != nil {
		t.Fatal(err)
	}
	check(selected, []string{admin})
}
