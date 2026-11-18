//go:build integration

package mindlink

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestMySQLPrincipalLifecycleReadinessRejectsBadSchema(t *testing.T) {
	for name, ddl := range map[string]string{
		"missing_table":        "DROP TABLE mindlink_principal_lifecycle",
		"wrong_owner_shape":    "ALTER TABLE mindlink_principal_lifecycle MODIFY owner_id VARBINARY(21) NOT NULL",
		"missing_identity_key": "ALTER TABLE mindlink_principal_lifecycle DROP INDEX uniq_mindlink_lifecycle_avatar",
		"prefix_identity_key":  "ALTER TABLE mindlink_principal_lifecycle DROP INDEX uniq_mindlink_lifecycle_avatar, ADD UNIQUE KEY uniq_mindlink_lifecycle_avatar(environment,tenant_id,appkey,avatar_id(8))",
	} {
		t.Run(name, func(t *testing.T) {
			db := nativeFixture(t)
			if _, err := NewNativeFence(context.Background(), db); err != nil {
				t.Fatal("fresh/upgraded lifecycle not ready", err)
			}
			if _, err := db.Exec(ddl); err != nil {
				t.Fatal(err)
			}
			if fence, err := NewNativeFence(context.Background(), db); err == nil || fence != nil {
				t.Fatal("invalid lifecycle schema advertised readiness")
			}
		})
	}
}

func TestMySQLPrincipalLifecycleExistingRowSerializesDistinctOperations(t *testing.T) {
	db := nativeFixture(t)
	v, auth, _ := principalProof(t)
	body, err := v.ParsePrincipalEnsure(auth)
	if err != nil {
		t.Fatal(err)
	}
	c := auth.claims
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	seed, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	defer seed.Rollback()
	if _, err = lockPrincipalLifecycle(ctx, seed, c, body.AvatarID, true); err != nil {
		t.Fatal(err)
	}
	if err = seed.Commit(); err != nil {
		t.Fatal(err)
	}
	hold, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Rollback()
	if _, err = lockPrincipalLifecycle(ctx, hold, c, body.AvatarID, false); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 2)
	for _, digit := range []string{"d", "e"} {
		go func(digit string) {
			other := c
			other.OperationID = strings.Repeat(digit, 64)
			tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
			if err != nil {
				done <- err
				return
			}
			defer tx.Rollback()
			if _, err = lockPrincipalLifecycle(ctx, tx, other, body.AvatarID, true); err == nil {
				err = tx.Commit()
			}
			done <- err
		}(digit)
	}
	waitDisableLock(t, db, "mindlink_principal_lifecycle", 2)
	if err = hold.Commit(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = <-done; err != nil {
			t.Fatal("existing lifecycle introduced a lock upgrade deadlock", err)
		}
	}
}
