//go:build integration

package mindlink

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func prepareDisableTransaction(t *testing.T) (*sql.DB, *Verifier, ed25519.PrivateKey, *Authorization) {
	t.Helper()
	db := nativeFixture(t)
	for _, q := range []string{
		"DELETE FROM mindlink_tickets", "DELETE FROM mindlink_operations",
		"UPDATE mindlink_owner_barriers SET write_enabled=TRUE",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	v, key, original, _ := queryFixture(t)
	return db, v, key, disableTransactionProof(t, v, key, original, nil)
}

func disableTransactionProof(t *testing.T, v *Verifier, key ed25519.PrivateKey, c Claims, change func(*Claims)) *Authorization {
	t.Helper()
	c.IssuedAt, c.ExpiresAt, c.JTI = time.Now().Unix(), time.Now().Unix()+60, strings.Repeat("b", 64)
	if change != nil {
		change(&c)
	}
	body := disableBody()
	body["operation_id"], body["request_id"] = c.OperationID, c.RequestID
	raw := fixtureJSON(t, body)
	c.BodySHA256 = fmt.Sprintf("%x", sha256.Sum256(raw))
	token := fixtureToken(t, key, []byte(`{"alg":"EdDSA","typ":"mindlink-actor+jwt","kid":"fixture-key"}`), fixtureJSON(t, c))
	a, err := v.Verify(fixtureRequest(t, token, c.Method, c.Path, string(raw)), OperationPrincipalDisable, time.Now())
	if err != nil {
		t.Fatal("signed disable fixture rejected", err)
	}
	return a
}

func assertDisableProjection(t *testing.T, db *sql.DB, state string, generation, policy int64, operations, tickets int) {
	t.Helper()
	var gotState string
	var gotGeneration, gotPolicy int64
	if err := db.QueryRow("SELECT state,avatar_generation,policy_version FROM mindlink_principals WHERE appkey='fixture-app' AND principal_id=?", disablePrincipal).Scan(&gotState, &gotGeneration, &gotPolicy); err != nil || gotState != state || gotGeneration != generation || gotPolicy != policy {
		t.Fatalf("projection mismatch: state=%s generation=%d policy=%d err=%v", gotState, gotGeneration, gotPolicy, err)
	}
	for table, want := range map[string]int{"mindlink_operations": operations, "mindlink_tickets": tickets} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != want {
			t.Fatalf("%s: count=%d want=%d err=%v", table, count, want, err)
		}
	}
}

func TestMySQLDisableTransactionCreatesOriginalAndReplaysReadOnly(t *testing.T) {
	db, v, key, auth := prepareDisableTransaction(t)
	// This fixture grant is not a production authority feed or public readiness.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("SET SESSION time_zone='+08:00'"); err != nil {
		t.Fatal(err)
	}
	before := time.Now().UTC().Add(-time.Second)
	r, err := v.transactPrincipalDisable(context.Background(), db, auth)
	if err != nil || r == nil || r.Replayed {
		t.Fatalf("first typed disable failed: %v", err)
	}
	assertDisableProjection(t, db, "disabled", 3, 8, 1, 1)
	if err := v.validateDisableResult(r.Result, auth.claims); err != nil {
		t.Fatal("producer did not emit exact N1 result", err)
	}
	var receipt OperationResult
	if json.Unmarshal(r.Result, &receipt) != nil || receipt.State != "ready" {
		t.Fatal("producer returned a nonterminal receipt")
	}
	accepted, err := time.Parse(ReceiptTimeFormat, receipt.AcceptedAt)
	if err != nil || accepted.Before(before) || accepted.After(time.Now().UTC()) {
		t.Fatal("first journal observation was not UTC")
	}
	var raw, created []byte
	if err := db.QueryRow("SELECT result_json,DATE_FORMAT(created_time,'%Y-%m-%dT%H:%i:%s.%fZ') FROM mindlink_operations").Scan(&raw, &created); err != nil || !bytes.Equal(raw, r.Result) {
		t.Fatal("receipt was not saved with the projection", err)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, string(created))
	if err != nil || createdAt.Format(ReceiptTimeFormat) != receipt.AcceptedAt {
		t.Fatal("accepted_at is not the first journal record time")
	}
	newOperation := disableTransactionProof(t, v, key, auth.claims, func(c *Claims) {
		c.OperationID, c.RequestID, c.JTI = strings.Repeat("e", 64), "new-disable", strings.Repeat("f", 64)
	})
	if result, err := v.transactPrincipalDisable(context.Background(), db, newOperation); result != nil || !errors.Is(err, ErrPrincipalDisabled) {
		t.Fatalf("new operation on tombstone: got %v want PRINCIPAL_DISABLED", err)
	}
	assertDisableProjection(t, db, "disabled", 3, 8, 1, 1)
	if _, err := db.Exec("UPDATE mindlink_owner_barriers SET write_enabled=FALSE; UPDATE mindlink_principals SET policy_version=9,avatar_generation=4"); err != nil {
		t.Fatal(err)
	}
	renewed := disableTransactionProof(t, v, key, auth.claims, func(c *Claims) { c.JTI = strings.Repeat("c", 64) })
	replay, err := v.transactPrincipalDisable(context.Background(), db, renewed)
	if err != nil || replay == nil || !replay.Replayed || !bytes.Equal(replay.Result, raw) {
		t.Fatalf("current reader lost original after disable/version change: %v", err)
	}
	assertDisableProjection(t, db, "disabled", 4, 9, 1, 2)
	query := queryProof(t, v, key, auth.claims, func(c *Claims) { c.ExpectedPolicyVersion, c.DesiredPolicyVersion = 9, 9 })
	queried, err := v.QueryOperation(context.Background(), db, query, func(tx *sql.Tx, c, original Claims) error {
		return authorizeOperationRead(context.Background(), tx, c, original)
	})
	if err != nil || queried == nil || !bytes.Equal(queried.Result, raw) {
		t.Fatalf("GET could not recover producer's immutable result: %v", err)
	}
	if _, err := db.Exec("UPDATE mindlink_owner_barriers SET read_enabled=FALSE"); err != nil {
		t.Fatal(err)
	}
	if result, err := v.transactPrincipalDisable(context.Background(), db, renewed); result != nil || !errors.Is(err, ErrForbidden) {
		t.Fatalf("revoked reader recovered original: %v", err)
	}
	assertDisableProjection(t, db, "disabled", 4, 9, 1, 3)
}

func TestMySQLDisableTransactionRejectsCurrentAuthorityWithoutJournal(t *testing.T) {
	for _, tc := range []struct {
		name, statement string
		want            error
	}{
		{"read_only", "UPDATE mindlink_owner_barriers SET write_enabled=FALSE", ErrForbidden},
		{"missing_owner", "DELETE FROM mindlink_owner_barriers", ErrNotReady},
		{"missing_principal", "DELETE FROM mindlink_principals", ErrNotReady},
		{"scope", "UPDATE mindlink_principals SET owner_id='43'", ErrScopeMismatch},
		{"avatar", "UPDATE mindlink_principals SET avatar_id='other'", ErrScopeMismatch},
		{"stale_policy", "UPDATE mindlink_principals SET policy_version=9", ErrStalePolicy},
		{"generation_regression", "UPDATE mindlink_principals SET avatar_generation=4", ErrStalePolicy},
		{"disabled", "UPDATE mindlink_principals SET state='disabled'", ErrPrincipalDisabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, v, _, auth := prepareDisableTransaction(t)
			if _, err := db.Exec(tc.statement); err != nil {
				t.Fatal(err)
			}
			snapshot := func() string {
				var principal, grant string
				if err := db.QueryRow("SELECT CONCAT(COUNT(*),':',COALESCE(MAX(CONCAT_WS('|',environment,tenant_id,appkey,owner_id,avatar_id,avatar_generation,policy_version,state)),'')) FROM mindlink_principals").Scan(&principal); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRow("SELECT CONCAT(COUNT(*),':',COALESCE(MAX(CONCAT_WS('|',environment,tenant_id,appkey,owner_id,read_enabled,write_enabled)),'')) FROM mindlink_owner_barriers").Scan(&grant); err != nil {
					t.Fatal(err)
				}
				return principal + "/" + grant
			}
			before := snapshot()
			r, err := v.transactPrincipalDisable(context.Background(), db, auth)
			if r != nil || !errors.Is(err, tc.want) {
				t.Fatalf("invalid authority accepted: got %v want %v", err, tc.want)
			}
			if snapshot() != before {
				t.Fatal("rejected request changed or created authority/projection")
			}
			for _, table := range []string{"mindlink_operations", "mindlink_tickets"} {
				var count int
				if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
					t.Fatalf("rejected request left %s rows: %d err=%v", table, count, err)
				}
			}
		})
	}
}

func TestMySQLDisableTransactionJournalFailureRollsBackEffect(t *testing.T) {
	db, v, _, auth := prepareDisableTransaction(t)
	if _, err := db.Exec("CREATE TRIGGER _mindlink_fail_receipt BEFORE UPDATE ON mindlink_operations FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='fixture receipt fault'"); err != nil {
		t.Fatal(err)
	}
	r, err := v.transactPrincipalDisable(context.Background(), db, auth)
	if r != nil || !errors.Is(err, ErrUnknownResult) {
		t.Fatalf("journal failure returned success: %v", err)
	}
	assertDisableProjection(t, db, "active", 2, 7, 0, 0)
}

func TestMySQLDisableTransactionLostCommitResponseRecoversOriginal(t *testing.T) {
	db, v, _, auth := prepareDisableTransaction(t)
	cfg := ownedDBConfig(t)
	if err := db.QueryRow("SELECT DATABASE()").Scan(&cfg.DBName); err != nil || !strings.HasPrefix(cfg.DBName, "mindlink_policy_c82_native_") {
		t.Fatal("refusing a non-owned scratch proxy")
	}
	addr, dropped := loseOneCommitResponse(t, cfg.Addr)
	cfg.Addr = addr
	proxied, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("open owned proxy")
	}
	defer proxied.Close()
	r, err := v.transactPrincipalDisable(context.Background(), proxied, auth)
	if r != nil || !errors.Is(err, ErrUnknownResult) || !dropped.Load() {
		t.Fatalf("actual lost COMMIT ACK: got %v dropped=%t", err, dropped.Load())
	}
	assertDisableProjection(t, db, "disabled", 3, 8, 1, 1)
	var raw []byte
	if err := db.QueryRow("SELECT result_json FROM mindlink_operations").Scan(&raw); err != nil || v.validateDisableResult(raw, auth.claims) != nil {
		t.Fatal("server did not save a typed original despite lost response", err)
	}
	// A new SQL pool is a new consumer, not a restarted IM process.
	cfg = ownedDBConfig(t)
	if err := db.QueryRow("SELECT DATABASE()").Scan(&cfg.DBName); err != nil {
		t.Fatal(err)
	}
	reopened, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("reopen owned scratch")
	}
	defer reopened.Close()
	r, err = v.transactPrincipalDisable(context.Background(), reopened, auth)
	if err != nil || r == nil || !r.Replayed || !bytes.Equal(r.Result, raw) {
		t.Fatalf("lost response recovery changed original or repeated effect: %v", err)
	}
	assertDisableProjection(t, db, "disabled", 3, 8, 1, 1)
}

func TestMySQLDisableTransactionRefusesCorruptOrNonterminalReplay(t *testing.T) {
	for _, state := range []string{"corrupt", "accepted"} {
		t.Run(state, func(t *testing.T) {
			db, v, key, auth := prepareDisableTransaction(t)
			r, err := v.transactPrincipalDisable(context.Background(), db, auth)
			if err != nil || r == nil {
				t.Fatal("first disable failed", err)
			}
			raw := []byte(`{}`)
			if state == "accepted" {
				var result OperationResult
				if json.Unmarshal(r.Result, &result) != nil {
					t.Fatal("invalid first result")
				}
				result.State, result.EffectivePolicyVersion, result.EffectiveAvatarGeneration, result.CommitVersion, result.CommittedAt = "accepted", nil, nil, nil, nil
				raw = fixtureJSON(t, result)
				if v.validateDisableResult(raw, auth.claims) != nil {
					t.Fatal("accepted fixture is not a valid generic N1 shape")
				}
			}
			if _, err := db.Exec("UPDATE mindlink_operations SET result_json=?", raw); err != nil {
				t.Fatal(err)
			}
			renewed := disableTransactionProof(t, v, key, auth.claims, func(c *Claims) { c.JTI = strings.Repeat("c", 64) })
			if r, err := v.transactPrincipalDisable(context.Background(), db, renewed); r != nil || !errors.Is(err, ErrUnknownResult) {
				t.Fatalf("invalid original replayed: %v", err)
			}
			assertDisableProjection(t, db, "disabled", 3, 8, 1, 1)
		})
	}
}

func waitDisableLock(t *testing.T, db *sql.DB, table string, count int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := db.QueryRow(`SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks l ON l.ENGINE_LOCK_ID=w.REQUESTING_ENGINE_LOCK_ID AND l.ENGINE=w.ENGINE WHERE l.OBJECT_SCHEMA=DATABASE() AND l.OBJECT_NAME=?`, table).Scan(&waiting); err != nil {
			t.Fatal("inspect actual owned lock wait", err)
		}
		if waiting >= count {
			var legacyMatches int
			if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.INNODB_TRX t JOIN information_schema.PROCESSLIST p ON p.ID=t.trx_mysql_thread_id WHERE t.trx_state='LOCK WAIT' AND p.DB=DATABASE() AND t.trx_query LIKE ?`, "%"+table+"%").Scan(&legacyMatches); err != nil {
				t.Fatal(err)
			}
			t.Logf("owned table %s: actual lock waits=%d, query-text observer matches=%d", table, waiting, legacyMatches)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no actual owned authority lock wait observed")
}

func TestMySQLDisableTransactionSerializesWriteRevocation(t *testing.T) {
	for _, revokeFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("revoke_first_%t", revokeFirst), func(t *testing.T) {
			db, v, _, auth := prepareDisableTransaction(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			locker, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer locker.Rollback()
			if revokeFirst {
				_, err = locker.ExecContext(ctx, "UPDATE mindlink_owner_barriers SET write_enabled=FALSE")
			} else {
				_, err = locker.ExecContext(ctx, "SELECT principal_id FROM mindlink_principals FOR UPDATE")
			}
			if err != nil {
				t.Fatal(err)
			}
			type outcome struct {
				r   *MutationResult
				err error
			}
			done := make(chan outcome, 1)
			go func() { r, err := v.transactPrincipalDisable(ctx, db, auth); done <- outcome{r, err} }()
			if revokeFirst {
				waitDisableLock(t, db, "mindlink_owner_barriers", 1)
				if err := locker.Commit(); err != nil {
					t.Fatal(err)
				}
				select {
				case got := <-done:
					if got.r != nil || !errors.Is(got.err, ErrForbidden) {
						t.Fatalf("revocation waiter committed: %v", got.err)
					}
				case <-ctx.Done():
					t.Fatal("disable waiter did not finish")
				}
				assertDisableProjection(t, db, "active", 2, 7, 0, 0)
				return
			}
			waitDisableLock(t, db, "mindlink_principals", 1)
			revoked := make(chan error, 1)
			go func() {
				_, err := db.ExecContext(ctx, "UPDATE mindlink_owner_barriers SET write_enabled=FALSE")
				revoked <- err
			}()
			waitDisableLock(t, db, "mindlink_owner_barriers", 1)
			if err := locker.Rollback(); err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-done:
				if got.err != nil || got.r == nil || got.r.Replayed {
					t.Fatalf("prior authorized writer did not commit: %v", got.err)
				}
			case <-ctx.Done():
				t.Fatal("disable writer did not finish")
			}
			select {
			case err := <-revoked:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("revoker did not finish")
			}
			assertDisableProjection(t, db, "disabled", 3, 8, 1, 1)
			var writable bool
			if err := db.QueryRow("SELECT write_enabled FROM mindlink_owner_barriers").Scan(&writable); err != nil || writable {
				t.Fatal("revocation did not persist after prior writer", err)
			}
		})
	}
}
