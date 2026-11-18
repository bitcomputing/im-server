//go:build integration

package mindlink

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func reliableWait(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		if err := db.QueryRow(query, args...).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("owned SQL lock observation timed out; race not verified")
}

const reliableOwnerWait = `SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks l ON l.ENGINE=w.ENGINE AND l.ENGINE_LOCK_ID=w.REQUESTING_ENGINE_LOCK_ID WHERE l.OBJECT_SCHEMA=DATABASE() AND l.OBJECT_NAME='mindlink_owner_barriers'`

func TestMySQLReliableMessageRevocationOrder(t *testing.T) {
	t.Run("revoke_first", func(t *testing.T) {
		db := reliableDB(t)
		reliableProjection(t, db, "avatar_owner_dm")
		store, err := NewMessageStore(context.Background(), db)
		if err != nil {
			t.Fatal(err)
		}
		v, auth := reliableProof(t, messageRequest(), "owner", 0)
		revoke, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer revoke.Rollback()
		if _, err := revoke.Exec("UPDATE mindlink_owner_barriers SET write_enabled=0"); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() { _, err := store.Send(ctx, v, auth); done <- err }()
		reliableWait(t, db, reliableOwnerWait)
		if err := revoke.Commit(); err != nil {
			t.Fatal(err)
		}
		if err := <-done; !errors.Is(err, ErrForbidden) {
			t.Fatalf("revoke-first send: %v", err)
		}
		for _, table := range []string{"p_hismsgs", "mindlink_operations", "mindlink_message_outbox", "message_sequence"} {
			if reliableRows(t, db, table) != 0 {
				t.Fatalf("revoked effect retained in %s", table)
			}
		}
	})
	t.Run("send_first", func(t *testing.T) {
		db := reliableDB(t)
		reliableProjection(t, db, "avatar_owner_dm")
		store, err := NewMessageStore(context.Background(), db)
		if err != nil {
			t.Fatal(err)
		}
		const gate = "mindlink-reliable-3f7-before-history"
		conn, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		var acquired int
		if err := conn.QueryRowContext(context.Background(), "SELECT GET_LOCK(?,0)", gate).Scan(&acquired); err != nil || acquired != 1 {
			t.Fatal("owned fixture gate unavailable")
		}
		defer conn.ExecContext(context.Background(), "DO RELEASE_LOCK(?)", gate)
		if _, err := db.Exec("CREATE TRIGGER _reliable_pause BEFORE INSERT ON p_hismsgs FOR EACH ROW BEGIN DECLARE acquired INT; SET acquired=GET_LOCK('" + gate + "',10); IF acquired IS NULL OR acquired<>1 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned fixture gate failed'; END IF; SET acquired=RELEASE_LOCK('" + gate + "'); END"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := db.Exec("DROP TRIGGER IF EXISTS _reliable_pause"); err != nil {
				t.Error(err)
			}
		})
		v, auth := reliableProof(t, messageRequest(), "owner", 0)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		type outcome struct {
			result *MutationResult
			err    error
		}
		sent := make(chan outcome, 1)
		go func() { r, e := store.Send(ctx, v, auth); sent <- outcome{r, e} }()
		reliableWait(t, db, "SELECT COUNT(*) FROM performance_schema.metadata_locks WHERE OBJECT_TYPE='USER LEVEL LOCK' AND OBJECT_NAME=? AND LOCK_STATUS='PENDING'", gate)
		revoked := make(chan error, 1)
		go func() {
			tx, e := db.BeginTx(ctx, nil)
			if e != nil {
				revoked <- e
				return
			}
			defer tx.Rollback()
			if _, e = tx.ExecContext(ctx, "UPDATE mindlink_owner_barriers SET write_enabled=0"); e == nil {
				e = tx.Commit()
			}
			revoked <- e
		}()
		reliableWait(t, db, reliableOwnerWait)
		if _, err := conn.ExecContext(ctx, "DO RELEASE_LOCK(?)", gate); err != nil {
			t.Fatal(err)
		}
		first := <-sent
		if first.err != nil || first.result == nil {
			t.Fatalf("send-first commit: %v", first.err)
		}
		if err := <-revoked; err != nil {
			t.Fatal(err)
		}
		qv, query := reliableQueryProof(t, auth, 7)
		original, err := store.Query(ctx, qv, query)
		if err != nil || original == nil || string(original.Result) != string(first.result.Result) {
			t.Fatalf("write revocation lost committed result: %v", err)
		}
		for _, table := range []string{"p_hismsgs", "mindlink_operations", "mindlink_message_outbox", "message_sequence"} {
			if reliableRows(t, db, table) != 1 {
				t.Fatalf("send-first count in %s", table)
			}
		}
	})
}
